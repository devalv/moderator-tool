package launcher

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const (
	privateFileMode = 0o600
	BackupSuffix    = ".moderator-backup"
	RunningMessage  = "клиент Space Client уже запущен. Закройте его самостоятельно и повторите операцию"
)

// ConfirmationError binds approval to the exact files and requested configuration.
type ConfirmationError struct {
	current, backup, prepared []byte
	restore                   bool
}

func (*ConfirmationError) Error() string {
	return "Текущие настройки отличаются от режима модератора. Продолжить?"
}

type Launcher struct {
	INI, Executable string
	Check           func() error
	Start           func() error
	mu              sync.Mutex
}

func New() (*Launcher, error) {
	ini, exe, err := systemPaths()
	if err != nil {
		return nil, err
	}
	l := &Launcher{INI: ini, Executable: exe}
	l.Check = func() error { return checkProcesses(exe) }
	l.Start = func() error {
		cmd := exec.Command(exe)
		cmd.Dir = filepath.Dir(exe)
		if err := cmd.Start(); err != nil {
			return fmt.Errorf("не удалось запустить процесс: %w", err)
		}
		go func() { _ = cmd.Wait() }()

		return nil
	}

	return l, nil
}

func Validate(address, port string) error {
	address = strings.TrimSpace(address)
	if net.ParseIP(address) == nil {
		host := strings.TrimSuffix(address, ".")
		if len(host) == 0 || len(host) > 253 {
			return errors.New("укажите корректный IP-адрес или hostname")
		}
		for _, label := range strings.Split(host, ".") {
			if len(label) == 0 || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
				return errors.New("некорректный hostname")
			}
			for _, c := range label {
				if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
					return errors.New("некорректный hostname")
				}
			}
		}
	}
	for _, c := range port {
		if c < '0' || c > '9' {
			return errors.New("порт: целое число от 1 до 65535")
		}
	}
	n, err := strconv.Atoi(port)
	if err != nil || n < 1 || n > 65535 {
		return errors.New("порт: целое число от 1 до 65535")
	}

	return nil
}

func readRegular(path string) ([]byte, os.FileMode, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, 0, fmt.Errorf("не удалось прочитать файл %s: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("ожидался обычный файл: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("не удалось прочитать файл %s: %w", path, err)
	}

	return data, info.Mode().Perm(), nil
}

func (l *Launcher) BackupExists() (bool, error) {
	info, err := os.Lstat(l.INI + BackupSuffix)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("не удалось проверить backup: %w", err)
	}
	if !info.Mode().IsRegular() {
		return false, errors.New("backup не является обычным файлом")
	}

	return true, nil
}

func (l *Launcher) Connect(address, port string, approval *ConfirmationError, status func(string)) error {
	if err := Validate(address, port); err != nil {
		return err
	}

	return l.operate(false, strings.TrimSpace(address), port, approval, status)
}

func (l *Launcher) Restore(approval *ConfirmationError, status func(string)) error {
	return l.operate(true, "", "", approval, status)
}

func (l *Launcher) operate(restore bool, address, port string,
	approval *ConfirmationError, status func(string),
) (err error) {
	if !l.mu.TryLock() {
		return errors.New("операция уже выполняется")
	}
	defer l.mu.Unlock()
	status("Проверка…")
	unlock, err := lockFile(l.INI + ".moderator-lock")
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unlock()) }()
	if err = l.Check(); err != nil {
		return err
	}
	current, mode, err := readRegular(l.INI)
	if err != nil {
		return err
	}
	prepared, ready, err := editINI(current, address, port)
	if err != nil {
		return err
	}
	backup, backupMode, backupErr := readRegular(l.INI + BackupSuffix)
	if backupErr != nil && !errors.Is(backupErr, os.ErrNotExist) {
		return backupErr
	}
	exists := backupErr == nil
	if restore && !exists {
		return errors.New("резервная копия отсутствует")
	}
	if exists && !ready && !approval.matches(restore, current, backup, prepared) {
		return &ConfirmationError{current: current, backup: backup, prepared: prepared, restore: restore}
	}
	unchanged := func() error { return l.checkUnchanged(current, backup, exists) }
	if err = unchanged(); err != nil {
		return err
	}
	if restore {
		status("Восстановление…")

		return l.restoreBackup(backup, backupMode, unchanged)
	}
	status("Подготовка конфигурации…")
	if !exists {
		if err = writeFile(l.INI+BackupSuffix, current, mode, true, unchanged); err != nil {
			return err
		}
		backup, exists = current, true
	}
	if !ready {
		if err = writeFile(l.INI, prepared, mode, false, unchanged); err != nil {
			return err
		}
		current = prepared
	} else {
		status("Конфигурация уже подготовлена")
	}
	if err = unchanged(); err != nil {
		return err
	}
	status("Запуск Space Client…")
	if err = l.Start(); err != nil {
		return fmt.Errorf("не удалось запустить Space Client; backup сохранён: %w", err)
	}

	return nil
}

func writeFile(path string, data []byte, mode os.FileMode, backup bool, before func() error) (err error) {
	f, err := os.CreateTemp(filepath.Dir(path), ".moderator-*")
	if err != nil {
		return fmt.Errorf("не удалось создать временный файл: %w", err)
	}
	defer func() {
		if e := os.Remove(f.Name()); e != nil && !errors.Is(e, os.ErrNotExist) {
			err = errors.Join(err, e)
		}
	}()
	if err = protectTemp(f.Name(), path, backup, mode); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	if err = before(); err != nil {
		return err
	}
	// A hard link publishes complete bytes and never replaces an existing backup.
	if backup {
		err = os.Link(f.Name(), path)
	} else {
		err = replaceFile(f.Name(), path)
	}
	if err != nil {
		return err
	}

	return syncDir(filepath.Dir(path))
}

func (c *ConfirmationError) matches(restore bool, current, backup, prepared []byte) bool {
	return c != nil && c.restore == restore && bytes.Equal(c.current, current) &&
		bytes.Equal(c.backup, backup) && bytes.Equal(c.prepared, prepared)
}

func (l *Launcher) checkUnchanged(current, backup []byte, exists bool) error {
	if err := l.Check(); err != nil {
		return err
	}
	b, _, err := readRegular(l.INI)
	if err != nil {
		return err
	}
	if !bytes.Equal(b, current) {
		return errors.New("INI изменился во время операции. Повторите действие")
	}
	if exists {
		b, _, err = readRegular(l.INI + BackupSuffix)
		if err != nil {
			return err
		}
		if !bytes.Equal(b, backup) {
			return errors.New("backup изменился во время операции")
		}
	}

	return nil
}

func (l *Launcher) restoreBackup(backup []byte, mode os.FileMode, before func() error) error {
	if err := writeFile(l.INI, backup, mode, false, before); err != nil {
		return err
	}
	b, _, e := readRegular(l.INI)
	if e != nil {
		return e
	}
	if !bytes.Equal(b, backup) {
		return errors.New("проверка восстановления не пройдена; backup сохранён")
	}
	b, _, e = readRegular(l.INI + BackupSuffix)
	if e != nil {
		return e
	}
	if !bytes.Equal(b, backup) {
		return errors.New("backup изменился; копия сохранена")
	}
	if err := os.Remove(l.INI + BackupSuffix); err != nil {
		return fmt.Errorf("не удалось удалить backup: %w", err)
	}

	return nil
}
