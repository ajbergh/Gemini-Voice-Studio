# Settings & Administration

This guide covers API-key security, key-pool behavior, render defaults, pronunciation dictionaries, performance styles, QC rules, storage cleanup, backup/restore, export profiles, and runtime configuration.

## Opening Settings

Open **Settings** from the gear icon in the application navigation. Available panels depend on the current build, but the backend contracts described here are authoritative.

## API keys

### Add or replace a key

1. Open **Settings → API Keys**.
2. Paste the Gemini API key.
3. Test it.
4. Save it.

The plaintext key is sent only to the Go backend. It is encrypted before being written to SQLite.

### Encryption format

New writes use:

- PBKDF2-HMAC-SHA256 key derivation
- A random per-installation salt
- AES-256-GCM authenticated encryption
- A versioned ciphertext envelope

The installation salt is stored in the generated-media directory with restrictive permissions. Legacy unversioned ciphertext remains decryptable for compatibility.

Do not migrate only `data.db`. Copy the whole data directory or use a portable backup so the database, generated media, and encryption metadata remain together.

### Delete a key

Deleting a provider key removes the encrypted row. Calls requiring that provider fail until a key is saved or an active pool entry is available.

## Key pool

A provider can have multiple encrypted keys. The backend leases active entries transactionally using least-recently-used order.

Pool state includes:

- Active or inactive status
- Last-used time
- Error count
- Health state
- Lease time
- Cooldown-until time

Authentication failures and repeated provider errors can place a key into cooldown or deactivate it. Resetting a pool entry clears the error state and makes it eligible again.

The leasing transaction prevents concurrent workers from selecting the same key as if it were still idle.

## Render defaults

Global defaults apply when a client, project, cast profile, or segment does not provide a more specific value.

Supported TTS models are supplied by `GET /api/providers`:

| Model | Streaming |
|---|---:|
| `gemini-3.1-flash-tts-preview` | Yes |
| `gemini-2.5-flash-preview-tts` | No |
| `gemini-2.5-pro-preview-tts` | No |

The backend rejects unknown model identifiers with HTTP `422` rather than silently substituting another model.

Batch concurrency is configurable and is capped at eight workers. The default is two workers when no stored or request-level value is supplied.

## Pronunciation dictionaries

### Global dictionaries

Global dictionaries are evaluated for every project before project-scoped rules.

Each dictionary can be enabled or disabled and can contain multiple word or phrase overrides. Enabled entries are incorporated into the render instruction and contribute to the stored dictionary hash for take provenance.

### Project dictionaries

Project dictionaries apply only to their project. They are evaluated after enabled global dictionaries.

Use the preview function before large renders to verify replacements and avoid unintended substring matches.

## Performance styles

Performance styles are reusable direction presets. A style can describe:

- Pacing
- Energy
- Emotion
- Articulation
- Pause density
- Director notes

Styles can be global or project-scoped. Saved revisions are versioned so earlier snapshots can be restored.

## QC rules

Global QC configuration includes:

| Setting | Behavior |
|---|---|
| Default severity | Initial severity for new QC issues |
| Auto-flag clipping | Creates a QC issue when clipped samples or the configured peak threshold are detected |
| Clipping threshold | Peak dBFS threshold used by automated analysis |
| Export only approved | Requires the selected take to have `approved` status before it is included |
| Notes export format | Controls the configured notes representation where supported |

Automated clipping analysis runs after audio is durably published and the take is persisted.

## Storage management

Generated audio referenced by application records is durable production data, not disposable cache.

The storage statistics endpoint reports:

- Total files and bytes
- Protected files and bytes
- Reclaimable files and bytes
- The active media directory

Protected media includes files referenced by:

- Generation history
- Custom preset samples
- Segment takes
- Preset artwork metadata
- Installation encryption metadata

**Clear Cache** removes only unreferenced files. It does not intentionally break history entries, presets, or project takes.

Deleting a history entry or take can make its media reclaimable if no other record references the same file.

## Portable backup

Create a backup from the UI or with:

```bash
curl -X POST http://localhost:8080/api/backup \
  --output gemini-voice-studio.gvsbackup
```

The server creates a consistent SQLite snapshot and packages it with durable media.

A `.gvsbackup` archive contains:

- `database/data.db`
- Files under `media/`
- `manifest.json`
- File sizes
- SHA-256 checksums
- Format version and creation time

The archive includes the installation salt because it resides in the media directory. This allows encrypted API-key rows to remain decryptable after a full migration when the same passphrase is used.

## Restore

Restore with:

```bash
curl -X POST http://localhost:8080/api/restore \
  -F "backup=@gemini-voice-studio.gvsbackup"
```

The multipart form field is named `backup`.

Restore processing:

1. Limits the upload size.
2. Detects portable ZIP versus legacy database-only format.
3. Rejects unsafe archive paths and symlinks.
4. Validates the manifest, file sizes, and SHA-256 checksums.
5. Validates the SQLite snapshot.
6. Stages media files.
7. Stages the database as `restore-pending.db`.
8. Applies the database atomically on the next application start.

Restart the process or container after a successful restore response.

A restore replaces the current application dataset. Create a fresh backup before restoring when the current state may be needed later.

Legacy database-only backups remain accepted, but they do not contain generated media or encryption metadata and are therefore not a complete migration mechanism.

## Export profiles

Export profiles currently control deterministic PCM finishing:

| Field | Description |
|---|---|
| Target kind | Named delivery use or category |
| Trim silence | Remove leading and trailing PCM below the threshold |
| Silence threshold dB | Threshold used by trimming |
| Leading silence ms | Padding before each finished segment and the master |
| Trailing silence ms | Padding after each finished segment and the master |
| Inter-segment silence ms | Spacing inserted between master segments |
| Normalize peak dB | Target peak used by PCM normalization |
| Metadata JSON | Optional profile metadata |

Current packaged audio is WAV at 24 kHz, 16-bit, mono. MP3, FLAC, arbitrary sample-rate conversion, LUFS normalization, and metadata embedding are not implemented and should not be presented as available profile options.

The stitch endpoint and ZIP exporter use the same shared finishing functions, preventing profile drift between the standalone master and packaged deliverable.

## Runtime configuration

Runtime configuration uses this precedence:

1. Platform defaults
2. Optional JSON file
3. `GVS_*` environment variables
4. Explicit CLI flags

Desktop builds bind to `127.0.0.1` by default. Binding to `0.0.0.0` or `::` exposes the HTTP server through available network interfaces. The application does not provide TLS or user authentication, so use an external trusted proxy and network controls before allowing remote access.

### Environment variables

| Variable | Purpose |
|---|---|
| `GVS_CONFIG` | JSON configuration path |
| `GVS_HOST` | HTTP bind host |
| `GVS_PORT` | HTTP port |
| `GVS_DATA_DIR` | Persistent root directory |
| `GVS_DB_PATH` | SQLite path |
| `GVS_AUDIO_DIR` | Generated-media path |
| `GVS_PASSPHRASE` | Encryption passphrase |
| `GVS_LOG_LEVEL` | `debug`, `info`, `warn`, or `error` |
| `GVS_OPEN_BROWSER` | Boolean browser-open setting |

### CLI flags

```text
--config
--host
--port
--data-dir
--db
--audio-dir
--passphrase
--log-level
--open
--version
```

The passphrase is not serialized into JSON configuration.

## Default data directories

| Platform | New installation path |
|---|---|
| Windows | `%APPDATA%\gemini-voice-studio` |
| macOS | `~/Library/Application Support/gemini-voice-studio` |
| Linux | `$XDG_DATA_HOME/gemini-voice-studio` or `~/.local/share/gemini-voice-studio` |

For upgrade compatibility, an existing `gemini-voice-library` directory is reused when no new directory exists. An existing `audio_cache` subdirectory is likewise reused when no `audio` directory exists.

## Health and diagnostics

`GET /api/health` reports:

- Health status
- Version
- Commit SHA
- Build date
- Database schema version

The same build information is available through `--version`, and startup logs include the resolved bind address, database path, and media path.

## Administrative checklist

Before upgrading, migrating, or restoring:

1. Create a portable backup.
2. Confirm the backup file is non-empty.
3. Record the passphrase source.
4. Stop active renders before moving the data directory.
5. Restart after restore.
6. Verify `/api/health` and play at least one existing take.
