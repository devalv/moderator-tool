package launcher

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const original = "; note\n[General]\ncur_remote_protocol_index=0\nkeep=hello\n[ServiceSettings]\nglobal_app_mode=0\n[GLINTSettings]\nstart_glint_session=1\n[Other]\nstart_glint_session=42\n"

func put(t *testing.T, path, text string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
}
func get(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
func fixture(t *testing.T) (*Launcher, *int) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "client_settings.ini")
	put(t, path, original)
	starts := new(int)
	return &Launcher{INI: path, Check: func() error { return nil }, Start: func() error { *starts++; return nil }}, starts
}
func quiet(string) {}
func connect(l *Launcher, c *ConfirmationError) error {
	return l.Connect("host.example", "443", c, quiet)
}
func approve(t *testing.T, err error) *ConfirmationError {
	t.Helper()
	var c *ConfirmationError
	if !errors.As(err, &c) {
		t.Fatalf("expected confirmation, got %v", err)
	}
	return c
}
func absent(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected absent file, got %v", err)
	}
}

func TestEditINI(t *testing.T) {
	want := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(original, "cur_remote_protocol_index=0", "cur_remote_protocol_index=3"), "global_app_mode=0", "global_app_mode=1"), "start_glint_session=1", "start_glint_session=0")
	for _, eol := range []string{"\n", "\r\n"} {
		for _, bom := range []string{"", "\xef\xbb\xbf"} {
			in := bom + strings.ReplaceAll(original, "\n", eol)
			out, ready, err := editINI([]byte(in), "", "")
			if err != nil || ready || string(out) != bom+strings.ReplaceAll(want, "\n", eol) {
				t.Fatalf("edit: ready=%v err=%v output=%q", ready, err, out)
			}
			out2, ready, err := editINI(out, "", "")
			if err != nil || !ready || !bytes.Equal(out, out2) {
				t.Fatalf("not idempotent: %v", err)
			}
		}
	}
}
func TestEditMissingKeysAndSections(t *testing.T) {
	for _, src := range []string{"", "[General]", "[General]\nx=2\n[Other]\nx=3", "[General]\n[ServiceSettings]\n[GLINTSettings]"} {
		out, ready, err := editINI([]byte(src), "", "")
		if err != nil || ready {
			t.Fatalf("%q: %v", src, err)
		}
		_, ready, err = editINI(out, "", "")
		if err != nil || !ready {
			t.Fatalf("invalid inserted keys: %q, %v", out, err)
		}
		if strings.Contains(src, "[Other]") && !strings.Contains(string(out), "[Other]\nx=3") {
			t.Fatal("other section changed")
		}
	}
}
func TestEditPreservesFormatting(t *testing.T) {
	src := "# заголовок\n [General] ; section\n  cur_remote_protocol_index = 9  ; keep\n[GLINTSettings]\nstart_glint_session = 1\t# comment\n[ServiceSettings]\nglobal_app_mode=0"
	want := strings.ReplaceAll(strings.ReplaceAll(strings.ReplaceAll(src, "= 9", "= 3"), "= 1\t", "= 0\t"), "mode=0", "mode=1")
	out, _, err := editINI([]byte(src), "", "")
	if err != nil || string(out) != want {
		t.Fatalf("%q, %v", out, err)
	}
}
func TestEditRejectsAmbiguity(t *testing.T) {
	for _, src := range []string{"[General]\n[General]", "[General]\nx=1\nx=2", "[Other]\nx=1\nx=2", "[General]\ncur_remote_protocol_index=1\ncur_remote_protocol_index=2", "[broken", "bad-line", "\xff", "x=\x00", "[General]\rcur_remote_protocol_index=0", "[General ]"} {
		if _, _, err := editINI([]byte(src), "", ""); err == nil {
			t.Fatalf("accepted %q", src)
		}
	}
}
func TestConnectRepeatAndRestore(t *testing.T) {
	l, starts := fixture(t)
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
	if *starts != 1 || get(t, l.INI+BackupSuffix) != original {
		t.Fatal("missing exact backup or launch")
	}
	prepared := get(t, l.INI)
	if !strings.Contains(prepared, "keep=hello") {
		t.Fatal("unexpected contents")
	}
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
	if *starts != 2 || get(t, l.INI) != prepared || get(t, l.INI+BackupSuffix) != original {
		t.Fatal("repeat changed files")
	}
	if err := l.Restore(nil, quiet); err != nil {
		t.Fatal(err)
	}
	if get(t, l.INI) != original {
		t.Fatal("restore differs")
	}
	absent(t, l.INI+BackupSuffix)
	if err := l.Restore(nil, quiet); err == nil {
		t.Fatal("restore without backup accepted")
	}
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
}
func TestConfirmationCancelAndStaleApproval(t *testing.T) {
	l, starts := fixture(t)
	put(t, l.INI+BackupSuffix, "original backup")
	c := approve(t, connect(l, nil))
	if *starts != 0 || get(t, l.INI) != original || get(t, l.INI+BackupSuffix) != "original backup" {
		t.Fatal("cancel changed files")
	}
	put(t, l.INI, original+"; new edit\n")
	c = approve(t, connect(l, c))
	if err := connect(l, c); err != nil {
		t.Fatal(err)
	}
	if get(t, l.INI+BackupSuffix) != "original backup" {
		t.Fatal("backup overwritten")
	}
}
func TestRestoreWarningAndCancel(t *testing.T) {
	l, _ := fixture(t)
	put(t, l.INI+BackupSuffix, "backup bytes")
	c := approve(t, l.Restore(nil, quiet))
	if get(t, l.INI) != original {
		t.Fatal("cancel altered INI")
	}
	if err := l.Restore(c, quiet); err != nil {
		t.Fatal(err)
	}
	if get(t, l.INI) != "backup bytes" {
		t.Fatal("not restored")
	}
	absent(t, l.INI+BackupSuffix)
}
func TestMissingINI(t *testing.T) {
	l, starts := fixture(t)
	if err := os.Remove(l.INI); err != nil {
		t.Fatal(err)
	}
	if err := connect(l, nil); err == nil || *starts != 0 {
		t.Fatal("missing INI accepted")
	}
	absent(t, l.INI+BackupSuffix)
}
func TestBackupFailure(t *testing.T) {
	l, starts := fixture(t)
	if err := os.Mkdir(l.INI+BackupSuffix, 0700); err != nil {
		t.Fatal(err)
	}
	if err := connect(l, nil); err == nil || *starts != 0 || get(t, l.INI) != original {
		t.Fatal("backup failure modified INI")
	}
}
func TestRunningAndUnknownProcessBlocks(t *testing.T) {
	for _, message := range []string{RunningMessage, "process inspection denied"} {
		l, starts := fixture(t)
		l.Check = func() error { return errors.New(message) }
		if err := connect(l, nil); err == nil {
			t.Fatal("process ignored")
		}
		absent(t, l.INI+BackupSuffix)
		put(t, l.INI+BackupSuffix, "backup")
		if err := l.Restore(nil, quiet); err == nil {
			t.Fatal("restore ignored process")
		}
		if *starts != 0 || get(t, l.INI) != original || get(t, l.INI+BackupSuffix) != "backup" {
			t.Fatal("files changed")
		}
	}
}
func TestProcessRecheckedBeforeCommit(t *testing.T) {
	l, starts := fixture(t)
	calls := 0
	l.Check = func() error {
		calls++
		if calls >= 3 {
			return errors.New(RunningMessage)
		}
		return nil
	}
	if err := connect(l, nil); err == nil || *starts != 0 || get(t, l.INI) != original {
		t.Fatal("process recheck missing")
	}
}

func TestProcessRecheckedBeforeLaunch(t *testing.T) {
	l, starts := fixture(t)
	calls := 0
	l.Check = func() error {
		calls++
		if calls >= 5 {
			return errors.New(RunningMessage)
		}
		return nil
	}
	if err := connect(l, nil); err == nil || *starts != 0 {
		t.Fatal("launch recheck missing")
	}
	if get(t, l.INI+BackupSuffix) != original {
		t.Fatal("backup lost")
	}
}

func TestDuplicatesAndCaseAmbiguityDoNotTouchFiles(t *testing.T) {
	for _, src := range []string{"[General]\n[General]", "[General]\ncur_remote_protocol_index=0\ncur_remote_protocol_index=1", "[General]\n[general]", "[general]", "[General]\nCUR_REMOTE_PROTOCOL_INDEX=0"} {
		l, starts := fixture(t)
		put(t, l.INI, src)
		if err := connect(l, nil); err == nil || *starts != 0 {
			t.Fatal("ambiguous INI accepted")
		}
		if get(t, l.INI) != src {
			t.Fatal("INI changed")
		}
		absent(t, l.INI+BackupSuffix)
	}
}
func TestLaunchFailureKeepsBackup(t *testing.T) {
	l, _ := fixture(t)
	l.Start = func() error { return errors.New("start failed") }
	if err := connect(l, nil); err == nil {
		t.Fatal("launch error hidden")
	}
	if get(t, l.INI+BackupSuffix) != original {
		t.Fatal("backup lost")
	}
	if err := l.Restore(nil, quiet); err != nil {
		t.Fatal(err)
	}
}
func TestRestoreFailureKeepsBackup(t *testing.T) {
	l, _ := fixture(t)
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
	err := l.Restore(nil, func(s string) {
		if s == "Восстановление…" {
			if e := os.Remove(l.INI); e != nil {
				t.Fatal(e)
			}
		}
	})
	if err == nil || get(t, l.INI+BackupSuffix) != original {
		t.Fatal("restore failure lost backup")
	}
}
func TestValidationDoesNotTouchFiles(t *testing.T) {
	for _, args := range [][2]string{{"", "443"}, {"bad host", "443"}, {"-bad", "443"}, {"host", "0"}, {"host", "65536"}, {"host", "77777"}, {"host", "+80"}, {"host", ""}, {"host", "abc"}} {
		l, starts := fixture(t)
		if err := l.Connect(args[0], args[1], nil, quiet); err == nil {
			t.Fatal("invalid accepted")
		}
		if *starts != 0 || get(t, l.INI) != original {
			t.Fatal("invalid modified INI")
		}
		absent(t, l.INI+BackupSuffix)
	}
	for _, host := range []string{"127.0.0.1", "::1", "example.test", "host", "host.example."} {
		if err := Validate(host, "65535"); err != nil {
			t.Fatal(err)
		}
	}
}
func TestAtomicPublicationNeverOverwritesBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "backup")
	put(t, path, "old")
	if err := writeFile(path, []byte("new"), 0600, true, func() error { return nil }); err == nil {
		t.Fatal("overwrite allowed")
	}
	if get(t, path) != "old" {
		t.Fatal("overwritten")
	}
}
func TestWriteFailureBeforeCommit(t *testing.T) {
	l, _ := fixture(t)
	if err := writeFile(l.INI, []byte("new"), 0600, false, func() error { return errors.New("failed") }); err == nil {
		t.Fatal("error hidden")
	}
	if get(t, l.INI) != original {
		t.Fatal("changed")
	}
	files, err := filepath.Glob(filepath.Join(filepath.Dir(l.INI), ".moderator-*"))
	if err != nil || len(files) != 0 {
		t.Fatal("temporary files leaked", files, err)
	}
}
func TestCrossInstanceLock(t *testing.T) {
	l, starts := fixture(t)
	unlock, err := lockFile(l.INI + ".moderator-lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := connect(l, nil); err == nil || *starts != 0 {
		t.Fatal("parallel operation accepted")
	}
	if err := unlock(); err != nil {
		t.Fatal(err)
	}
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
}

func TestRemoteViewerINI(t *testing.T) {
	for _, tail := range []string{
		"", "[RemoteViewerConnect]\n", "[RemoteViewerConnect]\nport=123\n",
		"[RemoteViewerConnect]\nbroker_addresses_list=old\n",
		"[RemoteViewerConnect]\n  port = 123 ; port comment\n broker_addresses_list = old # address comment\nkeep=yes\n",
	} {
		for _, eol := range []string{"\n", "\r\n"} {
			src := "\xef\xbb\xbf" + strings.ReplaceAll(original+tail, "\n", eol)
			out, ready, err := editINI([]byte(src), "10.254.32.206", "7777")
			if err != nil || ready {
				t.Fatalf("edit: ready=%v err=%v", ready, err)
			}
			text := string(out)
			if strings.Count(text, "[RemoteViewerConnect]") != 1 || !strings.Contains(text, "7777") || !strings.Contains(text, "10.254.32.206") {
				t.Fatalf("missing or duplicate connection parameters: %q", text)
			}
			if strings.Contains(tail, "; port comment") && !strings.Contains(text, "  port = 7777 ; port comment"+eol+" broker_addresses_list = 10.254.32.206 # address comment"+eol+"keep=yes") {
				t.Fatalf("formatting changed: %q", text)
			}
			out2, ready, err := editINI(out, "10.254.32.206", "7777")
			if err != nil || !ready || !bytes.Equal(out, out2) {
				t.Fatalf("repeat changed INI: %v", err)
			}
		}
	}
	for _, tail := range []string{
		"[RemoteViewerConnect]\nport=1\nport=2\n",
		"[RemoteViewerConnect]\nbroker_addresses_list=a\nbroker_addresses_list=b\n",
		"[RemoteViewerConnect]\nPORT=1\n", "[remoteviewerconnect]\nport=1\n",
	} {
		if _, _, err := editINI([]byte(original+tail), "host", "443"); err == nil {
			t.Fatalf("ambiguous connection accepted: %q", tail)
		}
	}
}

func TestConnectWritesVMParametersBeforeLaunch(t *testing.T) {
	l, _ := fixture(t)
	src := original + "[OtherConnection]\nport=123\nbroker_addresses_list=keep\n[RemoteViewerConnect]\nport=22\nbroker_addresses_list=old\n"
	put(t, l.INI, src)
	l.Start = func() error {
		text := get(t, l.INI)
		if !strings.Contains(text, "[RemoteViewerConnect]\nport=7777\nbroker_addresses_list=10.254.32.206\n") || !strings.Contains(text, "[OtherConnection]\nport=123\nbroker_addresses_list=keep\n") {
			t.Fatalf("wrong configuration at launch: %q", text)
		}
		return nil
	}
	if err := l.Connect(" 10.254.32.206 ", "7777", nil, quiet); err != nil {
		t.Fatal(err)
	}
	if get(t, l.INI+BackupSuffix) != src {
		t.Fatal("backup differs")
	}
	if err := l.Restore(nil, quiet); err != nil || get(t, l.INI) != src {
		t.Fatalf("connection parameters not restored: %v", err)
	}
}

func TestChangedVMParametersRequireConfirmation(t *testing.T) {
	l, starts := fixture(t)
	if err := connect(l, nil); err != nil {
		t.Fatal(err)
	}
	before := get(t, l.INI)
	c := approve(t, l.Connect("new.example", "7777", nil, quiet))
	if get(t, l.INI) != before || *starts != 1 {
		t.Fatal("unconfirmed connection changed files or launched")
	}
	// Approval must also match the requested address and port.
	c = approve(t, l.Connect("other.example", "8888", c, quiet))
	if get(t, l.INI) != before || *starts != 1 {
		t.Fatal("stale connection approval accepted")
	}
	if err := l.Connect("other.example", "8888", c, quiet); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(get(t, l.INI), "port=8888\nbroker_addresses_list=other.example\n") || get(t, l.INI+BackupSuffix) != original || *starts != 2 {
		t.Fatal("confirmed connection not applied or backup overwritten")
	}
}
