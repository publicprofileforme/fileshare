# fileshare

Утилита для быстрой передачи файлов и текста по локальной сети или VPN между двумя компьютерами. Написана на Go, без зависимостей — только стандартная библиотека.

---

## Возможности

- **Send** — отдача файла или текста другому человеку
- **Receive** — приём файлов и текстовых сообщений через браузер
- Оба сервера запускаются одновременно по умолчанию
- Drag & drop загрузка файлов
- Копирование текста в один клик (работает по `http://` на Windows)
- Автоматический поллинг на стороне клиента — страница обновляется как только файл или текст становится доступен
- Лог входящих текстовых сообщений с временны́ми метками
- Graceful shutdown по `Ctrl+C` / `SIGTERM`
- Дедупликация имён файлов при сохранении
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

### Кросс-компиляция

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

# Только приём файлов
./fileshare --no-send

# Только отдача, файл указан сразу (headless)
./fileshare --no-receive --file /home/user/archive.tar.gz

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
| `--file` | — | Путь к файлу для раздачи (headless-режим) |
| `--no-send` | `false` | Отключить сервер отдачи |
| `--no-receive` | `false` | Отключить сервер приёма |

---

## Режим Send

Поднимает два HTTP-сервера:

| Сервер | Адрес | Для кого |
|---|---|---|
| Admin | `localhost:<admin-port>` | Ты — выбираешь файл или вводишь текст |
| Client | `0.0.0.0:<send-port>` | Второй человек — скачивает или копирует |

**GUI-режим** (по умолчанию): открой `http://localhost:8081` → укажи путь к файлу или введи текст → клиент увидит его автоматически.

**Headless-режим** (`--file`): файл сразу доступен после запуска, без открытия браузера.

Клиентская страница автоматически опрашивает сервер и обновляется как только появляется файл или текст.

---

## Режим Receive

Поднимает один HTTP-сервер на `0.0.0.0:<receive-port>`.

Второй человек открывает `http://<твой_ip>:8082` и видит две вкладки:

- **Files** — drag & drop или выбор файлов, кнопка «Send files»
- **Text** — ввод текста, кнопка «Send text», лог входящих сообщений с кнопкой «Copy» у каждого

Принятые файлы сохраняются в `--dir`. Если файл с таким именем уже существует, к имени добавляется временна́я метка `_YYYY-MM-DD_HH-MM-SS`.

---

## Структура проекта

```
fileshare/
├── main.go
├── go.mod
└── templates/
    ├── send_admin.html    # Админка (localhost)
    ├── send_client.html   # Клиент скачивания
    └── receive.html       # Страница загрузки файлов и текста
```

---

## Заметки по безопасности

- Admin-сервер слушает **только на `127.0.0.1`** — недоступен снаружи
- Аутентификации нет — используй только в доверенных сетях (LAN, VPN)
- Имена файлов при сохранении обрабатываются через `filepath.Base` для защиты от path traversal

---

---

# fileshare

A utility for fast file and text transfer over a local network or VPN between two computers. Written in Go, zero dependencies — standard library only.

---

## Features

- **Send** — share a file or text with another person
- **Receive** — accept files and text messages via browser
- Both servers run simultaneously by default
- Drag & drop file upload
- One-click text copy (works over `http://` on Windows)
- Automatic client-side polling — page updates as soon as a file or text becomes available
- Incoming text message log with timestamps
- Graceful shutdown on `Ctrl+C` / `SIGTERM`
- Filename deduplication on save
- All templates embedded in the binary via `//go:embed` — single file, no runtime dependencies

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

### Cross-compilation

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
# Both modes (default)
./fileshare

# Receive only
./fileshare --no-send

# Send only, file specified immediately (headless)
./fileshare --no-receive --file /home/user/archive.tar.gz

# Custom ports
./fileshare --send-port 9090 --admin-port 9091 --receive-port 9092

# Custom save directory
./fileshare --dir /mnt/nas/inbox
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--send-port` | `8080` | Client download port (all interfaces) |
| `--admin-port` | `8081` | Send admin port (localhost only) |
| `--receive-port` | `8082` | File receive port (all interfaces) |
| `--dir` | `~/Downloads/Uploads` | Directory for received files |
| `--file` | — | File path to share (headless mode) |
| `--no-send` | `false` | Disable send server |
| `--no-receive` | `false` | Disable receive server |

---

## Send mode

Runs two HTTP servers:

| Server | Address | Who |
|---|---|---|
| Admin | `localhost:<admin-port>` | You — select file or enter text |
| Client | `0.0.0.0:<send-port>` | Other person — downloads or copies |

**GUI mode** (default): open `http://localhost:8081` → enter file path or type text → client sees it automatically.

**Headless mode** (`--file`): file is immediately available after launch, no browser needed.

The client page polls the server automatically and refreshes as soon as a file or text is available.

---

## Receive mode

Runs a single HTTP server on `0.0.0.0:<receive-port>`.

The other person opens `http://<your_ip>:8082` and sees two tabs:

- **Files** — drag & drop or file picker, «Send files» button
- **Text** — text input, «Send text» button, incoming message log with «Copy» button on each entry

Received files are saved to `--dir`. If a file with the same name already exists, a timestamp `_YYYY-MM-DD_HH-MM-SS` is appended.

---

## Project structure

```
fileshare/
├── main.go
├── go.mod
└── templates/
    ├── send_admin.html    # Admin UI (localhost)
    ├── send_client.html   # Download client page
    └── receive.html       # File and text upload page
```

---

## Security notes

- Admin server listens on **`127.0.0.1` only** — not accessible from outside
- No authentication — use only on trusted networks (LAN, VPN)
- Filenames are sanitized with `filepath.Base` on save to prevent path traversal

# Screenshots
![fileshare download](screenshots\fileshare-download.png?raw=true "fileshare download")
![fileshare receive](screenshots\fileshare-receive.png?raw=true "fileshare receive")
![fileshare send1](screenshots\fileshare-send1.png?raw=true "fileshare send1")
![fileshare send2](screenshots\fileshare-send2.png?raw=true "fileshare send2")