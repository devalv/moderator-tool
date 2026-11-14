package launcher

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

func systemPaths() (string, string, error) {
	root := os.Getenv("LOCALAPPDATA")
	if !filepath.IsAbs(root) {
		return "", "", errors.New("LOCALAPPDATA недоступен или не является абсолютным путём")
	}

	return filepath.Join(root, "SpaceClient", "client_settings.ini"), `C:\Program Files\Space Client\space_client.exe`, nil
}

func checkProcesses(exe string) (err error) {
	const maxPathLength = 32768
	if _, _, err := readRegular(exe); err != nil {
		return fmt.Errorf("клиент Space Client не найден: %w", err)
	}
	h, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return fmt.Errorf("не удалось получить список процессов: %w", err)
	}
	defer func() { err = errors.Join(err, windows.CloseHandle(h)) }()
	entry := windows.ProcessEntry32{Size: uint32(unsafe.Sizeof(windows.ProcessEntry32{}))}
	for e := windows.Process32First(h, &entry); ; e = windows.Process32Next(h, &entry) {
		if errors.Is(e, windows.ERROR_NO_MORE_FILES) {
			return nil
		}
		if e != nil {
			return fmt.Errorf("не удалось проверить процесс: %w", e)
		}
		name := windows.UTF16ToString(entry.ExeFile[:])
		if !strings.EqualFold(name, "space_client.exe") && !strings.EqualFold(name, "electron-client.exe") {
			continue
		}
		p, e := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, entry.ProcessID)
		if e != nil {
			return fmt.Errorf("не удалось проверить процесс %d: %w", entry.ProcessID, e)
		}
		buf := make([]uint16, maxPathLength)
		size := uint32(maxPathLength)
		e = windows.QueryFullProcessImageName(p, 0, &buf[0], &size)
		e = errors.Join(e, windows.CloseHandle(p))
		if e != nil {
			return fmt.Errorf("не удалось проверить процесс: %w", e)
		}
		path := filepath.Clean(windows.UTF16ToString(buf[:size]))
		electron := filepath.Join(filepath.Dir(exe), "electron-client-win32-x64", "electron-client.exe")
		if strings.EqualFold(path, exe) || strings.EqualFold(path, electron) {
			return errors.New(RunningMessage)
		}
	}
}

func lockFile(path string) (func() error, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, privateFileMode)
	if err != nil {
		return nil, fmt.Errorf("не удалось открыть файл блокировки: %w", err)
	}
	o := &windows.Overlapped{}
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	if err = windows.LockFileEx(windows.Handle(f.Fd()), flags, 0, 1, 0, o); err != nil {
		return nil, errors.Join(fmt.Errorf("другой Launcher выполняет операцию: %w", err), f.Close())
	}

	return func() error { return f.Close() }, nil
}

func protectTemp(temp, path string, backup bool, mode os.FileMode) error {
	if backup {
		path = strings.TrimSuffix(path, BackupSuffix)
	}
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("не удалось прочитать защиту файла: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("не удалось прочитать DACL файла: %w", err)
	}
	const security = windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION
	if err = windows.SetNamedSecurityInfo(temp, windows.SE_FILE_OBJECT, security, nil, nil, dacl, nil); err != nil {
		return fmt.Errorf("не удалось установить защиту файла: %w", err)
	}
	if err := os.Chmod(temp, mode); err != nil {
		return fmt.Errorf("не удалось установить права файла: %w", err)
	}

	return nil
}

func replaceFile(temp, path string) error {
	from, err := windows.UTF16PtrFromString(temp)
	if err != nil {
		return fmt.Errorf("не удалось преобразовать путь временного файла: %w", err)
	}
	to, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return fmt.Errorf("не удалось преобразовать путь целевого файла: %w", err)
	}
	if err := windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH); err != nil {
		return fmt.Errorf("не удалось заменить файл: %w", err)
	}

	return nil
}
func syncDir(string) error { return nil }
