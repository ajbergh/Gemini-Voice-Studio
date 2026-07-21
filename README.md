<p align="center">
  <img src="assets/banner.svg" alt="Gemini Voice Studio" width="100%">
</p>

# Gemini Voice Studio

Gemini Voice Studio is a local-first voice-production application for casting Gemini voices, generating single- and multi-speaker speech, managing multi-segment projects, reviewing takes, tracking QC issues, and packaging finished WAV deliverables.

The production build is a single self-contained executable with an embedded React frontend, a pure-Go HTTP backend, SQLite persistence, durable generated media, and encrypted API-key storage.

## Core capabilities

- Browse and compare the bundled Gemini voice catalogue.
- Generate single-speaker, two-speaker, and streaming TTS.
- Save reusable custom voice presets and generated artwork.
- Organize audiobook, podcast, training, commercial, and custom projects.
- Import scripts, run AI script preparation, and manage sections and segments.
- Assign voices, models, languages, cast profiles, pronunciation rules, and performance styles.
- Render segments individually or through cancellable bounded-concurrency batch jobs.
- Preserve multiple takes with render provenance and automated clipping analysis.
- Review, approve, flag, and annotate takes with structured QC issues.
- Export per-segment WAV files, a finished project master, metadata, cast data, pronunciation data, and QC reports in a ZIP deliverable.
- Create portable backups containing the SQLite snapshot, generated media, encryption metadata, a manifest, and SHA-256 checksums.

## Documentation

| Guide | Scope |
|---|---|
| [Getting Started](docs/getting-started.md) | Development, production builds, Docker, configuration, and storage |
| [Voice Studio](docs/voice-library.md) | Voice browsing, filtering, favorites, and AI casting |
| [Script Reader](docs/script-reader.md) | Single speaker, dialogue, compare, tags, and streaming |
| [Projects](docs/projects.md) | Sections, segments, takes, batch rendering, and project defaults |
| [Cast Bible](docs/cast-bible.md) | Cast profiles, auditions, and continuity warnings |
| [Review & Export](docs/review-export.md) | Review, QC, finishing profiles, stitched WAV, and ZIP delivery |
| [Settings & Administration](docs/settings-administration.md) | Keys, storage, backup/restore, QC, and configuration |
| [Keyboard Shortcuts](docs/keyboard-shortcuts.md) | Application and review shortcuts |
| [Remediation Record](docs/remediation-status.md) | Reliability and release-engineering changes delivered by PR #4 |

## Supported TTS models

The backend registry is authoritative. Unknown model identifiers are rejected with HTTP `422`; the server does not silently substitute a different model.

| Model | Streaming | Intended use |
|---|---:|---|
| `gemini-3.1-flash-tts-preview` | Yes | Default low-latency expressive generation |
| `gemini-2.5-flash-preview-tts` | No | Cost-efficient high-volume generation |
| `gemini-2.5-pro-preview-tts` | No | Highest-fidelity long-form generation |

The current model catalogue is also available from `GET /api/providers`.

## Requirements

For development and local release builds:

- Node.js 22
- npm with the checked-in `package-lock.json`
- Go 1.25
- A Gemini API key with access to the configured models

The distributed single binary and Docker image do not require Node.js or Go at runtime.

## Development

```bash
git clone https://github.com/ajbergh/Gemini-Voice-Studio.git
cd Gemini-Voice-Studio
npm ci
```

Start the backend:

```bash
cd backend
go run ./cmd/server --open=false
```

Start the frontend in a second terminal:

```bash
npm run dev
```

The Vite development server listens on `http://localhost:4000` and proxies `/api` to the Go server on `http://localhost:8080`.

Useful validation commands:

```bash
npm run typecheck
npm run build
npm run test:e2e

cd backend
go vet ./...
go test ./...
go test -race ./...
```

## Production binaries

Each platform script runs `npm ci`, TypeScript validation, the Vite production build, frontend embedding, and a metadata-stamped Go build.

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

Artifacts are written to `bin/` with names such as:

```text
gemini-voice-studio-windows-amd64.exe
gemini-voice-studio-linux-amd64
gemini-voice-studio-darwin-arm64
```

Inspect embedded build metadata with:

```bash
./bin/gemini-voice-studio-linux-amd64 --version
```

## Runtime configuration

Configuration is resolved in this order, with later sources overriding earlier ones:

1. Platform defaults
2. Optional JSON configuration file
3. `GVS_*` environment variables
4. Explicit CLI flags

### CLI flags

| Flag | Purpose |
|---|---|
| `--config PATH` | Optional JSON configuration file |
| `--port N` | HTTP port; default `8080` |
| `--data-dir PATH` | Root directory for SQLite and generated media |
| `--db PATH` | Explicit SQLite database path |
| `--audio-dir PATH` | Explicit generated-media directory |
| `--passphrase VALUE` | API-key encryption passphrase |
| `--log-level LEVEL` | `debug`, `info`, `warn`, or `error` |
| `--open=true|false` | Open the browser after startup; default `true` |
| `--version` | Print version, commit, and build date |

### Environment variables

| Variable | Equivalent |
|---|---|
| `GVS_CONFIG` | `--config` |
| `GVS_PORT` | `--port` |
| `GVS_DATA_DIR` | `--data-dir` |
| `GVS_DB_PATH` | `--db` |
| `GVS_AUDIO_DIR` | `--audio-dir` |
| `GVS_PASSPHRASE` | `--passphrase` |
| `GVS_LOG_LEVEL` | `--log-level` |
| `GVS_OPEN_BROWSER` | `--open` |

Example JSON configuration:

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

The passphrase is intentionally not serialized to JSON. Supply it through `GVS_PASSPHRASE` or `--passphrase`.

## Persistent storage

New installations use these platform directories:

| Platform | Default data directory |
|---|---|
| Windows | `%APPDATA%\gemini-voice-studio` |
| macOS | `~/Library/Application Support/gemini-voice-studio` |
| Linux | `$XDG_DATA_HOME/gemini-voice-studio` or `~/.local/share/gemini-voice-studio` |

The directory contains:

```text
data.db              SQLite database
audio/               generated audio, preset media, and encryption salt
exports/             generated export archives when applicable
restore-pending.db   validated database restore waiting for restart, when present
```

Existing `gemini-voice-library` and `audio_cache` installations are detected and reused when no new-name directory exists.

Files referenced by history, presets, segment takes, or preset artwork are durable assets. The storage cleanup endpoint removes only unreferenced files and reports protected and reclaimable totals separately.

## API-key security

New API-key writes use a versioned PBKDF2-HMAC-SHA256 key derivation envelope with a per-installation random salt and AES-256-GCM authenticated encryption. Legacy unversioned ciphertext remains decryptable for compatibility.

Back up the database and media together. The media directory contains the installation salt required to decrypt encrypted rows after migration to another machine.

## Backup and restore

Create a portable backup:

```bash
curl -X POST http://localhost:8080/api/backup \
  --output gemini-voice-studio.gvsbackup
```

Restore a backup:

```bash
curl -X POST http://localhost:8080/api/restore \
  -F "backup=@gemini-voice-studio.gvsbackup"
```

Portable backups contain the SQLite snapshot, durable media, encryption metadata, a versioned manifest, file sizes, and SHA-256 checksums. Legacy database-only backup files are still accepted.

Restore validation completes while the application is running, but the database replacement is staged and applied atomically on the next process start. Restart the application after a successful restore response.

## Docker

```bash
docker compose up --build
```

The application is available at `http://localhost:8080`. The Compose file mounts the named `app-data` volume at `/home/app/data` and supplies the supported `GVS_*` variables.

Example:

```bash
GVS_PASSPHRASE='use-a-secret-value' \
GVS_LOG_LEVEL=info \
docker compose up --build
```

The image runs as a non-root user and includes an HTTP health check against `/api/health`.

## Export behavior

Export jobs create ZIP archives on disk rather than buffering the whole package in memory. A package can include:

- One finished 24 kHz, 16-bit, mono WAV per selected take
- `audio/project-master.wav`
- `project.json`
- `cast-bible.json`
- `pronunciation-dictionary.json`
- `qc-issues.csv`
- `render-metadata.json`
- A package README

Finishing profiles currently control silence trimming, silence threshold, leading padding, trailing padding, inter-segment spacing, and peak normalization. The same primitives are used for stitched WAV and packaged exports.

## CI and releases

Pull-request CI validates:

- `npm ci`
- TypeScript typecheck
- Production frontend build
- Functional Chromium Playwright suite
- `go vet ./...`
- `go test ./...`
- `go test -race ./...`
- Clean multi-stage Docker build

Pushing a tag matching `v*` creates GitHub Release assets for:

- Linux amd64
- Windows amd64
- macOS arm64
- macOS amd64
- `SHA256SUMS.txt`

Windows Authenticode signing and Apple notarization are not enabled until the required certificates and repository secrets are provisioned.

## Health and diagnostics

`GET /api/health` reports application health plus version, commit, build date, and schema version. Startup logs include the resolved database and audio paths.

## License

Apache-2.0. See [LICENSE](LICENSE).
