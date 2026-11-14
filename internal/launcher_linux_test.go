package launcher

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/sys/unix"
)

func TestLinuxPaths(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ini, exe, err := systemPaths()
	if err != nil || ini != filepath.Join(home, ".config", "SpaceClient", "client_settings.ini") || exe != "/opt/thin-client/start_vdi_client.sh" {
		t.Fatalf("paths: %s %s %v", ini, exe, err)
	}
	t.Setenv("HOME", "")
	if _, _, err := systemPaths(); err == nil {
		t.Fatal("empty HOME accepted")
	}
}

func TestLinuxMetadata(t *testing.T) {
	l, _ := fixture(t)
	if err := os.Chmod(l.INI, 0640); err != nil {
		t.Fatal(err)
	}
	if err := unix.Setxattr(l.INI, "user.glint-test", []byte("preserved"), 0); err != nil {
		if errors.Is(err, unix.ENOTSUP) {
			t.Skip("filesystem does not support xattrs")
		}
		t.Fatal(err)
	}
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{l.INI, l.INI + BackupSuffix} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0640 {
			t.Fatalf("mode: %v", err)
		}
		value := make([]byte, 32)
		n, err := unix.Getxattr(path, "user.glint-test", value)
		if err != nil || string(value[:n]) != "preserved" {
			t.Fatalf("xattr: %v", err)
		}
	}
	if err := l.Restore(nil, quiet); err != nil {
		t.Fatal(err)
	}
}

func TestLinuxProcessHelper(t *testing.T) {
	if os.Getenv("GLINT_TEST_PROCESS") != "1" {
		return
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		os.Exit(2)
	}
	if _, err := os.Stdout.WriteString("ready\n"); err != nil {
		os.Exit(2)
	}
	_, _ = io.Copy(io.Discard, os.Stdin)
	os.Exit(0)
}

func TestLinuxProcessIdentity(t *testing.T) {
	for _, name := range []string{"space_client", "electron-client"} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			exe := filepath.Join(root, name)
			self, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			data, err := os.ReadFile(self)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(exe, data, 0700); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(exe, "-test.run=^TestLinuxProcessHelper$")
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
			if _, err := io.ReadFull(output, make([]byte, 6)); err != nil {
				t.Fatal(err)
			}
			if err := checkProcesses(exe); err == nil || err.Error() != RunningMessage {
				t.Fatalf("process not detected: %v", err)
			}
		})
	}
}

func TestLinuxUserProcesses(t *testing.T) {
	for _, tc := range []struct {
		name, status, want string
	}{
		{"own client", "Name: space_client\nUid: 1000 1000 1000 1000\n", RunningMessage},
		{"own electron", "Name: electron-client\nUid: 1000 1000 1000 1000\n", RunningMessage},
		{"foreign client", "Name: space_client\nUid: 2000 2000 2000 2000\n", ""},
		{"foreign electron", "Name: electron-client\nUid: 2000 2000 2000 2000\n", ""},
		{"sd-pam without exe", "Name: (sd-pam)\nUid: 1000 1000 1000 1000\n", ""},
		{"similar name", "Name: space_client2\nUid: 1000 1000 1000 1000\n", ""},
		{"similar electron", "Name: electron\nUid: 1000 1000 1000 1000\n", ""},
		{"UID before name", "Uid: 2000 1000 2000 2000\nName: space_client\n", RunningMessage},
		{"missing UID", "Name: space_client\n", "отсутствует поле"},
		{"missing name", "Uid: 1000 1000 1000 1000\n", "отсутствует поле"},
		{"malformed UID", "Name: space_client\nUid: 1000 invalid 1000 1000\n", "некорректный UID"},
		{"incomplete UID", "Name: space_client\nUid: 1000\n", "некорректное поле Uid"},
		{"unreadable status", "unreadable", "невозможно проверить процесс"},
		{"vanished process", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			procRoot := t.TempDir()
			proc := filepath.Join(procRoot, "123")
			if err := os.Mkdir(proc, 0700); err != nil {
				t.Fatal(err)
			}
			status := filepath.Join(proc, "status")
			if tc.status == "unreadable" {
				// A directory makes ReadFile fail even when tests run as root.
				if err := os.Mkdir(status, 0700); err != nil {
					t.Fatal(err)
				}
			} else if tc.status != "" {
				if err := os.WriteFile(status, []byte(tc.status), 0600); err != nil {
					t.Fatal(err)
				}
			}
			// No exe link exists: detection must rely only on status.
			for i := 0; i < 2; i++ {
				err := checkUserProcesses(procRoot, 1000)
				if tc.want == "" && err != nil || tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)) {
					t.Fatalf("check %d: got %v, want %q", i, err, tc.want)
				}
			}
		})
	}
}
