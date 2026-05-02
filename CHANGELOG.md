# Changelog

## v1.0.0 — 2026-05-02

First public release.

### Overview

`fileshare` is a single self-contained binary for transferring files over a local network or VPN.
No cloud, no accounts, no dependencies beyond the Go standard library.

---

### Features

#### Unified binary — send & receive in one process

Both servers start by default. Use `--no-send` or `--no-receive` to run only one side.

#### Send mode

- **GUI mode** (default): open `http://localhost:8081` in a browser, pick a file via the native OS file dialog or drag & drop — the file is instantly available for the peer to download.
- **Headless mode** (`--file <path>`): the file is registered and ready before the browser is opened. Suitable for scripts and headless servers.
- Admin UI binds exclusively to `127.0.0.1` — never reachable by peers.
- Client download endpoint binds to `0.0.0.0` — reachable on all interfaces.
- Upload progress bar in the Admin UI (XHR `upload.onprogress`).
- Uploaded file is buffered in a temporary directory; the original path is never modified.

#### Receive mode

- Drag-and-drop upload page served on `0.0.0.0:8082`.
- Multiple files can be uploaded in a single request.
- Automatic timestamp-based deduplication: if `file.zip` already exists, it is saved as `file_2026-05-02_15-04-05.zip`.
- Configurable save directory via `--dir`.

#### Network

- All non-loopback IPv4 addresses are detected automatically and printed at startup.
- Graceful shutdown on `Ctrl+C` / `SIGTERM` with a 5-second drain timeout.

#### Cross-platform

Tested on Linux (amd64, arm/armv7), macOS (arm64), Windows (amd64).
Single `go build` with no CGO — works anywhere Go does.

#### Self-contained binary

All HTML templates are embedded via `//go:embed templates/*`.
The compiled binary requires no auxiliary files.

---

### Default ports

| Server | Address | Purpose |
|--------|---------|---------|
| Send — Admin | `127.0.0.1:8081` | File picker UI (you) |
| Send — Client | `0.0.0.0:8080` | Download page (peer) |
| Receive | `0.0.0.0:8082` | Upload page (peer) |

---

### Known limitations

- One file at a time in send mode. Selecting a new file replaces the previous one.
- No TLS. Intended for trusted LANs and VPNs (e.g. WireGuard). Do not expose to the public internet.
- No authentication. Anyone who can reach the client port can download the file.

