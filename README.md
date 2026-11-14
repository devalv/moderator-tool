# Moderator Launcher

Отдельная утилита Go/Fyne для Windows 11, Astra Linux 1.8 и RED OS 8 (amd64).
Подготавливает три параметра режима модератора и записывает адрес/порт ВМ из GUI
в `[RemoteViewerConnect]`: `broker_addresses_list` и `port`. Затем запускает Space Client
без аргументов. Логин и пароль вводятся в Space Client вручную.
В Launcher только два поля: адрес и порт ВМ. Они не сохраняются между запусками.
Пароль Launcher не запрашивает и не обрабатывает.

## Использование

1. Закройте Space Client самостоятельно.
2. Введите IP/hostname, при необходимости измените порт (по умолчанию `39897`, допустимы 1–65535), нажмите «Подключиться».
3. Введите логин и пароль в Space Client; адрес и порт уже записаны в INI.
4. Для отката закройте Space Client и нажмите «Вернуть исходные настройки».

На Linux запускайте Launcher и Space Client одним обычным пользователем, без `sudo`.
Используется INI из домашнего каталога этого пользователя.

Backup `client_settings.ini.moderator-backup` находится рядом с INI. Он не перезаписывается.
При расхождении настроек или смене адреса/порта ВМ требуется подтверждение. Откат восстанавливает весь исходный файл,
включая сторонние настройки из backup. При ошибке запуска автоматического отката нет.
Наличие дубликатов или неподдерживаемой кодировки блокирует операцию.

Пути:
- Windows INI: `%LOCALAPPDATA%\SpaceClient\client_settings.ini`.
- Windows запуск: `C:\Program Files\Space Client\space_client.exe`, рабочий каталог — каталог программы.
- Linux INI: `$HOME/.config/SpaceClient/client_settings.ini`.
- Linux запуск: `/opt/thin-client/start_vdi_client.sh`, без аргументов.

## Сборка

Для сборки Linux amd64 в контейнере нужен Docker и GNU Make:

```sh
make build-linux-astra
```

Команда использует базовый образ
`registry.astralinux.ru/library/astra/ubi18-golang121:1.8.5-mg16.2.0`,
устанавливает build-зависимости Fyne и создаёт `dist/Moderator-Launcher`.
Артефакт принадлежит пользователю, запустившему Make. Нужен доступ к registry,
APT-репозиториям и серверам загрузки Go toolchain/модулей: проект требует Go >= 1.22,
поэтому для Go 1.21 включён `GOTOOLCHAIN=auto`. При повторном запуске Docker
использует кеш слоя с системными пакетами; Go-кеш временный, внутри контейнера.
На хосте другой архитектуры требуется настроенная эмуляция Linux amd64 в Docker.
Работу GUI и runtime-библиотеки нужно отдельно проверить на целевых ОС.

Для сборки в RED OS 8:

```sh
make build-linux-redos
```

Используются `Dockerfile.redos` и образ `registry.red-soft.ru/ubi8/golang:1.26`.
Зависимости Fyne устанавливаются через `dnf`; артефакт —
`dist/redos/Moderator-Launcher`, отдельно от сборки Astra Linux.
Docker должен доверять TLS-сертификату registry; необходимые сертификаты
настраиваются на хосте администратором. Проверка TLS в команде сборки не отключается.
Состав графических dev-пакетов основан на [инструкции Fyne для dnf](https://docs.fyne.io/started/quick/);
их доступность в образе и работу GUI необходимо подтвердить сборкой и запуском на RED OS.

Go >= 1.22, Fyne 2.8.2, CGO и C-компилятор.

Для сборки Windows 11 amd64 в Linux-контейнере:

```sh
make build-win-11
```

`Dockerfile.windows` использует `golang:1.26-bookworm` и MinGW-w64 GCC.
CGO включён, целевая платформа — Windows amd64; флаг `-H=windowsgui`
убирает консольное окно. Артефакт: `dist/win-11/Moderator-Launcher.exe`.
Нужны Docker в режиме Linux-контейнеров, GNU Make, POSIX-shell и доступ к Docker Hub,
APT и серверам Go-модулей. Кросс-компиляция Fyne описана в
[официальной документации](https://docs.fyne.io/started/cross-compiling/).
Цель собирает EXE; выполнение Windows-тестов и ручная проверка GUI,
Space Client и восстановления выполняются отдельно на Windows 11.

Для сборки `.deb` для Astra Linux 1.8 amd64:

```sh
make build-deb VERSION=1.2.3
```

Цель сначала выполняет `build-linux-astra`, затем создаёт
`dist/moderator-launcher_1.2.3_amd64.deb`. Пакет устанавливает
`/usr/bin/moderator-launcher` и ярлык в меню приложений.
