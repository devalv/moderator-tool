package gui

import (
	"errors"
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	launcher "github.com/devalv/moderator-tool/internal"
)

const defaultPort = "39897"

func Run() {
	a := app.New()
	l, err := launcher.New()
	newWindow(a, l, err).ShowAndRun()
}

func newWindow(a fyne.App, l *launcher.Launcher, initErr error) fyne.Window {
	w := a.NewWindow("GLINT Moderator Launcher")
	address, port := widget.NewEntry(), widget.NewEntry()
	port.SetText(defaultPort)
	status := widget.NewLabel("Введите параметры подключения")
	status.Alignment = fyne.TextAlignCenter
	status.Wrapping = fyne.TextWrapWord
	connect, restore := widget.NewButton("Подключиться", nil), widget.NewButton("Вернуть исходные настройки", nil)
	busy := false
	refresh := func() {
		if busy || initErr != nil {
			connect.Disable()
			restore.Disable()
			return
		}
		connect.Enable()
		exists, err := l.BackupExists()
		if err != nil {
			status.SetText(fmt.Sprintf("Не удалось проверить backup: %v", err))
		}
		if exists {
			restore.Enable()
		} else {
			restore.Disable()
		}
	}
	var run func(bool, *launcher.Confirmation)
	run = func(rollback bool, approval *launcher.Confirmation) {
		if busy {
			return
		}
		if !rollback {
			if err := launcher.Validate(address.Text, port.Text); err != nil {
				dialog.ShowError(err, w)
				return
			}
		}
		busy = true
		refresh()
		address.Disable()
		port.Disable()
		host, service := address.Text, port.Text
		go func() {
			update := func(s string) { fyne.Do(func() { status.SetText(s) }) }
			var err error
			if rollback {
				err = l.Restore(approval, update)
			} else {
				err = l.Connect(host, service, approval, update)
			}
			fyne.Do(func() {
				busy = false
				address.Enable()
				port.Enable()
				refresh()
				var confirmation *launcher.Confirmation
				if errors.As(err, &confirmation) {
					busy = true
					refresh()
					message := "Backup уже существует, а текущие настройки отличаются от режима модератора. Применить режим модератора и указанные адрес и порт ВМ?"
					if rollback {
						message = "Текущие настройки отличаются от режима модератора. Откат заменит весь INI содержимым backup, включая последующие изменения. Продолжить?"
					}
					d := dialog.NewConfirm("Подтверждение", message, func(ok bool) {
						busy = false
						refresh()
						if ok {
							run(rollback, confirmation)
						} else {
							status.SetText("Операция отменена")
						}
					}, w)
					d.SetConfirmText("Продолжить")
					d.SetDismissText("Отмена")
					d.Show()
				} else if err != nil {
					status.SetText("Ошибка")
					dialog.ShowError(err, w)
				} else if rollback {
					status.SetText("Исходные настройки восстановлены")
				} else {
					status.SetText("Space Client запущен. Адрес и порт ВМ сохранены. Введите логин и пароль в его интерфейсе вручную")
				}
			})
		}()
	}
	connect.OnTapped = func() { run(false, nil) }
	restore.OnTapped = func() { run(true, nil) }
	w.SetCloseIntercept(func() {
		if busy {
			dialog.ShowInformation("Операция выполняется", "Дождитесь завершения работы с файлами", w)
		} else {
			w.Close()
		}
	})
	w.SetContent(container.NewVBox(widget.NewForm(
		widget.NewFormItem("Адрес удалённой машины", address),
		widget.NewFormItem("Порт", port)), connect, restore, status))
	w.Resize(fyne.NewSize(620, 310))
	refresh()
	if initErr != nil {
		dialog.ShowError(initErr, w)
	}
	return w
}
