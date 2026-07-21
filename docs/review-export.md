# Review & Export

This guide covers take review, approval, QC issues, export readiness, finishing profiles, stitched WAV generation, and ZIP deliverables.

## Review workflow

Open a project and select **Review**. The workspace combines the segment queue, selected-take player, and QC issues for the active segment.

### Queue filters

| Filter | Shows |
|---|---|
| All | Every segment |
| Unreviewed | Segments without an approve or flag decision |
| Flagged | Segments whose selected take is flagged |
| Open Issues | Segments with unresolved QC issues |

### Review shortcuts

| Action | Key |
|---|---|
| Play or pause | `Space` |
| Approve | `A` |
| Flag | `F` |
| Replay | `R` |
| Previous segment | `P` |
| Next segment | `N` |
| Add QC marker | `M` |

## Approve and flag

Approving a take marks it ready for delivery. Flagging records that the take requires attention or replacement.

The global **Export only approved** rule determines whether the exporter may include a rendered but unapproved take. When the rule is enabled, a segment without an approved take is omitted from the package.

## QC issues

A QC issue records a structured problem against a project segment and, when available, a specific take.

Issues include:

- Issue type
- Severity
- Note
- Optional time offset
- Open, resolved, or won't-fix status
- Optional take reference

Automated clipping analysis can create a high-severity volume issue after rendering when clipped PCM samples are detected or the configured peak threshold is reached.

## Timeline and readiness

The Timeline view presents segment waveforms and delivery state in script order. Use it to inspect pacing, find missing audio, and confirm approval coverage.

Typical readiness blockers are:

- Draft or changed segments without current audio
- Segments without an eligible take
- Unapproved takes when approval-only export is enabled
- Open QC issues that must be resolved under the team's delivery policy

**Render missing** starts a bounded-concurrency batch job for eligible draft and changed segments. Progress and cancellation are available through the Job Center.

## Export profiles

Export profiles are deterministic PCM finishing profiles. They currently control:

| Setting | Behavior |
|---|---|
| Trim silence | Removes leading and trailing PCM below the threshold |
| Silence threshold dB | Threshold used by the trim operation |
| Leading silence ms | Adds padding before each segment and the master |
| Trailing silence ms | Adds padding after each segment and the master |
| Inter-segment silence ms | Inserts spacing between master segments |
| Normalize peak dB | Applies peak normalization |
| Target kind | Names the intended delivery category |
| Metadata JSON | Stores optional profile metadata |

The current audio output is 24 kHz, 16-bit, mono WAV. MP3, FLAC, arbitrary sample-rate conversion, arbitrary bit-depth conversion, LUFS normalization, and metadata embedding are not implemented.

## Start an export

1. Open the project **Export** tab.
2. Select an export profile when finishing is required.
3. Review the readiness summary.
4. Start the export.
5. Track the background job in the Job Center.
6. Download the ZIP after the job reaches `complete`.

The exporter streams the ZIP to a temporary file in the export directory and atomically publishes the completed archive. It does not buffer the entire deliverable in memory.

Cancellation is checked while the package is assembled. Failed or cancelled work does not publish a partial ZIP as a completed export.

## ZIP contents

A completed package can contain:

```text
audio/
  001-<voice>.wav
  002-<voice>.wav
  ...
  project-master.wav
project.json
cast-bible.json
pronunciation-dictionary.json
qc-issues.csv
render-metadata.json
README.txt
```

### Per-segment WAV files

Each included take is written as a finished WAV using the selected profile. The filename is ordered and sanitized for portability.

### Project master

`audio/project-master.wav` concatenates the selected takes in project order. The profile's inter-segment spacing is inserted between segments, followed by master-level leading/trailing padding and peak normalization.

### Metadata files

- `project.json` contains project, section, segment, and selected-take data.
- `cast-bible.json` contains project cast profiles.
- `pronunciation-dictionary.json` contains enabled project pronunciation entries.
- `qc-issues.csv` contains exported QC issues.
- `render-metadata.json` records selected take provenance, provider/model values, audio format, and profile information.

## Take selection

For each segment, the exporter asks the store for the best eligible take. When approval-only export is enabled, the selected take must have `approved` status.

A take with a missing or unreadable audio path fails the export instead of creating a package that claims to contain complete audio.

## Stitch to WAV

The stitch endpoint creates a master WAV directly without starting an export job.

1. Open the Timeline or stitch action.
2. Optionally choose a profile.
3. Optionally choose a section.
4. Download the generated WAV.

Stitching and ZIP packaging use the same shared audio-finishing package, so silence trimming, padding, inter-segment spacing, peak normalization, and WAV encoding behave consistently.

The stitch workflow skips segments without readable audio. It returns an error when no renderable segment remains.

## Job states

Background jobs can report:

- `queued`
- `running`
- `complete`
- `partial`
- `failed`
- `cancelled`

Batch rendering uses `partial` when some segments complete and others fail. Export jobs publish a downloadable file only after complete archive finalization.

## Operational checks before delivery

1. Confirm the latest script changes are rendered.
2. Review all intended takes.
3. Resolve or explicitly accept open QC issues.
4. Verify approval-only policy.
5. Select the intended finishing profile.
6. Download and inspect the master WAV.
7. Verify the package metadata and segment count.
8. Retain the ZIP and a portable application backup separately.

## Related guides

- [Projects](projects.md)
- [Settings & Administration](settings-administration.md)
- [Keyboard Shortcuts](keyboard-shortcuts.md)
