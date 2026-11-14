package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func systemPaths() (string, string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	if !filepath.IsAbs(home) {
		return "", "", fmt.Errorf("Домашний каталог недоступен")
	}
	return filepath.Join(home, ".config", "SpaceClient", "client_settings.ini"), "/opt/thin-client/start_vdi_client.sh", nil
}

func checkProcesses(exe string) error {
	if _, _, err := readRegular(exe); err != nil {
		return fmt.Errorf("Обёртка Space Client не найдена: %w", err)
	}
	return checkUserProcesses("/proc", os.Getuid())
}

func checkUserProcesses(procRoot string, uid int) error {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return err
	}
	var inspectionErr error
	for _, entry := range entries {
		if _, err := strconv.Atoi(entry.Name()); err != nil {
			continue
		}
		running, err := processIsClient(filepath.Join(procRoot, entry.Name()), uid)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			if inspectionErr == nil {
				inspectionErr = fmt.Errorf("Невозможно проверить процесс %s: %w", entry.Name(), err)
			}
			continue
		}
		if running {
			return errors.New(RunningMessage)
		}
	}
	return inspectionErr
}

func processIsClient(proc string, uid int) (bool, error) {
	status, err := os.ReadFile(filepath.Join(proc, "status"))
	if err != nil {
		return false, err
	}
	name, own, hasUID := "", false, false
	for _, line := range strings.Split(string(status), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		switch fields[0] {
		case "Name:":
			name = strings.TrimSpace(strings.TrimPrefix(line, "Name:"))
		case "Uid:":
			if len(fields) != 5 {
				return false, errors.New("Некорректное поле Uid процесса")
			}
			for _, field := range fields[1:] {
				value, err := strconv.ParseUint(field, 10, 32)
				if err != nil {
					return false, fmt.Errorf("Некорректный UID процесса: %w", err)
				}
				own = own || value == uint64(uid)
			}
			hasUID = true
		}
	}
	if name == "" || !hasUID {
		return false, errors.New("В status процесса отсутствует поле Name или Uid")
	}
	return own && (name == "space_client" || name == "electron-client"), nil
}

func lockFile(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, errors.Join(fmt.Errorf("Другой Launcher выполняет операцию: %w", err), f.Close())
	}
	return func() error { return f.Close() }, nil
}
func protectTemp(temp, path string, backup bool, mode os.FileMode) error {
	if backup {
		path = strings.TrimSuffix(path, BackupSuffix)
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	stat := info.Sys().(*syscall.Stat_t)
	if err = os.Chown(temp, int(stat.Uid), int(stat.Gid)); err != nil {
		return err
	}
	if err = os.Chmod(temp, mode); err != nil {
		return err
	}
	size, err := unix.Listxattr(path, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return nil
	}
	if err != nil {
		return err
	}
	names := make([]byte, size)
	n, err := unix.Listxattr(path, names)
	if err != nil {
		return err
	}
	hasACL := false
	for _, name := range strings.Split(string(names[:n]), "\x00") {
		if name == "" {
			continue
		}
		hasACL = hasACL || name == "system.posix_acl_access"
		size, err = unix.Getxattr(path, name, nil)
		if err != nil {
			return err
		}
		value := make([]byte, size)
		n, err = unix.Getxattr(path, name, value)
		if err != nil {
			return err
		}
		value = value[:n]
		old := make([]byte, len(value))
		oldSize, readErr := unix.Getxattr(temp, name, old)
		if readErr == nil && oldSize == len(value) && bytes.Equal(old, value) {
			continue
		}
		if err = unix.Setxattr(temp, name, value, 0); err != nil {
			return err
		}
	}
	if !hasACL {
		err = unix.Removexattr(temp, "system.posix_acl_access")
		if err != nil && !errors.Is(err, unix.ENODATA) && !errors.Is(err, unix.ENOTSUP) {
			return err
		}
	}
	return nil
}
func replaceFile(temp, path string) error { return os.Rename(temp, path) }
func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	return errors.Join(f.Sync(), f.Close())
}
