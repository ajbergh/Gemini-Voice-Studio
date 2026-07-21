# Gemini Voice Studio — Remediation Record

Delivery pull request: `#4`  
Implementation branch: `agent/full-remediation`  
Target branch: `main`

This document records the reliability, data-integrity, security, documentation, and release-engineering work delivered by the full-remediation program.

## Phase 0 — Release blockers

| Work item | Status | Delivered behavior |
|---|---|---|
| Preserve streaming HTTP interfaces | Complete | Logging middleware preserves `Flusher`, `Hijacker`, optimized transfer interfaces, and response-controller unwrapping. SSE and WebSocket regressions are covered by tests. |
| Clean-checkout backend build | Complete | The embedded frontend package has a tracked placeholder; production builds replace it with Vite output. |
| Reproducible container build | Complete | Node 22 and Go 1.25 build stages, deterministic installs, metadata injection, non-root runtime, persistent volume, and health checking are configured. |
| Functional container networking | Complete | Desktop defaults remain loopback-only; `--host` and `GVS_HOST` support explicit binding, and the container binds to `0.0.0.0`. |
| Protect durable media | Complete | Cleanup removes only unreferenced files and reports protected versus reclaimable media. History, presets, takes, artwork, and encryption metadata are protected. |
| Portable backup and restore | Complete with restart boundary | `.gvsbackup` archives contain a SQLite snapshot, durable media, encryption metadata, a versioned manifest, sizes, and SHA-256 checksums. Restores are validated and staged for atomic application at restart. |
| Apply finishing profiles | Complete | Shared trim, peak normalization, padding, inter-segment spacing, and WAV encoding are used by stitch and ZIP export. |
| Prevent false rendered state | Complete | Audio is written and atomically published before take persistence; files are removed if take persistence fails, and rendered state requires a persisted take. |
| Reject unsupported model IDs | Complete | Unknown TTS models return HTTP `422`; requested and effective render settings are recorded. |

## Phase 1 — Job and provider reliability

| Work item | Status | Delivered behavior |
|---|---|---|
| Propagate cancellation | Complete | Single-speaker, multi-speaker, streaming, and batch calls use contexts; retry delays are cancellable. |
| Bounded batch concurrency | Complete | Worker count is configurable, defaults to two, and is capped at eight. |
| Durable progress and final state | Complete | SQLite progress records and WebSocket events distinguish complete, partial, failed, and cancelled jobs. |
| Isolate slow WebSocket clients | Complete | Bounded per-client queues, writer goroutines, ping checks, and disconnect-on-backpressure prevent one browser from blocking workers. |
| Atomic API-key leasing | Complete | Pool entries are leased transactionally in least-recently-used order with lease and cooldown metadata. |
| Provider-aware key health | Foundation complete | Healthy, leased, cooldown, authentication failure, repeated-error, inactive, and reset states are persisted. |
| Safe restore lifecycle | Complete | Active processes do not replace their own open SQLite database; validated restore state is applied before the next open. |

## Phase 2 — Architecture and data integrity

| Work item | Status | Delivered behavior |
|---|---|---|
| Central model registry | Complete | Validation and provider discovery share the backend registry; project model choices load from `/api/providers`. |
| Shared audio processing | Complete | `internal/audio` owns finishing and WAV encoding for both direct stitch and packaged export. |
| Durable media lifecycle | Complete | Generated files are published atomically, referenced media is protected, and only orphaned files are reclaimable. |
| Versioned migrations | Complete | `schema_migrations` records transactional migration application. |
| Deterministic SQLite behavior | Complete | Foreign keys, WAL, busy timeout, synchronous mode, and single-connection behavior are configured consistently. |
| Runtime configuration hierarchy | Complete | Defaults → JSON config → `GVS_*` environment → explicit CLI flags. Host, port, paths, logging, passphrase, and browser behavior are covered. |
| Structured API errors | Complete and backward compatible | Responses retain the string `error` field and add stable code, retryability, and HTTP status data. Strict JSON parsing rejects unknown fields and trailing values. |
| Frontend provider catalogue | Partially complete by design | Project model choices are backend-driven. The visual voice catalogue remains bundled as an offline fallback. |

## Phase 3 — Security and release engineering

| Work item | Status | Delivered behavior |
|---|---|---|
| Harden API-key encryption | Complete with compatibility | New rows use a versioned PBKDF2-HMAC-SHA256 and AES-256-GCM envelope with a random installation salt. Legacy ciphertext remains decryptable. |
| Build/version diagnostics | Complete | `--version`, startup logs, and `/api/health` expose version, commit, build date, and schema information. |
| Continuous integration | Complete | CI runs frontend typecheck/build/functional Playwright, Go vet/test/race, and a clean Docker build. |
| Dependency automation | Complete | Dependabot covers npm, Go modules, and GitHub Actions. |
| Cross-platform releases | Complete | `v*` tags publish Linux amd64, Windows amd64, macOS Intel, macOS Apple Silicon, and SHA-256 checksums. |
| Signing and notarization | External prerequisite | Windows signing certificates and Apple Developer credentials must be provisioned before signed distribution can be enabled. |
| OS-native credential vaults | Deferred product decision | The portable encrypted envelope avoids new platform dependencies; DPAPI, Keychain, and Secret Service integration remain optional. |
| Automatic updater | Deferred product decision | Release assets are machine-readable, but update policy should follow signing and distribution decisions. |

## Documentation completion pass

The final pass reconciled user-facing guidance with the implementation and corrected:

- Repository clone path and product/binary names
- Node and Go prerequisites
- Build and validation commands
- Model registry and streaming capability
- Runtime configuration precedence
- `--host` and `GVS_HOST` network binding
- Platform storage and legacy-directory compatibility
- Durable versus reclaimable media semantics
- Encryption salt and migration requirements
- Portable backup contents, multipart field name, and restart boundary
- Actual WAV finishing fields and output format
- ZIP package contents and take-selection behavior
- Docker environment variables, volume, non-root runtime, and bind address
- Release targets, checksums, and unsigned-distribution caveats

Documentation reviewed without requiring functional changes: Voice Studio, Script Reader, Cast Bible, and Keyboard Shortcuts.

## Required validation gates

The pull request is eligible for merge only after its final head passes:

1. `npm ci`
2. `npm run typecheck`
3. `npm run build`
4. Functional Playwright Chromium suite
5. `go vet ./...`
6. `go test ./...`
7. `go test -race ./...`
8. Clean multi-stage Docker build

## Post-merge product decisions

- Provision Apple and Windows signing/notarization credentials.
- Decide whether to add an in-app updater.
- Decide whether to replace the bundled offline voice catalogue with backend-first loading.
- Optionally expose key-pool health and cooldown diagnostics in Settings.
- Add Firefox, WebKit, and mobile viewport gates if broader browser support becomes a release requirement.
