# Getting Started

This guide covers development setup, production builds, runtime configuration, data storage, and portable backup/restore for Gemini Voice Studio.

## Prerequisites

| Requirement | Supported version | Purpose |
|---|---:|---|
| Node.js | 22+ | Frontend development and production asset builds |
| Go | 1.25+ | Backend and single-binary builds |
| Gemini API key | Current Google AI Studio key | Casting, script preparation, TTS, and portrait generation |

## Clone and install

```bash
git clone https://github.com/ajbergh/Gemini-Voice-Studio.git
cd Gemini-Voice-Studio
npm ci
```

`npm ci` is used instead of `npm install` so local builds match CI and the committed lockfile.

## Development mode

Development uses two processes. Vite serves the frontend and proxies API/WebSocket traffic to the Go backend.

**Terminal 1 — backend**

```bash
cd backend
go run ./cmd/server --open=false
```

Windows users can alternatively run:

```powershell
.\scripts\start-backend-dev.ps1
```

**Terminal 2 — frontend**

```bash
npm run dev
```

Open **http://localhost:3000**. The backend listens on `127.0.0.1:8080` by default. Vite proxies `/api/` and `/api/ws` to that backend.

The tracked `backend/internal/embed/dist/.gitkeep` file allows a clean checkout to compile without first building the frontend. Development traffic is still served by Vite; production builds replace the placeholder with the generated `dist` files.

## Configure a Gemini API key

1. Open **Settings**.
2. Enter a Gemini API key.
3. Use **Test Key**.
4. Save the key.

Gemini requests are made by the Go backend, not by the browser. New credentials are encrypted with a versioned PBKDF2-HMAC-SHA256 + AES-256-GCM envelope. Existing credentials encrypted by earlier releases remain readable.

For portable credentials, start the app with an explicit passphrase through `--passphrase`, `GVS_PASSPHRASE`, or the JSON config file. When no passphrase is supplied, the fallback remains machine-derived and is therefore intentionally tied to that user/machine identity.

### Key pools

Multiple Gemini keys can be added to a pool. The backend leases healthy keys transactionally, rotates by least-recently-used order, and supports cooldown/error health metadata. The primary key remains the fallback when the pool contains no eligible key.

## Validate before building

```bash
npm run typecheck
npm run build
cd backend
go vet ./...
go test ./...
go test -race ./...
```

The pull-request CI workflow runs the same validation plus Playwright and Docker builds.

## Production single-binary builds

Platform scripts build the frontend, embed it in the Go binary, and inject version metadata.

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

Artifacts are named consistently, for example:

```bash
./bin/gemini-voice-studio-linux-amd64 --version
./bin/gemini-voice-studio-linux-amd64 --open=true
```

Set `VERSION`, `COMMIT_SHA`, and `BUILD_DATE` before invoking a script to override embedded build metadata. Tag-driven GitHub releases set these values automatically.

## Runtime configuration

Resolution order is:

1. Compiled platform defaults
2. JSON configuration file
3. `GVS_*` environment variables
4. Explicit CLI flags

Later sources override earlier sources.

### CLI flags

| Flag | Description |
|---|---|
| `--config` | Optional JSON configuration file |
| `--port` | HTTP port, default `8080` |
| `--data-dir` | Persistent application data directory |
| `--db` | Explicit SQLite database path |
| `--audio-dir` | Durable generated-media directory |
| `--passphrase` | Credential-encryption passphrase |
| `--log-level` | `debug`, `info`, `warn`, or `error` |
| `--open` | Open the browser on startup |
| `--version` | Print build version, commit, and date |

### Environment variables

| Variable | Equivalent setting |
|---|---|
| `GVS_CONFIG` | `--config` |
| `GVS_PORT` | `--port` |
| `GVS_DATA_DIR` | `--data-dir` |
| `GVS_DB_PATH` | `--db` |
| `GVS_AUDIO_DIR` | `--audio-dir` |
| `GVS_PASSPHRASE` | `--passphrase` |
| `GVS_LOG_LEVEL` | `--log-level` |
| `GVS_OPEN_BROWSER` | `--open` |

Example:

```bash
GVS_DATA_DIR=/srv/gemini-voice-studio \
GVS_PASSPHRASE='use-a-secret-manager-in-production' \
./gemini-voice-studio-linux-amd64 --open=false
```

## Docker

```bash
docker build -t gemini-voice-studio .
docker run --rm \
  -p 8080:8080 \
  -v "$PWD/data:/home/app/data" \
  gemini-voice-studio
```

The image runs as a non-root user, exposes a health check at `/api/health`, and persists the database, generated media, encryption salt, and export workspace beneath `/home/app/data`.

## Data storage

The application maintains several classes of local state:

- SQLite database and migration ledger
- Encrypted API keys and key-pool health
- Project takes, TTS history audio, preset samples, and portrait artwork
- Temporary/reclaimable generated files
- Export ZIPs
- Installation encryption salt

Generated project media is durable. **Clear cache** removes only files that are not referenced by database records and never deletes the encryption salt.

SQLite runs in WAL mode with foreign keys enabled and a deterministic single-connection policy. Migrations are transactional and recorded in `schema_migrations`.

## Portable backup and restore

Settings → Backup creates a `.gvsbackup` archive containing:

- A consistent SQLite snapshot
- Durable generated media and artwork
- The installation encryption salt
- A versioned manifest
- File sizes and SHA-256 checksums

Command-line example:

```bash
curl -X POST http://localhost:8080/api/backup \
  --output gemini-voice-studio.gvsbackup
```

Restore uses multipart form data:

```bash
curl -X POST http://localhost:8080/api/restore \
  -F "backup=@gemini-voice-studio.gvsbackup"
```

The restore endpoint validates paths, schema compatibility, sizes, and checksums. It returns HTTP `202 Accepted` with `restart_required: true`. Restart the application immediately after a successful restore response; the database snapshot is applied atomically at process startup rather than swapped beneath active jobs and requests.

Legacy database-only backups are accepted, but they cannot restore media files or installation salt that were not present in the old format.

## Release builds

Pushing a `v*` tag runs the release workflow and produces:

- Linux AMD64
- Windows AMD64
- macOS AMD64
- macOS ARM64
- `SHA256SUMS.txt`

Apple notarization and Windows Authenticode signing require external signing credentials and are not enabled until those secrets are provisioned.

## Next steps

- [Voice Studio](voice-library.md)
- [Script Reader](script-reader.md)
- [Projects](projects.md)
- [Cast Bible](cast-bible.md)
- [Review & Export](review-export.md)
- [Settings & Administration](settings-administration.md)
- [Remediation Status](remediation-status.md)
