# fileshare

Утилита для быстрой передачи файлов и текста по локальной сети или VPN. Написана на Go, без зависимостей — только стандартная библиотека.

---

## Возможности

- **Send** — раздача одного или нескольких файлов другому человеку
  - 1 файл → прямое скачивание
  - 2+ файла → ZIP-стрим на лету, без записи на диск
- **Receive** — приём файлов и текста через браузер
  - Drag & drop файлов и папок (с сохранением структуры директорий)
  - Кнопка «Select folder» — активна в Chromium-браузерах (Chrome, Edge, Opera); в остальных — серая с подсказкой
  - Множественный выбор файлов через пикер
- Передача текста в обоих направлениях
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

# Только приём
./fileshare --no-send

# Только отдача, файл задан сразу (headless)
./fileshare --no-receive --file /home/user/archive.tar.gz

# Несколько файлов в headless-режиме
./fileshare --no-receive --file /home/user/a.txt,/home/user/b.zip

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
| `--file` | — | Путь к файлу(ам) для раздачи, через запятую (headless) |
| `--no-send` | `false` | Отключить сервер отдачи |
| `--no-receive` | `false` | Отключить сервер приёма |

---

## Режим Send

Поднимает два HTTP-сервера:

| Сервер | Адрес | Для кого |
|---|---|---|
| Admin | `localhost:<admin-port>` | Ты — выбираешь файлы или вводишь текст |
| Client | `0.0.0.0:<send-port>` | Второй человек — скачивает |

**GUI-режим**: открой `http://localhost:8081` → загрузи файлы или введи текст → клиент увидит автоматически.

**Headless-режим** (`--file`): файлы сразу доступны после запуска, без открытия браузера. Несколько файлов — через запятую.

Поведение клиентской страницы:

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
- Кнопка **Select files** — множественный выбор, все браузеры
- Кнопка **Select folder** — выбор папки целиком; активна только в Chromium (Chrome, Edge, Opera), в остальных браузерах кнопка серая с tooltip-объяснением

**Text**
- Поле ввода + кнопка **Send text**
- Лог входящих сообщений с временны́ми метками и кнопкой **Copy** у каждого

Принятые файлы сохраняются в `--dir`. Вложенность папок сохраняется. При конфликте имён добавляется суффикс `_YYYY-MM-DD_HH-MM-SS`.

---

## Структура проекта

```
fileshare/
├── main.go
├── go.mod
└── templates/
    ├── send_admin.html    # Админка (localhost)
    ├── send_client.html   # Клиент скачивания
    └── receive.html       # Загрузка файлов и текста
```

---

## Заметки по безопасности

- Admin-сервер слушает **только на `127.0.0.1`** — недоступен снаружи
- Аутентификации нет — используй только в доверенных сетях (LAN, VPN)
- Имена файлов при сохранении обрабатываются через `filepath.Base` + `sanitizeRelPath` — защита от path traversal

---

---

# fileshare

A utility for fast file and text transfer over a local network or VPN. Written in Go, zero dependencies — standard library only.

---

## Features

- **Send** — share one or multiple files with another person
  - 1 file → direct download
  - 2+ files → on-the-fly ZIP stream, no temp file on disk
- **Receive** — accept files and text via browser
  - Drag & drop files and folders (preserves directory structure)
  - «Select folder» button — active in Chromium browsers (Chrome, Edge, Opera); disabled with tooltip in others
  - Multi-file picker
- Text sharing in both directions
- Incoming text message log with timestamps and per-message copy button
- Automatic client-side polling — page refreshes as soon as a file or text becomes available
- Graceful shutdown on `Ctrl+C` / `SIGTERM`
- Filename deduplication on save (`_YYYY-MM-DD_HH-MM-SS` suffix)
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

# Send only, headless
./fileshare --no-receive --file /home/user/archive.tar.gz

# Multiple files, headless
./fileshare --no-receive --file /home/user/a.txt,/home/user/b.zip

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
| `--file` | — | Comma-separated file paths to share (headless) |
| `--no-send` | `false` | Disable send server |
| `--no-receive` | `false` | Disable receive server |

---

## Send mode

Two HTTP servers:

| Server | Address | Who |
|---|---|---|
| Admin | `localhost:<admin-port>` | You — select files or enter text |
| Client | `0.0.0.0:<send-port>` | Other person — downloads |

**GUI mode**: open `http://localhost:8081` → upload files or type text → client sees it automatically.

**Headless mode** (`--file`): files are immediately available. Multiple files via comma: `--file a.txt,b.zip`.

Client page behaviour:

| Situation | Client sees |
|---|---|
| 1 file | Name, size, **Download** button |
| 2+ files | File list, total size, **Download all as ZIP** button |
| Text | Message, **Copy** button |
| Nothing | Waiting screen, auto-polls |

---

## Receive mode

Single HTTP server on `0.0.0.0:<receive-port>`.

The other person opens `http://<your_ip>:8082` and sees two tabs:

**Files**
- Drag & drop files and folders — directory structure preserved on disk
- **Select files** button — multi-select, all browsers
- **Select folder** button — picks entire folder; active only in Chromium (Chrome, Edge, Opera), disabled with tooltip in other browsers

**Text**
- Input field + **Send text** button
- Incoming message log with timestamps and per-message **Copy** button

Received files are saved to `--dir`. Folder structure is preserved. On name conflict a `_YYYY-MM-DD_HH-MM-SS` suffix is added.

---

## Project structure

```
fileshare/
├── main.go
├── go.mod
└── templates/
    ├── send_admin.html    # Admin UI (localhost)
    ├── send_client.html   # Download client page
    └── receive.html       # Upload page
```

---

## Security notes

- Admin server listens on **`127.0.0.1` only** — not reachable from outside
- No authentication — use only on trusted networks (LAN, VPN)
- Filenames are sanitized via `filepath.Base` + `sanitizeRelPath` to prevent path traversal
