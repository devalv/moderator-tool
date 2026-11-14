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
		return "", "", fmt.Errorf("не удалось определить домашний каталог: %w", err)
	}
	if !filepath.IsAbs(home) {
		return "", "", errors.New("домашний каталог недоступен")
	}
	ini := filepath.Join(home, ".config", "SpaceClient", "client_settings.ini")

	return ini, "/opt/thin-client/start_vdi_client.sh", nil
}

func checkProcesses(exe string) error {
	if _, _, err := readRegular(exe); err != nil {
		return fmt.Errorf("обёртка Space Client не найдена: %w", err)
	}

	return checkUserProcesses("/proc", os.Getuid())
}

func checkUserProcesses(procRoot string, uid int) error {
	entries, err := os.ReadDir(procRoot)
	if err != nil {
		return fmt.Errorf("не удалось прочитать каталог процессов: %w", err)
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
				inspectionErr = fmt.Errorf("невозможно проверить процесс %s: %w", entry.Name(), err)
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
	const uidFieldCount = 5
	status, err := os.ReadFile(filepath.Join(proc, "status"))
	if err != nil {
		return false, fmt.Errorf("не удалось прочитать status процесса: %w", err)
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
			if len(fields) != uidFieldCount {
				return false, errors.New("некорректное поле Uid процесса")
			}
			for _, field := range fields[1:] {
				value, err := strconv.ParseUint(field, 10, 32)
				if err != nil {
					return false, fmt.Errorf("некорректный UID процесса: %w", err)
				}
				own = own || (uid >= 0 && value == uint64(uid))
			}
			hasUID = true
		}
	}
	if name == "" || !hasUID {
		return false, errors.New("в status процесса отсутствует поле Name или Uid")
	}

	return own && (name == "space_client" || name == "electron-client"), nil
}

func lockFile(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, privateFileMode)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл блокировки: %w", err)
	}
	if err = unix.Flock(int(f.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return nil, errors.Join(fmt.Errorf("другой Launcher выполняет операцию: %w", err), f.Close())
	}

	return func() error { return f.Close() }, nil
}

func protectTemp(temp, path string, backup bool, mode os.FileMode) error {
	if backup {
		path = strings.TrimSuffix(path, BackupSuffix)
	}
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("не удалось прочитать метаданные файла: %w", err)
	}
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return errors.New("не удалось прочитать владельца файла")
	}
	if err = os.Chown(temp, int(stat.Uid), int(stat.Gid)); err != nil {
		return fmt.Errorf("не удалось установить владельца файла: %w", err)
	}
	if err = os.Chmod(temp, mode); err != nil {
		return fmt.Errorf("не удалось установить права файла: %w", err)
	}
	size, err := unix.Listxattr(path, nil)
	if errors.Is(err, unix.ENOTSUP) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("не удалось определить размер списка атрибутов: %w", err)
	}
	names := make([]byte, size)
	n, err := unix.Listxattr(path, names)
	if err != nil {
		return fmt.Errorf("не удалось прочитать список атрибутов: %w", err)
	}
	hasACL := false
	for _, name := range strings.Split(string(names[:n]), "\x00") {
		if name == "" {
			continue
		}
		hasACL = hasACL || name == "system.posix_acl_access"
		size, err = unix.Getxattr(path, name, nil)
		if err != nil {
			return fmt.Errorf("не удалось определить размер атрибута: %w", err)
		}
		value := make([]byte, size)
		n, err = unix.Getxattr(path, name, value)
		if err != nil {
			return fmt.Errorf("не удалось прочитать атрибут: %w", err)
		}
		value = value[:n]
		old := make([]byte, len(value))
		oldSize, readErr := unix.Getxattr(temp, name, old)
		if readErr == nil && oldSize == len(value) && bytes.Equal(old, value) {
			continue
		}
		if err = unix.Setxattr(temp, name, value, 0); err != nil {
			return fmt.Errorf("не удалось записать атрибут: %w", err)
		}
	}
	if !hasACL {
		err = unix.Removexattr(temp, "system.posix_acl_access")
		if err != nil && !errors.Is(err, unix.ENODATA) && !errors.Is(err, unix.ENOTSUP) {
			return fmt.Errorf("не удалось удалить ACL: %w", err)
		}
	}

	return nil
}

func replaceFile(temp, path string) error {
	if err := os.Rename(temp, path); err != nil {
		return fmt.Errorf("не удалось заменить файл %s: %w", path, err)
	}

	return nil
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("не удалось открыть каталог для синхронизации: %w", err)
	}

	return errors.Join(f.Sync(), f.Close())
}
