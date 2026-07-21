# Projects

The Projects workspace is the production pipeline for multi-segment narration, audiobook, podcast, training, commercial, and custom voice work.

A project combines script structure, rendering defaults, cast assignments, pronunciation rules, performance styles, take history, QC, and deliverable export.

## Project structure

```text
Project
└── Section
    ├── Segment
    ├── Segment
    └── Segment
```

A segment is the smallest independently rendered script unit. Each successful render creates a new take rather than replacing prior audio.

## Create a project

1. Open **Projects**.
2. Select **New**.
3. Choose a template or blank project.
4. Enter a title and optional description.
5. Optionally assign a client.
6. Review production defaults.
7. Create the project or create and immediately import a script.

Client defaults can seed new project settings, but project and segment values can override them.

## Project workspace

| Tab | Purpose |
|---|---|
| Script | Sections, segments, text import, AI prep, and rendering |
| Cast | Cast profiles, auditions, and continuity warnings |
| Review | Take playback, approve/flag workflow, and QC issues |
| Timeline | Ordered waveforms, pacing, and readiness |
| Export | Finishing profile, package readiness, export jobs, and downloads |

## Sections

Sections group related segments into chapters, scenes, folders, intros, outros, or other structural units.

Common actions:

- Add a section.
- Rename it.
- Change its kind.
- Collapse or expand it.
- Reorder it.
- Delete it after confirmation.

Deleting a section can affect all child segments; create a portable backup before large destructive edits.

## Segments

A segment can store or inherit:

- Script text
- Speaker label
- Voice
- Cast profile
- Provider
- Model
- Language code
- Accent
- Performance style
- Sort order
- Render status

Editing script text after rendering changes the segment to `changed`, indicating that existing audio no longer represents the current text.

### Segment states

| State | Meaning |
|---|---|
| `draft` | No current render |
| `rendering` | A render is in progress |
| `rendered` | Audio exists and awaits review |
| `approved` | The selected take is approved |
| `flagged` | The selected take requires attention |
| `changed` | Script changed after the last render |
| `failed` | The most recent render attempt failed |

## Model selection

The frontend loads project model options from the backend provider catalogue rather than maintaining an independent project-model list.

Supported TTS models are:

| Model | Streaming |
|---|---:|
| `gemini-3.1-flash-tts-preview` | Yes |
| `gemini-2.5-flash-preview-tts` | No |
| `gemini-2.5-pro-preview-tts` | No |

Unknown IDs are rejected with HTTP `422`. The requested provider/model and effective provider/model are stored in take provenance.

## Single-segment rendering

Select **Render** on a segment to create a take synchronously.

The backend performs the following lifecycle:

1. Resolves segment, cast, project, client, and global settings.
2. Applies pronunciation and performance instructions.
3. Validates the selected model.
4. Calls the provider with the request context.
5. Decodes the returned PCM.
6. Writes a temporary media file.
7. Flushes and atomically renames the file.
8. Analyzes duration, peak, RMS, and clipping.
9. Persists the take and render provenance.
10. Creates an automated QC issue when configured.
11. Marks the segment rendered.

A segment is not marked rendered unless a take was successfully persisted. If take persistence fails, the newly published audio file is removed.

Closing the request or losing the browser connection cancels provider work where the endpoint is request-bound.

## Batch rendering

**Render all** selects eligible draft, changed, or failed segments. A forced render can include segments regardless of current state.

Batch jobs use a bounded worker pool:

- Default concurrency: 2
- Minimum: 1
- Maximum: 8
- Request-level concurrency can override the stored default within those limits

Each worker updates segment state independently. Aggregate progress records completed and failed counts.

Final states are:

- `complete` when every selected segment succeeds
- `partial` when at least one succeeds and at least one fails
- `failed` when all fail
- `cancelled` when the user cancels or the job context ends

Cancellation reaches active Gemini HTTP calls and retry waits. Segments interrupted by cancellation are restored to their original state when possible.

## Retry behavior

Transient provider responses can be retried with cancellable backoff. Authentication failures and repeated provider errors are reported to the key-pool health system rather than treated as generic success.

## Takes and provenance

Every render creates a take containing production evidence such as:

- Provider and model
- Provider voice and application voice
- Language
- Script text
- System instruction
- Dictionary hash
- Prompt hash
- Render settings JSON
- Duration
- Peak and RMS levels
- Clipping result
- Sample rate, channel count, and format
- Audio path
- Status

This history supports comparison, auditability, re-review, and reproducible delivery decisions.

Take actions include:

- Play
- Approve
- Flag
- Add or edit notes
- Select the best take
- Delete

Deleting a take can make its media file reclaimable if no other record references it.

## Text import

Import supports Markdown and plain text.

Typical mapping:

- Headings become sections.
- Paragraphs or lines become segments.
- A preview is shown before changes are applied.

Review the preview carefully because imported structure can create many records at once.

## AI script preparation

AI prep can propose:

- Section structure
- Segment boundaries
- Speaker labels
- Speaker candidates
- Pronunciation candidates
- Performance suggestions
- Formatting warnings

The proposed result is reviewable before application. Applying it creates the corresponding project records.

## Project defaults

Project settings can define:

- Default voice
- Default provider/model
- Fallback provider/model
- Default language
- Default preset
- Default performance style
- Default export profile

Segments can override project defaults. Cast-profile assignments can provide additional voice and performance context.

## Pronunciation dictionaries

Enabled global entries are evaluated before enabled project entries. The applied pronunciation set contributes to the take's dictionary hash.

Use preview and test renders before launching a large batch.

## Performance styles

Styles can be global or project-scoped and can express pacing, energy, emotion, articulation, pause density, and director notes.

Style changes do not retroactively alter existing takes. Re-render changed segments to produce audio with the new direction.

## Cast profiles

Cast profiles associate named characters or roles with voice and performance defaults. Continuity checks identify segments whose current assignment differs from the linked cast profile.

See [Cast Bible](cast-bible.md).

## Review and QC

Rendered takes can be approved, flagged, and linked to QC issues. Automated clipping checks can add high-severity volume issues.

See [Review & Export](review-export.md).

## Export

The Export tab packages eligible takes into a ZIP with per-segment WAV files, a project master WAV, project metadata, cast data, pronunciation data, QC CSV, and render provenance.

Finishing profiles control silence trim, threshold, padding, inter-segment spacing, and peak normalization. Current output is 24 kHz, 16-bit, mono WAV.

## Job Center

The Job Center receives persisted progress events over WebSocket. Each connected client has a bounded queue; a slow or unhealthy browser is disconnected without blocking render workers.

Jobs survive page reloads through persisted progress records, although a process restart cannot resume an in-flight provider request from its midpoint.

## Recommended workflow

1. Create or import the script.
2. Define project defaults.
3. Configure cast, pronunciation, and performance styles.
4. Test representative segments.
5. Batch-render eligible segments.
6. Review and approve takes.
7. Resolve or accept QC issues.
8. Select a finishing profile.
9. Inspect the stitched master.
10. Create and retain the final ZIP.
11. Create a portable application backup.

## Related guides

- [Cast Bible](cast-bible.md)
- [Review & Export](review-export.md)
- [Settings & Administration](settings-administration.md)
