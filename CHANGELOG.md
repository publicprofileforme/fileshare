# CHANGELOG

All notable changes to this project are documented here.
Format: [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)

---

## [1.3.0] — 2026-05-24

### Added
- **Optional password protection** for client (send) and receive servers
  - Set via `--password` flag or `FILESHARE_PASSWORD` environment variable
  - Disabled by default — behaviour unchanged when not set
  - Session stored in `fs_session` cookie (48-char random hex token via `crypto/rand`)
  - Password comparison via `crypto/subtle.ConstantTimeCompare` — timing-attack safe
  - Admin server (localhost only) is **never** password-protected
  - On startup with password set, banner shows `[AUTH] Password protection : ON`
- **Login page** (`templates/login.html`) — minimal form in the same visual style, with error message on wrong password and `next` redirect after successful login
- **`Makefile`** for cross-compilation:
  - Targets: `windows`, `linux-amd64`, `linux-arm`, `mac-arm`, `mac-x86`, `all`, `clean`
  - Output goes to `dist/`
  - Version injected via `git describe --tags` → `-ldflags -X main.version=...`

---

## [1.2.0] — 2026-05-23

### Added — Send
- **Multi-file send**: admin UI now accepts multiple files via drag & drop or file picker
- **On-the-fly ZIP**: when 2+ files are selected, client receives a single ZIP streamed directly from memory — no temp file written to disk (`/download-zip` endpoint, `archive/zip`)
- Single file is still served directly without any archive
- Admin shows file list with individual sizes and total; client mirrors the same list
- `--file` flag now accepts comma-separated paths: `--file a.txt,b.zip,c.pdf`
- New API endpoints: `POST /api/upload-files`, `GET /download/<index>`, `GET /download-zip`, `POST /api/clear`

### Added — Receive
- **Folder upload via drag & drop**: uses `DataTransferItem.webkitGetAsEntry()` to recursively traverse dropped directories; folder structure preserved on disk
- **Select folder button**: uses `<input webkitdirectory>` for Chromium-based browsers (Chrome, Edge, Opera)
- On non-Chromium browsers (Firefox, Safari) the button is **disabled** (greyed out) with a tooltip: *"Folder selection requires a Chromium-based browser"*
- Relative paths sent as a separate `relpaths[]` form field alongside `files[]`
- Server creates parent directories with `os.MkdirAll` to reconstruct folder tree under `--dir`
- Upload response changed from plain text to JSON: `{"saved": ["path/to/file", ...]}`

### Changed
- `sendState.files` is now `[]sendFileEntry` instead of a single entry
- `sendSnapshot` struct added to avoid holding the lock during template rendering
- `sanitizeRelPath()` helper added — strips `..` components and normalises separators
- Admin page reloads after successful upload/text-set instead of partial DOM update

---

## [1.1.0] — 2026-05-02

### Added
- **Text sharing** in both directions
  - Send admin: «Text» tab with textarea and «Share text» button
  - Send client: displays received text with one-click copy
  - Receive UI: «Text» tab — type text and send to the host
  - Incoming text log with timestamps and per-message copy button
  - Client page auto-polls and reloads when text or file becomes available

### Fixed
- **Copy to clipboard on Windows over `http://`** — `navigator.clipboard` (Secure Context API) blocked on non-`localhost` HTTP. Replaced with `execCommand('copy')` fallback using an off-screen `<textarea>`
- **Copy buttons in receive message log** — broken by `JSON.stringify` inside inline `onclick` for text with quotes/angle brackets/newlines. Replaced with `data-idx` attributes + `addEventListener` after DOM render

### Changed
- `sendState.mode` field (`idle` / `file` / `text`) replaces boolean `ready` flag
- New HTTP endpoints: `GET /api/status`, `POST /api/set-text`, `POST /send-text`, `GET /api/texts`

---

## [1.0.0] — 2026-05-02

Initial release. Rewrite of `send.py` + `receive.py` into a single Go binary.

### Features
- Single binary, zero runtime dependencies (`//go:embed` for HTML templates)
- **Send mode**: Admin UI (localhost only) + Client server (all interfaces)
  - GUI mode: select file by path via browser
  - Headless mode: `--file` flag
- **Receive mode**: drag & drop file upload, saves to configurable `--dir`
- Both modes run simultaneously by default; disable either with `--no-send` / `--no-receive`
- Auto-detects local non-loopback IPv4 addresses, prints them on startup
- Filename deduplication with `_YYYY-MM-DD_HH-MM-SS` suffix on collision
- Graceful shutdown (`Ctrl+C` / `SIGTERM`) with 5-second timeout
- Cross-platform: Linux (x64, ARM, ARM64), macOS (Intel, Apple Silicon), Windows
