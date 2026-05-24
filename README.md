# fileshare

Утилита для быстрой передачи файлов и текста по локальной сети или VPN. Написана на Go, без зависимостей — только стандартная библиотека.

---

## Возможности

- **Send** — раздача одного или нескольких файлов другому человеку
  - 1 файл → прямое скачивание
  - 2+ файла → ZIP-стрим на лету, без записи на диск
- **Receive** — приём файлов и текста через браузер
  - Drag & drop файлов и папок (с сохранением структуры директорий)
  - Кнопка «Select folder» — активна в Chromium (Chrome, Edge, Opera); в остальных браузерах серая с tooltip
  - Множественный выбор файлов через пикер
- Передача текста в обоих направлениях
- **Опциональная парольная защита** клиентского и receive-серверов
- Лог входящих текстовых сообщений с временны́ми метками и кнопкой «Copy»
- Автоматический поллинг на клиенте — страница обновляется как только файл или текст становится доступен
- Graceful shutdown по `Ctrl+C` / `SIGTERM`
- Дедупликация имён файлов при сохранении (суффикс `_YYYY-MM-DD_HH-MM-SS`)
- Все шаблоны вшиты в бинарь через `//go:embed` — один файл, никаких зависимостей

---

## Требования

- Go 1.21+
- Только стандартная библиотека

---

## Сборка

```bash
git clone https://github.com/you/fileshare
cd fileshare
go build -o fileshare .
```

### Сборка через Make

```bash
make all          # все платформы
make windows      # Windows amd64
make linux-amd64
make linux-arm    # ARMv7 (Raspberry Pi и др.)
make mac-arm      # Apple Silicon
make mac-x86      # Intel Mac
make clean        # удалить dist/
```

Бинари кладутся в `dist/`. Версия берётся из `git describe --tags` и вшивается через `-ldflags`.

### Кросс-компиляция вручную

| Платформа | Команда |
|---|---|
| Windows x64 | `GOOS=windows GOARCH=amd64 go build -o fileshare.exe .` |
| macOS Apple Silicon | `GOOS=darwin GOARCH=arm64 go build -o fileshare-mac-arm .` |
| macOS Intel | `GOOS=darwin GOARCH=amd64 go build -o fileshare-mac-x64 .` |
| Linux x64 | `GOOS=linux GOARCH=amd64 go build -o fileshare-linux-x64 .` |
| Linux ARM (Raspberry Pi) | `GOOS=linux GOARCH=arm GOARM=7 go build -o fileshare-linux-arm .` |
| Linux ARM64 (Orange Pi) | `GOOS=linux GOARCH=arm64 go build -o fileshare-linux-arm64 .` |

---

## Запуск

```bash
# Оба режима (по умолчанию)
./fileshare

# Только приём
./fileshare --no-send

# Только отдача, headless (один файл)
./fileshare --no-receive --file /home/user/archive.tar.gz

# Несколько файлов в headless-режиме
./fileshare --no-receive --file /home/user/a.txt,/home/user/b.zip

# С паролем (через флаг)
./fileshare --password mysecret

# С паролем (через переменную окружения)
FILESHARE_PASSWORD=mysecret ./fileshare

# Свои порты
./fileshare --send-port 9090 --admin-port 9091 --receive-port 9092

# Своя папка для принятых файлов
./fileshare --dir /mnt/nas/inbox
```

### Флаги

| Флаг | По умолчанию | Описание |
|---|---|---|
| `--send-port` | `8080` | Порт клиента для скачивания (все интерфейсы) |
| `--admin-port` | `8081` | Порт админки send (только localhost) |
| `--receive-port` | `8082` | Порт приёма файлов (все интерфейсы) |
| `--dir` | `~/Downloads/Uploads` | Папка для принятых файлов |
| `--file` | — | Пути к файлам через запятую (headless) |
| `--password` | — | Пароль для клиентских страниц (или `FILESHARE_PASSWORD`) |
| `--no-send` | `false` | Отключить сервер отдачи |
| `--no-receive` | `false` | Отключить сервер приёма |

---

## Авторизация

По умолчанию пароль не задан и всё открыто. Если задать пароль — клиентский и receive-серверы потребуют входа через форму логина. Сессия хранится в cookie и не требует повторного ввода пароля.

**Admin-сервер (`localhost`) никогда не защищается паролем** — он и так недоступен снаружи.

```bash
# через флаг
./fileshare --password secret

# через env (удобно для systemd/Docker)
FILESHARE_PASSWORD=secret ./fileshare
```

При запуске с паролем в баннере:
```
  [AUTH]  Password protection : ON
          Admin (localhost)   : no password
```

---

## Режим Send

Поднимает два HTTP-сервера:

| Сервер | Адрес | Для кого |
|---|---|---|
| Admin | `localhost:<admin-port>` | Ты — выбираешь файлы или вводишь текст |
| Client | `0.0.0.0:<send-port>` | Второй человек — скачивает |

**GUI-режим**: открой `http://localhost:8081` → загрузи файлы или введи текст → клиент увидит автоматически.

**Headless-режим** (`--file`): файлы сразу доступны после запуска. Несколько файлов — через запятую.

| Ситуация | Что видит клиент |
|---|---|
| 1 файл | Имя, размер, кнопка **Download** |
| 2+ файла | Список файлов, общий размер, кнопка **Download all as ZIP** |
| Текст | Сообщение, кнопка **Copy** |
| Ничего | Экран ожидания, автообновление |

---

## Режим Receive

Поднимает один HTTP-сервер на `0.0.0.0:<receive-port>`.

Второй человек открывает `http://<твой_ip>:8082` и видит две вкладки:

**Files**
- Drag & drop файлов и папок — структура папки сохраняется на диске
- **Select files** — множественный выбор, все браузеры
- **Select folder** — выбор папки целиком; активна только в Chromium (Chrome, Edge, Opera), в остальных — серая с tooltip

**Text**
- Поле ввода + кнопка **Send text**
- Лог входящих сообщений с временны́ми метками и кнопкой **Copy**

---

## Структура проекта

```
fileshare/
├── main.go
├── go.mod
├── Makefile
└── templates/
    ├── send_admin.html    # Админка (localhost)
    ├── send_client.html   # Клиент скачивания
    ├── receive.html       # Загрузка файлов и текста
    └── login.html         # Форма логина (при включённом пароле)
```

---

## Заметки по безопасности

- Admin-сервер слушает **только на `127.0.0.1`** — недоступен снаружи, пароль не нужен
- Пароль сравнивается через `crypto/subtle.ConstantTimeCompare` — защита от timing-атак
- Сессионный токен генерируется через `crypto/rand`
- Аутентификации по HTTPS нет — используй только в доверенных сетях (LAN, VPN) или за reverse proxy с TLS
- Имена файлов при сохранении обрабатываются через `filepath.Base` + `sanitizeRelPath` — защита от path traversal

---

---

# fileshare

A utility for fast file and text transfer over a local network or VPN. Written in Go, zero dependencies — standard library only.

---

## Features

- **Send** — share one or multiple files
  - 1 file → direct download
  - 2+ files → on-the-fly ZIP stream, no temp file on disk
- **Receive** — accept files and text via browser
  - Drag & drop files and folders (preserves directory structure)
  - «Select folder» button — active in Chromium (Chrome, Edge, Opera); disabled with tooltip elsewhere
  - Multi-file picker
- Text sharing in both directions
- **Optional password protection** for client and receive servers
- Incoming text message log with timestamps and per-message copy button
- Automatic client-side polling
- Graceful shutdown on `Ctrl+C` / `SIGTERM`
- Filename deduplication (`_YYYY-MM-DD_HH-MM-SS` suffix)
- All templates embedded via `//go:embed` — single binary, no runtime dependencies

---

## Requirements

- Go 1.21+
- Standard library only

---

## Build

```bash
git clone https://github.com/you/fileshare
cd fileshare
go build -o fileshare .
```

### Build with Make

```bash
make all          # all platforms
make windows
make linux-amd64
make linux-arm
make mac-arm
make mac-x86
make clean
```

Binaries are placed in `dist/`. Version is injected from `git describe --tags` via `-ldflags`.

### Manual cross-compilation

| Platform | Command |
|---|---|
| Windows x64 | `GOOS=windows GOARCH=amd64 go build -o fileshare.exe .` |
| macOS Apple Silicon | `GOOS=darwin GOARCH=arm64 go build -o fileshare-mac-arm .` |
| macOS Intel | `GOOS=darwin GOARCH=amd64 go build -o fileshare-mac-x64 .` |
| Linux x64 | `GOOS=linux GOARCH=amd64 go build -o fileshare-linux-x64 .` |
| Linux ARM (Raspberry Pi) | `GOOS=linux GOARCH=arm GOARM=7 go build -o fileshare-linux-arm .` |
| Linux ARM64 (Orange Pi) | `GOOS=linux GOARCH=arm64 go build -o fileshare-linux-arm64 .` |

---

## Usage

```bash
./fileshare                                        # both modes
./fileshare --no-send                              # receive only
./fileshare --no-receive --file archive.tar.gz     # send only, headless
./fileshare --no-receive --file a.txt,b.zip        # multiple files
./fileshare --password mysecret                    # with password
FILESHARE_PASSWORD=mysecret ./fileshare            # via env
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--send-port` | `8080` | Client download port (all interfaces) |
| `--admin-port` | `8081` | Send admin port (localhost only) |
| `--receive-port` | `8082` | File receive port (all interfaces) |
| `--dir` | `~/Downloads/Uploads` | Directory for received files |
| `--file` | — | Comma-separated file paths (headless) |
| `--password` | — | Password for client pages (or `FILESHARE_PASSWORD`) |
| `--no-send` | `false` | Disable send server |
| `--no-receive` | `false` | Disable receive server |

---

## Authentication

Disabled by default. When a password is set, client and receive servers require login via a form. Session is stored in a cookie — no repeated prompts.

**Admin server (localhost) is never password-protected.**

```bash
./fileshare --password secret
FILESHARE_PASSWORD=secret ./fileshare
```

---

## Send mode

| Server | Address | Who |
|---|---|---|
| Admin | `localhost:<admin-port>` | You — select files or enter text |
| Client | `0.0.0.0:<send-port>` | Other person — downloads |

**GUI mode**: open `http://localhost:8081`, upload files or type text.
**Headless mode** (`--file`): files available immediately. Multiple files via comma.

| Situation | Client sees |
|---|---|
| 1 file | Name, size, **Download** button |
| 2+ files | File list, total size, **Download all as ZIP** |
| Text | Message, **Copy** button |
| Nothing | Waiting screen, auto-polls |

---

## Receive mode

Single HTTP server on `0.0.0.0:<receive-port>`.

**Files tab**: drag & drop files/folders (structure preserved), multi-select picker, folder picker (Chromium only — disabled with tooltip in other browsers).

**Text tab**: send text to host, view incoming message log with copy buttons.

---

## Project structure

```
fileshare/
├── main.go
├── go.mod
├── Makefile
└── templates/
    ├── send_admin.html
    ├── send_client.html
    ├── receive.html
    └── login.html
```

---

## Security notes

- Admin listens on **`127.0.0.1` only** — never password-protected
- Password compared via `crypto/subtle.ConstantTimeCompare` — timing-attack safe
- Session token generated with `crypto/rand`
- No HTTPS — use on trusted networks (LAN, VPN) or behind a TLS reverse proxy
- Filenames sanitized via `filepath.Base` + `sanitizeRelPath` against path traversal
