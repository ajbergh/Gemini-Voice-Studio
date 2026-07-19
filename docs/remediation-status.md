# Gemini Voice Studio — Full Remediation Status

Branch: `agent/full-remediation`  
Pull request: `#4`  
Baseline: `main` at `cba5883d9b10058aa53b6a1b5afbc7f19818e84f`

This document maps the engineering-review findings to the implementation delivered on the remediation branch. It distinguishes completed code changes from items that still require release credentials, platform-specific signing infrastructure, or a separate product decision.

## Phase 0 — Release blockers

| Work item | Status | Implementation |
|---|---|---|
| Preserve streaming HTTP interfaces | Complete | Logging middleware forwards `Flusher`, `Hijacker`, `ReaderFrom`, and supports `ResponseController` unwrapping. Regression tests cover SSE and WebSocket interfaces. |
| Clean-checkout backend build | Complete | The embedded frontend directory contains a tracked placeholder. Build scripts and Docker replace it with Vite output for production. |
| Reproducible container build | Complete | Go version matches `go.mod`; frontend sources are copied comprehensively; build metadata, health check, non-root runtime, and persistent volume are configured. |
| Protect production audio from cache clearing | Complete | Cache clearing removes only files not referenced by history, presets, takes, or preset artwork metadata. Statistics distinguish protected and reclaimable media. |
| Portable backup and restore | Complete with restart boundary | `.gvsbackup` archives contain a SQLite snapshot, media, manifest, sizes, and SHA-256 checksums. Database replacement is staged and applied atomically on restart rather than replacing a live DB. Legacy database-only backups remain accepted. |
| Apply finishing profiles to ZIP exports | Complete | Shared trim, peak normalization, padding, inter-segment spacing, and WAV encoding are used by stitched and packaged exports. ZIPs include per-segment WAVs plus a finished project master. |
| Prevent false rendered state | Complete | Audio is written to a temporary file, fsynced, atomically published, and removed if take persistence fails. A segment is not marked rendered without a persisted take. |
| Reject unsupported model IDs | Complete | Backend model validation returns `422` rather than silently substituting another model. Requested and effective settings are recorded in take provenance. |

## Phase 1 — Job and provider reliability

| Work item | Status | Implementation |
|---|---|---|
| Propagate cancellation to Gemini | Complete | Single-speaker, multi-speaker, streaming, and batch TTS use request contexts. Retry delays are cancellable. Browser disconnects terminate streaming work. |
| Bounded batch concurrency | Complete | Batch jobs use a configurable worker pool, capped at eight workers, with ordered aggregate progress and cancellation. |
| Durable job progress | Complete | Progress continues to be persisted to SQLite and broadcast over WebSocket. Final job states distinguish complete, partial, failed, and cancelled. |
| Isolate slow WebSocket clients | Complete | Each client has a bounded queue and writer goroutine; slow or unhealthy clients are disconnected without blocking render workers. Ping health checks are included. |
| Atomic API-key leasing | Complete | Pooled keys are leased transactionally by least-recently-used order and respect cooldown periods. |
| Provider-aware key health | Foundation complete | Storage supports healthy, leased, cooldown, reset, authentication failure, and repeated-error states. Provider call sites can report outcomes through the new store methods; full UI surfacing of health diagnostics remains optional follow-up work. |
| Safe restore lifecycle | Complete | Restores are validated and staged for next startup, avoiding races with active requests and background jobs. |

## Phase 2 — Architecture consolidation

| Work item | Status | Implementation |
|---|---|---|
| One model registry | Complete | Gemini model validation and provider discovery share one backend registry. Project settings load model options from `/api/providers`. |
| Shared audio processing | Complete | `internal/audio` owns finishing and WAV encoding used by stitch and deliverable export. |
| Durable media lifecycle | Complete | Database-referenced media is protected; generated files are atomically published; unreferenced files remain reclaimable. |
| Versioned migrations | Complete | `schema_migrations` records transactional migration application. Existing databases bootstrap from idempotent historical migrations. |
| Deterministic SQLite behavior | Complete | The store uses one pooled connection, enables foreign keys, WAL, busy timeout, and normal synchronous mode consistently. |
| Runtime configuration hierarchy | Complete | Resolution order is defaults → JSON config → `GVS_*` environment variables → explicit CLI flags. Validation runs before startup. |
| Structured API errors | Complete, backward compatible | Error responses retain the historical string `error` field and add `code`, `retryable`, and `http_status`. JSON request parsing rejects unknown fields and trailing values. |
| Frontend provider catalogue | Partially complete | Project model settings are dynamic. The static voice visual catalogue remains bundled as an offline fallback and can be migrated to backend-first loading in a future UI-only change. |

## Phase 3 — Security and release engineering

| Work item | Status | Implementation |
|---|---|---|
| Harden API-key encryption | Complete with compatibility | New rows use a versioned PBKDF2-HMAC-SHA256 key bundle and AES-256-GCM. Existing unprefixed ciphertext remains decryptable through the legacy key half. A random installation salt is stored with restrictive permissions. |
| Build/version diagnostics | Complete | `--version`, startup logs, and `/api/health` expose version, commit, and build date. Logs also include schema version. |
| Continuous integration | Complete | CI runs frontend typecheck/build/Playwright, Go vet/test/race, and a clean Docker build. |
| Dependency automation | Complete | Dependabot is configured for npm, Go modules, and GitHub Actions. |
| Cross-platform releases | Complete | Tag-driven builds produce Linux, Windows, macOS Intel, and macOS Apple Silicon binaries, generate SHA-256 checksums, and publish GitHub release assets. |
| Platform code signing/notarization | External prerequisite | Workflow structure is ready for signing steps, but Windows certificates and Apple Developer notarization credentials must be provisioned as repository secrets before enabling signed distribution. |
| OS-native credential vaults | Deferred product decision | The versioned local encryption envelope materially improves at-rest protection without new native dependencies. DPAPI/Keychain/Secret Service integration remains an optional platform-specific enhancement. |
| Automatic updater | Deferred product decision | Releases are versioned and machine-readable. In-app automatic update behavior should be selected only after signing and distribution channels are finalized. |

## Validation gates

The branch introduces CI as the source of truth for clean-checkout validation:

1. `npm ci`
2. `npm run typecheck`
3. `npm run build`
4. Playwright Chromium suite
5. `go vet ./...`
6. `go test ./...`
7. `go test -race ./...`
8. Docker multi-stage build

The pull request should remain draft until the latest head commit passes all CI jobs. No claim in this document substitutes for a green workflow run.

## Recommended post-merge work

The remaining work is intentionally outside the core reliability remediation:

- Provision Apple and Windows signing/notarization credentials.
- Decide whether to add an in-app updater or publish only signed GitHub releases.
- Decide whether the visual voice library should become backend-first rather than retain its bundled offline fallback.
- Optionally expose pooled-key health and cooldown diagnostics in Settings.
- Add Firefox/WebKit and mobile viewport jobs if broader browser support becomes a release requirement.
