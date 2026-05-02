# fileshare

Лёгкая утилита для передачи файлов по локальной сети или VPN — без облаков, без регистрации.  
Запускаешь у себя, второй человек открывает браузер.

> **Lightweight peer-to-peer file transfer over LAN or VPN — no cloud, no signup.**  
> Run it locally, the other person opens a browser.

---

## Оглавление / Table of Contents

- [Русский](#русский)
- [English](#english)

---

# Русский

## Требования

- Go 1.21+
- Только стандартная библиотека — никаких зависимостей

## Сборка

```bash
git clone https://github.com/publicprofileforme/fileshare.git
cd fileshare
go build -o fileshare .
```

### Кросс-компиляция

```bash
# Windows (64-bit)
GOOS=windows GOARCH=amd64 go build -o fileshare.exe .

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o fileshare-mac-arm .

# Linux ARM (Raspberry Pi / Orange Pi)
GOOS=linux GOARCH=arm GOARM=7 go build -o fileshare-linux-arm .

# Linux x86-64
GOOS=linux GOARCH=amd64 go build -o fileshare-linux-amd64 .
```

## Запуск

```bash
# Оба режима (отдача + приём) — по умолчанию
./fileshare

# Только приём файлов
./fileshare --no-send

# Только отдача, headless (файл сразу готов)
./fileshare --no-receive --file /home/user/archive.tar.gz

# Свои порты
./fileshare --send-port 9090 --admin-port 9091 --receive-port 9092

# Своя папка для принятых файлов
./fileshare --dir /mnt/nas/inbox
```

## Флаги

| Флаг | По умолчанию | Описание |
|------|-------------|---------|
| `--send-port` | `8080` | Порт для клиента (скачивание файла) |
| `--admin-port` | `8081` | Порт Admin UI (только localhost) |
| `--receive-port` | `8082` | Порт для загрузки файлов от собеседника |
| `--dir` | `~/Downloads/Uploads` | Папка для сохранения принятых файлов |
| `--file` | — | Путь к файлу → headless-режим отдачи |
| `--no-send` | `false` | Отключить сервер отдачи |
| `--no-receive` | `false` | Отключить сервер приёма |

## Как это работает

### Отдача файла (send)

Поднимаются два сервера:

- **Admin** (`localhost:8081`) — только для тебя. Открой в браузере, выбери файл через файловый менеджер браузера или перетащи. Файл загрузится на сервер и станет доступен клиенту.
- **Client** (`0.0.0.0:8080`) — для собеседника. Открывает страницу с кнопкой **Download**.

```
Ты                           Собеседник
────────────────────         ─────────────────────────
localhost:8081               http://<твой_ip>:8080
  ↓ выбрал файл                ↓ нажал Download
  файл сохранён во             ← получил файл
  tmp и зарегистрирован
```

**GUI-режим** (по умолчанию): открой `http://localhost:8081`, выбери файл — он станет доступен.  
**Headless-режим** (`--file`): файл готов к раздаче сразу после запуска, без открытия браузера.

### Приём файлов (receive)

Один сервер на `0.0.0.0:8082`. Собеседник открывает `http://<твой_ip>:8082`, выбирает файлы или перетаскивает, нажимает **Send files**. Файлы сохраняются в `--dir`.

При совпадении имён к файлу автоматически добавляется временна́я метка: `file_2026-05-02_13-00-00.zip`.

## Вывод при запуске

```
  fileshare started!
  ──────────────────────────────────────────────
  [SEND]  Mode         : GUI
          Admin (you)  : http://localhost:8081
          Client       : http://10.0.0.2:8080
  ──────────────────────────────────────────────
  [RECV]  Upload URL   : http://10.0.0.2:8082
          Localhost    : http://localhost:8082
          Save dir     : /home/user/Downloads/Uploads
  ──────────────────────────────────────────────
  Stop: Ctrl+C
```

## Структура проекта

```
fileshare/
├── main.go               # Весь серверный код
├── go.mod
└── templates/
    ├── send_admin.html   # Admin UI (выбор файла)
    ├── send_client.html  # Страница скачивания
    └── receive.html      # Страница загрузки файлов
```

Все шаблоны вшиты в бинарь через `//go:embed` — итоговый исполняемый файл полностью самодостаточен.

---

# English

## Requirements

- Go 1.21+
- Standard library only — zero external dependencies

## Build

```bash
git clone https://github.com/publicprofileforme/fileshare.git
cd fileshare
go build -o fileshare .
```

### Cross-compilation

```bash
# Windows (64-bit)
GOOS=windows GOARCH=amd64 go build -o fileshare.exe .

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -o fileshare-mac-arm .

# Linux ARM (Raspberry Pi / Orange Pi)
GOOS=linux GOARCH=arm GOARM=7 go build -o fileshare-linux-arm .

# Linux x86-64
GOOS=linux GOARCH=amd64 go build -o fileshare-linux-amd64 .
```

## Usage

```bash
# Both modes (send + receive) — default
./fileshare

# Receive only
./fileshare --no-send

# Send only, headless (file ready immediately)
./fileshare --no-receive --file /home/user/archive.tar.gz

# Custom ports
./fileshare --send-port 9090 --admin-port 9091 --receive-port 9092

# Custom receive directory
./fileshare --dir /mnt/nas/inbox
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--send-port` | `8080` | Client-facing download port |
| `--admin-port` | `8081` | Admin UI port (localhost only) |
| `--receive-port` | `8082` | Upload port for incoming files |
| `--dir` | `~/Downloads/Uploads` | Directory to save received files |
| `--file` | — | File path → headless send mode |
| `--no-send` | `false` | Disable send server |
| `--no-receive` | `false` | Disable receive server |

## How It Works

### Sending a file

Two servers are started:

- **Admin** (`localhost:8081`) — your local UI. Open in a browser, pick a file via the native file dialog or drag & drop. The file is uploaded to the server and made available to the client.
- **Client** (`0.0.0.0:8080`) — the peer's endpoint. Shows a **Download** button.

```
You                          Peer
────────────────────         ─────────────────────────
localhost:8081               http://<your_ip>:8080
  ↓ picked a file              ↓ clicked Download
  file stored in tmp           ← received the file
  and registered
```

**GUI mode** (default): open `http://localhost:8081`, pick a file — it becomes available immediately.  
**Headless mode** (`--file`): the file is ready as soon as the process starts, no browser needed.

### Receiving files

A single server on `0.0.0.0:8082`. The peer opens `http://<your_ip>:8082`, selects or drags files, clicks **Send files**. Files are saved to `--dir`.

Name collisions are resolved automatically by appending a timestamp: `file_2026-05-02_13-00-00.zip`.

## Startup Output

```
  fileshare started!
  ──────────────────────────────────────────────
  [SEND]  Mode         : GUI
          Admin (you)  : http://localhost:8081
          Client       : http://10.0.0.2:8080
  ──────────────────────────────────────────────
  [RECV]  Upload URL   : http://10.0.0.2:8082
          Localhost    : http://localhost:8082
          Save dir     : /home/user/Downloads/Uploads
  ──────────────────────────────────────────────
  Stop: Ctrl+C
```

## Project Structure

```
fileshare/
├── main.go               # All server logic
├── go.mod
└── templates/
    ├── send_admin.html   # Admin UI (file picker)
    ├── send_client.html  # Download page
    └── receive.html      # Upload page
```

All HTML templates are embedded into the binary via `//go:embed` — the resulting executable is fully self-contained, no extra files needed.

## Security Notes

- The Admin server binds exclusively to `127.0.0.1` — it is never reachable by peers.
- No authentication is implemented — intended for use on trusted LANs or VPNs (e.g. WireGuard).
- Each file upload overwrites the previous selection; only one file can be served at a time.

## License

MIT
