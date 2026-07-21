# Getting Started

This guide covers development setup, production builds, first launch, runtime configuration, persistent storage, Docker, and backup/restore.

## Requirements

Development and local release builds use:

- Node.js 22
- npm with the checked-in lockfile
- Go 1.25
- A Gemini API key with access to the configured models

A released single binary or Docker image does not require Node.js or Go at runtime.

## Clone and install

```bash
git clone https://github.com/ajbergh/Gemini-Voice-Studio.git
cd Gemini-Voice-Studio
npm ci
```

`npm ci` is preferred over `npm install` because CI and release workflows use the lockfile exactly.

## Development mode

Start the Go backend in one terminal:

```bash
cd backend
go run ./cmd/server --open=false
```

On Windows, the repository also includes:

```powershell
.\scripts\start-backend-dev.ps1
```

Start the Vite frontend in a second terminal:

```bash
npm run dev
```

The frontend listens on `http://localhost:4000` and proxies `/api` requests to the backend at `http://localhost:8080`.

## First launch

1. Open `http://localhost:4000` in development or `http://localhost:8080` from a production binary.
2. Open **Settings → API Keys**.
3. Paste a Gemini API key.
4. Test the key.
5. Save it.

The browser never stores the plaintext key. The backend encrypts it before writing it to SQLite.

## Supported TTS models

The backend provider registry is the source of truth:

| Model | Streaming | Description |
|---|---:|---|
| `gemini-3.1-flash-tts-preview` | Yes | Default low-latency expressive generation |
| `gemini-2.5-flash-preview-tts` | No | Cost-efficient high-volume generation |
| `gemini-2.5-pro-preview-tts` | No | Highest-fidelity long-form generation |

Unknown model identifiers are rejected rather than silently substituted.

## Validate a checkout

Frontend:

```bash
npm run typecheck
npm run build
npm run test:e2e
```

Backend:

```bash
cd backend
go vet ./...
go test ./...
go test -race ./...
```

## Build a single binary

The platform scripts run a deterministic frontend install, TypeScript validation, Vite build, frontend embedding, and metadata-stamped Go build.

### Windows

```powershell
.\scripts\build-windows.ps1
.\scripts\build-windows.ps1 -Arch arm64
.\scripts\build-windows.ps1 -Clean
```

### Linux

```bash
chmod +x scripts/build-linux.sh
./scripts/build-linux.sh
./scripts/build-linux.sh --arch arm64
./scripts/build-linux.sh --clean
```

### macOS

```bash
chmod +x scripts/build-macos.sh
./scripts/build-macos.sh
./scripts/build-macos.sh --arch amd64
./scripts/build-macos.sh --universal
./scripts/build-macos.sh --clean
```

Artifacts are written to `bin/` as `gemini-voice-studio-<os>-<arch>` with `.exe` on Windows.

Run a binary:

```bash
./bin/gemini-voice-studio-linux-amd64 --port 8080
```

Inspect its embedded build metadata:

```bash
./bin/gemini-voice-studio-linux-amd64 --version
```

## Runtime configuration

Configuration precedence is:

1. Platform defaults
2. Optional JSON configuration file
3. `GVS_*` environment variables
4. Explicit CLI flags

### CLI flags

| Flag | Description |
|---|---|
| `--config PATH` | Optional JSON configuration file |
| `--port N` | HTTP server port |
| `--data-dir PATH` | Persistent application root |
| `--db PATH` | Explicit SQLite path |
| `--audio-dir PATH` | Explicit generated-media directory |
| `--passphrase VALUE` | Encryption passphrase |
| `--log-level LEVEL` | `debug`, `info`, `warn`, or `error` |
| `--open=true|false` | Open the browser on startup |
| `--version` | Print version information and exit |

### Environment variables

| Variable | Meaning |
|---|---|
| `GVS_CONFIG` | JSON configuration path |
| `GVS_PORT` | HTTP port |
| `GVS_DATA_DIR` | Persistent application root |
| `GVS_DB_PATH` | SQLite path |
| `GVS_AUDIO_DIR` | Generated-media directory |
| `GVS_PASSPHRASE` | Encryption passphrase |
| `GVS_LOG_LEVEL` | Log level |
| `GVS_OPEN_BROWSER` | Boolean browser-open setting |

Example JSON file:

```json
{
  "port": 8080,
  "data_dir": "/srv/gemini-voice-studio",
  "db_path": "/srv/gemini-voice-studio/data.db",
  "audio_cache_dir": "/srv/gemini-voice-studio/audio",
  "log_level": "info",
  "open_browser": false
}
```

The passphrase is not serialized. Supply it through `GVS_PASSPHRASE` or `--passphrase`.

## Default storage locations

| Platform | Data directory |
|---|---|
| Windows | `%APPDATA%\gemini-voice-studio` |
| macOS | `~/Library/Application Support/gemini-voice-studio` |
| Linux | `$XDG_DATA_HOME/gemini-voice-studio` or `~/.local/share/gemini-voice-studio` |

The directory contains the SQLite database and generated media. Existing `gemini-voice-library` and `audio_cache` directories are reused when present and no new-name directory exists.

Do not copy only `data.db` when migrating an installation. The media directory contains durable audio and the installation salt used by encrypted API-key rows.

## Docker

```bash
docker compose up --build
```

The Compose configuration:

- Publishes port `8080`.
- Mounts a named volume at `/home/app/data`.
- Runs the application as a non-root user.
- Disables browser opening.
- Uses supported `GVS_*` environment variables.
- Includes an HTTP health check.

Example:

```bash
GVS_PASSPHRASE='replace-with-a-secret' docker compose up --build
```

## Backup and restore

Create a portable backup:

```bash
curl -X POST http://localhost:8080/api/backup \
  --output gemini-voice-studio.gvsbackup
```

Restore it:

```bash
curl -X POST http://localhost:8080/api/restore \
  -F "backup=@gemini-voice-studio.gvsbackup"
```

A portable archive contains:

- A consistent SQLite snapshot
- Durable generated media
- Encryption metadata
- A versioned manifest
- File sizes and SHA-256 checksums

Legacy database-only backups remain accepted.

The server validates and stages the database replacement while running. Restart the application after a successful restore so the staged database is applied atomically before SQLite opens.

## Updating

1. Create a portable backup.
2. Stop the current process or container.
3. Replace the binary or pull the new image.
4. Start the application using the same data directory or volume.
5. Confirm `/api/health` reports the expected version and schema.

Database migrations are tracked transactionally and run automatically at startup.

## Next steps

- [Voice Studio](voice-library.md)
- [Script Reader](script-reader.md)
- [Projects](projects.md)
- [Settings & Administration](settings-administration.md)
