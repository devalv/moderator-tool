package launcher

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestProcessHelper(t *testing.T) {
	if os.Getenv("GLINT_TEST_PROCESS") != "1" {
		return
	}
	if _, err := os.Stdout.WriteString("ready\n"); err != nil {
		os.Exit(2)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestWindowsPaths(t *testing.T) {
	root := t.TempDir()
	t.Setenv("LOCALAPPDATA", root)
	ini, exe, err := systemPaths()
	if err != nil || ini != filepath.Join(root, "SpaceClient", "client_settings.ini") || exe != `C:\Program Files\Space Client\space_client.exe` {
		t.Fatalf("paths: %s %s %v", ini, exe, err)
	}
	t.Setenv("LOCALAPPDATA", "")
	if _, _, err := systemPaths(); err == nil {
		t.Fatal("empty LOCALAPPDATA accepted")
	}
}

func denyDelete(t *testing.T, path string) func() {
	t.Helper()
	p, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(p, windows.GENERIC_READ, windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE, nil, windows.OPEN_EXISTING, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	return func() {
		if err := windows.CloseHandle(h); err != nil {
			t.Error(err)
		}
	}
}

func TestWindowsReplaceFailurePreservesBackup(t *testing.T) {
	l, starts := fixture(t)
	closeFile := denyDelete(t, l.INI)
	defer closeFile()
	if err := connect(l, nil); err == nil || *starts != 0 {
		t.Fatal("replacement error hidden")
	}
	if get(t, l.INI) != original || get(t, l.INI+BackupSuffix) != original {
		t.Fatal("original/backup lost")
	}
}

func TestWindowsBackupDeletionFailure(t *testing.T) {
	l, _ := fixture(t)
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
	closeFile := denyDelete(t, l.INI+BackupSuffix)
	err := l.Restore(nil, quiet)
	closeFile()
	if err == nil {
		t.Fatal("deletion error hidden")
	}
	if get(t, l.INI) != original || get(t, l.INI+BackupSuffix) != original {
		t.Fatal("restored INI/backup lost")
	}
	c := approve(t, l.Restore(nil, quiet))
	if err := l.Restore(c, quiet); err != nil {
		t.Fatal(err)
	}
	absent(t, l.INI+BackupSuffix)
}

func TestWindowsProcessIdentity(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "space_client.exe")
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(self)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, data, 0700); err != nil {
		t.Fatal(err)
	}
	if err := checkProcesses(target); err != nil {
		t.Fatal(err)
	}
	for _, location := range []string{filepath.Join(root, "unrelated", "space_client.exe"), target, filepath.Join(root, "electron-client-win32-x64", "electron-client.exe")} {
		t.Run(filepath.Base(filepath.Dir(location)), func(t *testing.T) {
			if err := os.MkdirAll(filepath.Dir(location), 0700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(location, data, 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(location, "-test.run=^TestProcessHelper$")
			cmd.Env = append(os.Environ(), "GLINT_TEST_PROCESS=1")
			input, err := cmd.StdinPipe()
			if err != nil {
				t.Fatal(err)
			}
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := input.Close(); err != nil {
					t.Error(err)
				}
				if err := cmd.Wait(); err != nil {
					t.Error(err)
				}
			}()
			ready := make([]byte, 6)
			if _, err := io.ReadFull(output, ready); err != nil {
				t.Fatal(err)
			}
			err = checkProcesses(target)
			wantRunning := filepath.Dir(location) != filepath.Join(root, "unrelated")
			if wantRunning && (err == nil || err.Error() != RunningMessage) {
				t.Fatalf("expected running, got %v", err)
			}
			if !wantRunning && err != nil {
				t.Fatalf("unrelated process blocked operation: %v", err)
			}
		})
	}
}
