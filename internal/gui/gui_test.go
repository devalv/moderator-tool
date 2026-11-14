package gui

import (
	"os"
	"path/filepath"
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/test"
	"fyne.io/fyne/v2/widget"
	launcher "github.com/devalv/moderator-tool/internal"
)

func TestInitialWindowAndValidation(t *testing.T) {
	a := test.NewApp()
	defer a.Quit()
	l := &launcher.Launcher{INI: filepath.Join(t.TempDir(), "client_settings.ini")}
	w := newWindow(a, l, nil)
	root := w.Content().(*fyne.Container)
	form := root.Objects[0].(*widget.Form)
	if len(form.Items) != 2 || form.Items[0].Text != "Адрес удалённой машины" || form.Items[1].Text != "Порт" {
		t.Fatal("expected only address and port fields")
	}
	if form.Items[0].Widget.(*widget.Entry).Text != "" || form.Items[1].Widget.(*widget.Entry).Text != defaultPort {
		t.Fatal("expected empty address and default port " + defaultPort)
	}
	if root.Objects[3].(*widget.Label).Alignment != fyne.TextAlignCenter {
		t.Fatal("status must be centered below the buttons")
	}
	connect, restore := root.Objects[1].(*widget.Button), root.Objects[2].(*widget.Button)
	if connect.Disabled() || !restore.Disabled() {
		t.Fatal("initial button state")
	}
	test.Tap(connect)
	if _, err := os.Stat(l.INI); !os.IsNotExist(err) {
		t.Fatal("invalid input touched INI")
	}
}
