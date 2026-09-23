/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * api.ts — Frontend API Client
 *
 * Typed abstraction layer for the Go backend. All Gemini AI and media-generation
 * calls are proxied through the backend so API keys and generated assets stay
 * server-side.
 *
 * Main endpoint groups:
 *   /api/voices             — voice listing, casting recommendations, TTS, script formatting
 *   /api/projects           — project, section, segment, render, stitch, and prep workflows
 *   /api/presets            — saved voice presets, tags, versions, headshots, import/export
 *   /api/keys and /providers — API-key pool management and provider metadata
 *   /api/history and /jobs  — generation history, progress events, and persisted jobs
 *   /api/qc, /api/clients,
 *   /api/export-profiles    — review, client workspace, and deliverable settings
 *
 * In development, Vite proxies /api to http://localhost:8080. In production,
 * the Go binary serves both the SPA and API on the same port.
 */

import {
  AiRecommendation,
  CastAuditionInput,
  CastAuditionResponse,
  CastProfile,
  CastProfileVersion,
  CreateCastProfileInput,
  CreateEntryInput,
  CreateQcIssueInput,
  CreateScriptProjectInput,
  CreateScriptSectionInput,
  CreateScriptSegmentInput,
  CreateStyleInput,
  CreateTakeInput,
  CustomPreset,
  ExportProfile,
  ImportPreview,
  PerformanceStyle,
  PerformanceStyleVersion,
  PresetTag,
  PronunciationDictionary,
  PronunciationEntry,
  PreviewResult,
  ProjectSummary,
  QcIssue,
  QcRollup,
  ScriptProject,
  ScriptSection,
  ScriptSegment,
  SegmentTake,
  TakeNote,
  UpdateCastProfileInput,
  UpdateQcIssueInput,
  UpdateScriptProjectInput,
  UpdateScriptSectionInput,
  UpdateScriptSegmentInput,
  UpdateStyleInput,
  Voice,
} from './types';

/** Base path for all API requests; proxied to Go backend in dev. */
const API_BASE = '/api';

/**
 * Generic fetch wrapper with JSON content-type and error handling.
 * Throws on non-OK responses with the response body as the error message.
 */
async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const res = await fetch(`${API_BASE}${path}`, {
    headers: { 'Content-Type': 'application/json' },
    ...options,
  });
  if (!res.ok) {
    const body = await res.text().catch(() => res.statusText);
    throw new Error(body || `Request failed: ${res.status}`);
  }
  return res.json();
}

// --- Voices ---

/** Fetch the full voice catalogue from the backend database. */
export async function getVoices(): Promise<Voice[]> {
  return request<Voice[]>('/voices');
}

/**
 * Request AI voice recommendations from Gemini via the backend proxy.
 * Sends the user's query and the full voice catalogue for context.
 */
export async function recommendVoices(query: string, voices: { name: string; gender: string; pitch: string; characteristics: string[] }[]): Promise<AiRecommendation> {
  const raw = await request<AiRecommendation | Array<{ voiceName?: string; name?: string; prompt?: string }>>('/voices/recommend', {
    method: 'POST',
    body: JSON.stringify({ query, voices }),
  });
  if (Array.isArray(raw)) {
    const voiceNames = raw
      .map(item => item.voiceName || item.name || '')
      .filter(Boolean);
    return {
      voiceNames,
      systemInstruction: raw[0]?.prompt || 'Read naturally with the recommended voice style.',
      sampleText: query,
      sourceQuery: query,
    };
  }
  return raw;
}

/**
 * Generate TTS audio via the Gemini backend proxy.
 * Returns base64-encoded raw PCM audio (24kHz, 16-bit, mono).
 * An optional systemInstruction shapes the voice's delivery style.
 * An optional languageCode (e.g. "en", "es") overrides automatic language detection.
 */
export async function generateTts(text: string, voiceName: string, systemInstruction?: string, languageCode?: string, model?: string): Promise<string> {
  const body: Record<string, string> = { text, voiceName };
  if (systemInstruction) body.systemInstruction = systemInstruction;
  if (languageCode) body.languageCode = languageCode;
  if (model) body.model = model;
  const data = await request<{ audioBase64: string }>('/voices/tts', {
    method: 'POST',
    body: JSON.stringify(body),
  });
  return data.audioBase64;
}

/**
 * Generate multi-speaker dialogue TTS audio via the Gemini backend proxy.
 * Returns base64-encoded raw PCM audio (24kHz, 16-bit, mono).
 */
export async function generateMultiSpeakerTts(
  text: string,
  speakers: { speaker: string; voiceName: string }[],
  languageCode?: string,
  model?: string
): Promise<string> {
  const body: Record<string, any> = { text, speakers };
  if (languageCode) body.languageCode = languageCode;
  if (model) body.model = model;
  const data = await request<{ audioBase64: string }>('/voices/tts/multi', {
    method: 'POST',
    body: JSON.stringify(body),
  });
  return data.audioBase64;
}

// --- API Keys ---

/** Metadata for a stored API key (the actual key value is never sent to the frontend). */
export interface APIKeyInfo {
  id: number;
  provider: string;
  created_at: string;
  updated_at: string;
}

/** List all stored API key providers (without exposing key values). */
export async function listApiKeys(): Promise<APIKeyInfo[]> {
  return request<APIKeyInfo[]>('/keys');
}

/** Save an API key for the given provider; the backend encrypts it at rest. */
export async function storeApiKey(provider: string, key: string): Promise<void> {
  await request<void>('/keys', {
    method: 'POST',
    body: JSON.stringify({ provider, key }),
  });
}

/** Delete the stored API key for the given provider. */
export async function deleteApiKey(provider: string): Promise<void> {
  await request<void>(`/keys/${encodeURIComponent(provider)}`, {
    method: 'DELETE',
  });
}

/** Validate the stored API key by making a lightweight Gemini API call. */
export async function testApiKey(provider: string): Promise<{ valid: boolean; message: string }> {
  return request<{ valid: boolean; message: string }>(`/keys/${encodeURIComponent(provider)}/test`);
}

// --- API Key Pool (rotation) ---

/** Metadata for a key in the rotation pool. */
export interface APIKeyPoolEntry {
  id: number;
  provider: string;
  label: string;
  is_active: boolean;
  error_count: number;
  last_used_at: string;
  created_at: string;
  updated_at: string;
}

/** List all keys in the rotation pool for a provider. */
export async function listKeyPool(provider: string): Promise<APIKeyPoolEntry[]> {
  return request<APIKeyPoolEntry[]>(`/keys/${encodeURIComponent(provider)}/pool`);
}

/** Add a new key to the rotation pool. */
export async function addKeyToPool(provider: string, key: string, label?: string): Promise<{ id: number; status: string }> {
  return request<{ id: number; status: string }>(`/keys/${encodeURIComponent(provider)}/pool`, {
    method: 'POST',
    body: JSON.stringify({ key, label: label || '' }),
  });
}

/** Remove a key from the rotation pool. */
export async function deleteKeyFromPool(provider: string, id: number): Promise<void> {
  await request<void>(`/keys/${encodeURIComponent(provider)}/pool`, {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  });
}

/** Reset error count and reactivate a pool key. */
export async function resetPoolKey(provider: string, id: number): Promise<void> {
  await request<void>(`/keys/${encodeURIComponent(provider)}/pool/reset`, {
    method: 'POST',
    body: JSON.stringify({ id }),
  });
}

// --- Config ---

/**
 * Typed constants for well-known config keys stored in the backend `config`
 * table. Use these instead of raw string literals to prevent typos and enable
 * easy future refactoring.
 */
export const CONFIG_KEYS = {
  DEFAULT_MODEL: 'default_model',
  DEFAULT_LANGUAGE_CODE: 'default_language_code',
  DEFAULT_BATCH_CONCURRENCY: 'default_batch_concurrency',
  DEFAULT_RETRY_COUNT: 'default_retry_count',
  CONTINUE_BATCH_ON_ERROR: 'continue_batch_on_error',
  DEFAULT_PROVIDER: 'default_provider',
  FALLBACK_PROVIDER: 'fallback_provider',
  FALLBACK_MODEL: 'fallback_model',
  LAST_OPEN_PROJECT_ID: 'last_open_project_id',
  DEFAULT_EXPORT_PROFILE_ID: 'default_export_profile_id',
  QC_DEFAULT_SEVERITY: 'qc_default_severity',
  QC_AUTO_FLAG_CLIPPING: 'qc_auto_flag_clipping',
  QC_CLIPPING_THRESHOLD_DB: 'qc_clipping_threshold_db',
  QC_EXPORT_ONLY_APPROVED: 'qc_export_only_approved',
  QC_EXPORT_NOTES_FORMAT: 'qc_export_notes_format',
  APPEARANCE_THEME: 'appearance_theme',
  APPEARANCE_ACCENT_COLOR: 'appearance_accent_color',
  APPEARANCE_HIGH_CONTRAST: 'appearance_high_contrast',
} as const;

/** Union type of all known config key values. */
export type ConfigKey = typeof CONFIG_KEYS[keyof typeof CONFIG_KEYS];

/** Retrieve all key-value config pairs from the backend. */
export async function getConfig(): Promise<Record<string, string>> {
  return request<Record<string, string>>('/config');
}

/** Upsert one or more config key-value pairs. */
export async function updateConfig(config: Record<string, string>): Promise<void> {
  await request<void>('/config', {
    method: 'PUT',
    body: JSON.stringify(config),
  });
}

// --- Script Projects ---

export async function listProjects(): Promise<ScriptProject[]> {
  return request<ScriptProject[]>('/projects');
}

export async function listProjectSummaries(): Promise<ProjectSummary[]> {
  return request<ProjectSummary[]>('/projects/summary');
}

export async function createProject(data: CreateScriptProjectInput): Promise<ScriptProject> {
  return request<ScriptProject>('/projects', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function getProject(id: number): Promise<ScriptProject> {
  return request<ScriptProject>(`/projects/${id}`);
}

export async function updateProject(id: number, data: UpdateScriptProjectInput): Promise<ScriptProject> {
  return request<ScriptProject>(`/projects/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function archiveProject(id: number): Promise<void> {
  await request<void>(`/projects/${id}`, { method: 'DELETE' });
}

export async function listProjectSections(projectId: number): Promise<ScriptSection[]> {
  return request<ScriptSection[]>(`/projects/${projectId}/sections`);
}

export async function createProjectSection(projectId: number, data: CreateScriptSectionInput): Promise<{ id: number }> {
  return request<{ id: number }>(`/projects/${projectId}/sections`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateProjectSection(projectId: number, sectionId: number, data: UpdateScriptSectionInput): Promise<void> {
  await request<void>(`/projects/${projectId}/sections/${sectionId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteProjectSection(projectId: number, sectionId: number): Promise<void> {
  await request<void>(`/projects/${projectId}/sections/${sectionId}`, { method: 'DELETE' });
}

export async function listProjectSegments(projectId: number): Promise<ScriptSegment[]> {
  return request<ScriptSegment[]>(`/projects/${projectId}/segments`);
}

export async function createProjectSegment(projectId: number, data: CreateScriptSegmentInput): Promise<{ id: number }> {
  return request<{ id: number }>(`/projects/${projectId}/segments`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function updateProjectSegment(projectId: number, segmentId: number, data: UpdateScriptSegmentInput): Promise<void> {
  await request<void>(`/projects/${projectId}/segments/${segmentId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteProjectSegment(projectId: number, segmentId: number): Promise<void> {
  await request<void>(`/projects/${projectId}/segments/${segmentId}`, { method: 'DELETE' });
}

export async function importProjectText(
  projectId: number,
  text: string,
  filename?: string,
): Promise<{ sections_created: number; segments_created: number }> {
  return request<{ sections_created: number; segments_created: number }>(
    `/projects/${projectId}/import`,
    { method: 'POST', body: JSON.stringify({ text, filename: filename ?? '' }) },
  );
}

export async function previewProjectImport(
  projectId: number,
  text: string,
  filename?: string,
): Promise<ImportPreview> {
  return request<ImportPreview>(
    `/projects/${projectId}/import/preview`,
    { method: 'POST', body: JSON.stringify({ text, filename: filename ?? '' }) },
  );
}

// --- Cast Bible ---

export async function listProjectCast(projectId: number): Promise<CastProfile[]> {
  return request<CastProfile[]>(`/projects/${projectId}/cast`);
}

export async function createCastProfile(projectId: number, data: CreateCastProfileInput): Promise<CastProfile> {
  return request<CastProfile>(`/projects/${projectId}/cast`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function getCastProfile(profileId: number): Promise<CastProfile> {
  return request<CastProfile>(`/cast/${profileId}`);
}

export async function updateCastProfile(profileId: number, data: UpdateCastProfileInput): Promise<CastProfile> {
  return request<CastProfile>(`/cast/${profileId}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

export async function deleteCastProfile(profileId: number): Promise<void> {
  await request<void>(`/cast/${profileId}`, { method: 'DELETE' });
}

export async function listCastProfileVersions(profileId: number): Promise<CastProfileVersion[]> {
  return request<CastProfileVersion[]>(`/cast/${profileId}/versions`);
}

export async function revertCastProfileVersion(profileId: number, versionId: number): Promise<CastProfile> {
  return request<CastProfile>(`/cast/${profileId}/versions/${versionId}/revert`, { method: 'POST' });
}

export async function auditionCastProfile(profileId: number, input: CastAuditionInput): Promise<CastAuditionResponse> {
  return request<CastAuditionResponse>(`/cast/${profileId}/audition`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

// --- Segment Takes ---

export async function listSegmentTakes(projectId: number, segmentId: number): Promise<SegmentTake[]> {
  return request<SegmentTake[]>(`/projects/${projectId}/segments/${segmentId}/takes`);
}

export async function createSegmentTake(projectId: number, segmentId: number, data: CreateTakeInput): Promise<SegmentTake> {
  return request<SegmentTake>(`/projects/${projectId}/segments/${segmentId}/takes`, {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

export async function getSegmentTake(projectId: number, segmentId: number, takeId: number): Promise<SegmentTake> {
  return request<SegmentTake>(`/projects/${projectId}/segments/${segmentId}/takes/${takeId}`);
}

export async function deleteSegmentTake(projectId: number, segmentId: number, takeId: number): Promise<void> {
  await request<void>(`/projects/${projectId}/segments/${segmentId}/takes/${takeId}`, { method: 'DELETE' });
}

export async function listTakeNotes(projectId: number, segmentId: number, takeId: number): Promise<TakeNote[]> {
  return request<TakeNote[]>(`/projects/${projectId}/segments/${segmentId}/takes/${takeId}/notes`);
}

export async function createTakeNote(projectId: number, segmentId: number, takeId: number, note: string): Promise<{ id: number }> {
  return request<{ id: number }>(`/projects/${projectId}/segments/${segmentId}/takes/${takeId}/notes`, {
    method: 'POST',
    body: JSON.stringify({ note }),
  });
}

export async function deleteTakeNote(projectId: number, segmentId: number, takeId: number, noteId: number): Promise<void> {
  await request<void>(`/projects/${projectId}/segments/${segmentId}/takes/${takeId}/notes/${noteId}`, { method: 'DELETE' });
}

/** Retrieve cached PCM audio for a take as a base64-encoded string. */
export async function getTakeAudio(projectId: number, segmentId: number, takeId: number): Promise<string> {
  const data = await request<{ audioBase64: string }>(
    `/projects/${projectId}/segments/${segmentId}/takes/${takeId}/audio`,
  );
  return data.audioBase64;
}

// --- Batch Render ---

export interface BatchRenderOptions {
  /** Limit rendering to specific segment IDs; omit to render all draft/changed segments. */
  segmentIds?: number[];
  /** If true, render even already-rendered segments. */
  force?: boolean;
}

export interface BatchRenderResponse {
  job_id: string;
  segment_count?: number;
  rendered?: number;
  failed?: number;
  skipped?: number;
  total_ms?: number;
}

export async function batchRenderProject(projectId: number, options?: BatchRenderOptions): Promise<BatchRenderResponse> {
  const body = {
    force: options?.force,
    segment_ids: options?.segmentIds,
  };
  return request<BatchRenderResponse>(`/projects/${projectId}/batch-render`, {
    method: 'POST',
    body: JSON.stringify(body),
  });
}

export async function cancelJob(jobId: string): Promise<void> {
  await request<void>(`/jobs/${jobId}/cancel`, { method: 'PATCH' });
}

/**
 * Render a single project segment on demand.  The backend calls Gemini TTS,
 * saves the audio, creates a SegmentTake, and updates the segment status.
 * Returns the new SegmentTake on success.
 */
export async function reRenderSegment(projectId: number, segmentId: number): Promise<SegmentTake> {
  return request<SegmentTake>(`/projects/${projectId}/segments/${segmentId}/render`, {
    method: 'POST',
  });
}

// --- Pronunciation dictionaries ---

export async function listDictionaries(projectId: number): Promise<PronunciationDictionary[]> {
  return request<PronunciationDictionary[]>(`/projects/${projectId}/dictionaries`);
}

export async function createDictionary(projectId: number, name: string): Promise<PronunciationDictionary> {
  return request<PronunciationDictionary>(`/projects/${projectId}/dictionaries`, {
    method: 'POST',
    body: JSON.stringify({ name }),
  });
}

export async function updateDictionary(projectId: number, dictId: number, name: string): Promise<PronunciationDictionary> {
  return request<PronunciationDictionary>(`/projects/${projectId}/dictionaries/${dictId}`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  });
}

export async function deleteDictionary(projectId: number, dictId: number): Promise<void> {
  await request<void>(`/projects/${projectId}/dictionaries/${dictId}`, { method: 'DELETE' });
}

export async function listEntries(projectId: number, dictId: number): Promise<PronunciationEntry[]> {
  return request<PronunciationEntry[]>(`/projects/${projectId}/dictionaries/${dictId}/entries`);
}

export async function createEntry(projectId: number, dictId: number, input: CreateEntryInput): Promise<PronunciationEntry> {
  return request<PronunciationEntry>(`/projects/${projectId}/dictionaries/${dictId}/entries`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function updateEntry(projectId: number, dictId: number, entryId: number, input: Partial<PronunciationEntry>): Promise<PronunciationEntry> {
  return request<PronunciationEntry>(`/projects/${projectId}/dictionaries/${dictId}/entries/${entryId}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
}

export async function deleteEntry(projectId: number, dictId: number, entryId: number): Promise<void> {
  await request<void>(`/projects/${projectId}/dictionaries/${dictId}/entries/${entryId}`, { method: 'DELETE' });
}

export async function previewDictionary(projectId: number, dictId: number, text: string): Promise<PreviewResult> {
  return request<PreviewResult>(`/projects/${projectId}/dictionaries/${dictId}/preview`, {
    method: 'POST',
    body: JSON.stringify({ text }),
  });
}

export async function listGlobalDictionaries(): Promise<PronunciationDictionary[]> {
  return request<PronunciationDictionary[]>('/pronunciation/dictionaries');
}

export async function createGlobalDictionary(name: string): Promise<PronunciationDictionary> {
  return request<PronunciationDictionary>('/pronunciation/dictionaries', {
    method: 'POST',
    body: JSON.stringify({ name }),
  });
}

export async function updateGlobalDictionary(dictId: number, name: string): Promise<PronunciationDictionary> {
  return request<PronunciationDictionary>(`/pronunciation/dictionaries/${dictId}`, {
    method: 'PUT',
    body: JSON.stringify({ name }),
  });
}

export async function deleteGlobalDictionary(dictId: number): Promise<void> {
  await request<void>(`/pronunciation/dictionaries/${dictId}`, { method: 'DELETE' });
}

export async function listGlobalEntries(dictId: number): Promise<PronunciationEntry[]> {
  return request<PronunciationEntry[]>(`/pronunciation/dictionaries/${dictId}/entries`);
}

export async function createGlobalEntry(dictId: number, input: CreateEntryInput): Promise<PronunciationEntry> {
  return request<PronunciationEntry>(`/pronunciation/dictionaries/${dictId}/entries`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

export async function updateGlobalEntry(dictId: number, entryId: number, input: Partial<PronunciationEntry>): Promise<PronunciationEntry> {
  return request<PronunciationEntry>(`/pronunciation/dictionaries/${dictId}/entries/${entryId}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
}

export async function deleteGlobalEntry(dictId: number, entryId: number): Promise<void> {
  await request<void>(`/pronunciation/dictionaries/${dictId}/entries/${entryId}`, { method: 'DELETE' });
}

export async function previewGlobalDictionary(dictId: number, text: string): Promise<PreviewResult> {
  return request<PreviewResult>(`/pronunciation/dictionaries/${dictId}/preview`, {
    method: 'POST',
    body: JSON.stringify({ text }),
  });
}

// --- History ---

/** A single history entry (TTS generation or recommendation) stored in the backend. */
export interface HistoryEntry {
  id: number;
  type: 'tts' | 'tts_multi' | 'recommendation';
  voice_name: string | null;
  input_text: string;
  result_json: string | null;
  audio_path: string | null;
  created_at: string;
}

/** Optional filters for fetching history entries. */
export interface HistoryFilters {
  type?: string;
  q?: string;
  voice?: string;
  from?: string;
  to?: string;
  limit?: number;
  offset?: number;
}

/** Fetch paginated history entries with optional filters. */
export async function getHistory(filters: HistoryFilters = {}): Promise<HistoryEntry[]> {
  const params = new URLSearchParams();
  if (filters.type) params.set('type', filters.type);
  if (filters.q) params.set('q', filters.q);
  if (filters.voice) params.set('voice', filters.voice);
  if (filters.from) params.set('from', filters.from);
  if (filters.to) params.set('to', filters.to);
  params.set('limit', String(filters.limit ?? 50));
  params.set('offset', String(filters.offset ?? 0));
  return request<HistoryEntry[]>(`/history?${params.toString()}`);
}

/** Fetch a single history entry by ID. */
export async function getHistoryEntry(id: number): Promise<HistoryEntry> {
  return request<HistoryEntry>(`/history/${id}`);
}

/** Delete a single history entry by ID. */
export async function deleteHistoryEntry(id: number): Promise<void> {
  await request<void>(`/history/${id}`, { method: 'DELETE' });
}

/** Retrieve cached TTS audio for a history entry as a base64-encoded PCM string. */
export async function getHistoryAudio(id: number): Promise<string> {
  const data = await request<{ audioBase64: string }>(`/history/${id}/audio`);
  return data.audioBase64;
}

/** Delete all history entries. */
export async function clearHistory(): Promise<void> {
  await request<void>('/history', { method: 'DELETE' });
}

/** Get the export URL for history (opens direct download). */
export function getHistoryExportUrl(format: 'csv' | 'json' = 'json'): string {
  return `${API_BASE}/history/export?format=${format}`;
}

// --- Health ---

/** Check if the backend is running and healthy. */
export async function getHealth(): Promise<{ status: string }> {
  return request<{ status: string }>('/health');
}

// --- Audio Cache ---

/** Cache stats response. */
export interface CacheStats {
  total_size: number;
  file_count: number;
  cache_dir: string;
}

/** Get audio cache statistics. */
export async function getCacheStats(): Promise<CacheStats> {
  return request<CacheStats>('/cache/stats');
}

/** Clear the audio cache. */
export async function clearCache(): Promise<{ status: string; removed: number }> {
  return request<{ status: string; removed: number }>('/cache', { method: 'DELETE' });
}

// --- Backup & Restore ---

/** Create a database backup and download it. */
export async function createBackup(): Promise<void> {
  const response = await fetch(`${API_BASE}/backup`, { method: 'POST' });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Backup failed');
  }
  const blob = await response.blob();
  const disposition = response.headers.get('Content-Disposition') || '';
  const match = disposition.match(/filename="(.+)"/);
  const filename = match ? match[1] : 'gemini-voice-backup.db';
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = filename;
  a.click();
  URL.revokeObjectURL(url);
}

/** Restore the database from an uploaded backup file. */
export async function restoreBackup(file: File): Promise<{ status: string }> {
  const form = new FormData();
  form.append('backup', file);
  const response = await fetch(`${API_BASE}/restore`, {
    method: 'POST',
    body: form,
  });
  if (!response.ok) {
    const text = await response.text();
    throw new Error(text || 'Restore failed');
  }
  return response.json();
}

// --- WebSocket Progress ---

/** Progress event received from the WebSocket. */
export interface ProgressEvent {
  job_id?: string;
  type: string;
  status: string;
  message?: string;
  percent: number;
  item_id?: string;
  project_id?: string;
  segment_id?: string;
  completed_items?: number;
  total_items?: number;
  failed_items?: number;
  error_code?: string;
}

export type ProgressConnectionStatus = 'connecting' | 'connected' | 'disconnected' | 'reconnecting';

/** Persisted job state returned by /api/jobs. */
export interface ApiJob {
  id: string;
  type: string;
  status: string;
  project_id?: string;
  section_id?: string;
  segment_id?: string;
  total_items: number;
  completed_items: number;
  failed_items: number;
  percent: number;
  message?: string;
  error?: string;
  error_code?: string;
  metadata_json?: string;
  created_at: string;
  updated_at: string;
  completed_at?: string;
}

/** Fetch recent persisted jobs for startup reconciliation. */
export async function getJobs(limit: number = 50): Promise<ApiJob[]> {
  const params = new URLSearchParams({ limit: String(limit) });
  return request<ApiJob[]>(`/jobs?${params.toString()}`);
}

/** Connect to the WebSocket progress endpoint. Returns a cleanup function. */
export function connectProgress(
  onEvent: (event: ProgressEvent) => void,
  onStatusChange?: (status: ProgressConnectionStatus) => void,
): () => void {
  // Wails v2's asset server supports HTTP requests but not WebSockets. The
  // native shell therefore relies on the existing persisted-job polling path.
  // Keep the connection state explicit so the Job Center can explain why live
  // updates are unavailable instead of repeatedly attempting a bad socket URL.
  if (window.location.protocol === 'wails:' || window.location.hostname === 'wails.localhost') {
    onStatusChange?.('disconnected');
    return () => {};
  }
  const protocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
  const url = `${protocol}//${window.location.host}/api/ws/progress`;
  let ws: WebSocket | null = null;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  let stopped = false;
  let hasConnected = false;

  const connect = () => {
    if (stopped) return;
    onStatusChange?.(hasConnected ? 'reconnecting' : 'connecting');
    ws = new WebSocket(url);
    ws.onopen = () => {
      hasConnected = true;
      onStatusChange?.('connected');
    };
    ws.onmessage = (e) => {
      try {
        const event = JSON.parse(e.data) as ProgressEvent;
        onEvent(event);
      } catch { /* ignore malformed */ }
    };
    ws.onclose = () => {
      ws = null;
      if (stopped) return;
      onStatusChange?.('disconnected');
      reconnectTimer = setTimeout(connect, 3000);
    };
  };

  connect();

  return () => {
    stopped = true;
    if (reconnectTimer) clearTimeout(reconnectTimer);
    if (ws) {
      ws.onclose = null;
      ws.close();
      ws = null;
    }
  };
}

// --- Custom Presets ---

/** List all custom voice presets. */
export async function listPresets(): Promise<CustomPreset[]> {
  return request<CustomPreset[]>('/presets');
}

/** Get a single custom preset by ID. */
export async function getPreset(id: number): Promise<CustomPreset> {
  return request<CustomPreset>(`/presets/${id}`);
}

/** Create a new custom voice preset, optionally with cached audio. */
export async function createPreset(data: {
  name: string;
  voice_name: string;
  system_instruction?: string;
  sample_text?: string;
  audio_base64?: string;
  source_query?: string;
  metadata_json?: string;
  generate_headshot?: boolean;
  person_description?: string;
}): Promise<{ id: number }> {
  return request<{ id: number }>('/presets', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

/** Update a custom preset's mutable fields. */
export async function updatePreset(id: number, data: {
  name?: string;
  system_instruction?: string;
  sample_text?: string;
  audio_base64?: string;
  metadata_json?: string;
  color?: string;
}): Promise<void> {
  await request<void>(`/presets/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

/** Delete a custom preset and its cached audio. */
export async function deletePreset(id: number): Promise<void> {
  await request<void>(`/presets/${id}`, { method: 'DELETE' });
}

/** Retrieve cached audio for a preset as base64 PCM. */
export async function getPresetAudio(id: number): Promise<string> {
  const data = await request<{ audioBase64: string }>(`/presets/${id}/audio`);
  return data.audioBase64;
}

/** Retrieve a generated preset headshot image URL for direct img src usage. */
export function getPresetImageUrl(id: number): string {
  return `${API_BASE}/presets/${id}/image`;
}

/** Regenerate the headshot for a preset using its stored personDescription. */
export async function regeneratePresetImage(id: number): Promise<CustomPreset> {
  return request<CustomPreset>(`/presets/${id}/image/regenerate`, { method: 'POST' });
}

/** List all distinct tags across all presets. */
export async function listAllTags(): Promise<PresetTag[]> {
  return request<PresetTag[]>('/presets/tags');
}

/** Replace all tags for a given preset. */
export async function setPresetTags(presetId: number, tags: { tag: string; color: string }[]): Promise<void> {
  await request<void>(`/presets/${presetId}/tags`, {
    method: 'PUT',
    body: JSON.stringify({ tags }),
  });
}

/** Export all presets as JSON (triggers download). */
export async function exportPresets(): Promise<void> {
  const res = await fetch(`${API_BASE}/presets/export`);
  if (!res.ok) throw new Error('Export failed');
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = 'voice-presets.json';
  a.click();
  URL.revokeObjectURL(url);
}

/** Import presets from a JSON array. Returns count of imported and skipped. */
export async function importPresets(data: unknown[]): Promise<{ imported: number; skipped: number }> {
  return request<{ imported: number; skipped: number }>('/presets/import', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

/** Reorder presets by providing an ordered array of preset IDs. */
export async function reorderPresets(orderedIds: number[]): Promise<void> {
  await request<void>('/presets/reorder', {
    method: 'PATCH',
    body: JSON.stringify({ ordered_ids: orderedIds }),
  });
}

/** A preset version snapshot. */
export interface PresetVersion {
  id: number;
  preset_id: number;
  name: string;
  voice_name: string;
  system_instruction: string;
  sample_text: string;
  color: string;
  metadata_json?: string;
  created_at: string;
}

/** List version history for a preset. */
export async function listPresetVersions(presetId: number): Promise<PresetVersion[]> {
  return request<PresetVersion[]>(`/presets/${presetId}/versions`);
}

/** Revert a preset to a specific version. Returns the updated preset. */
export async function revertPresetVersion(presetId: number, versionId: number): Promise<CustomPreset> {
  return request<CustomPreset>(`/presets/${presetId}/versions/${versionId}/revert`, { method: 'POST' });
}

// --- Favorites ---

/** List all favorited voice names. */
export async function listFavorites(): Promise<string[]> {
  return request<string[]>('/favorites');
}

/** Add or remove a voice from favorites. */
export async function toggleFavorite(voiceName: string, favorite: boolean): Promise<void> {
  await request<{ ok: boolean }>('/favorites', {
    method: 'POST',
    body: JSON.stringify({ voice_name: voiceName, favorite }),
  });
}

/** Use Gemini to reformat raw script text into optimised TTS prompt structure. */
export async function formatScript(script: string): Promise<string> {
  const data = await request<{ formatted: string }>('/voices/format-script', {
    method: 'POST',
    body: JSON.stringify({ script }),
  });
  return data.formatted;
}

/** Stream TTS audio chunks via SSE. Calls onChunk with each base64 audio fragment. */
export async function streamTts(
  text: string,
  voiceName: string,
  opts: {
    systemInstruction?: string;
    languageCode?: string;
    model?: string;
    onChunk: (audioBase64: string, index: number) => void;
    onDone: () => void;
    onError: (error: string) => void;
    signal?: AbortSignal;
  }
): Promise<void> {
  const body: Record<string, string> = { text, voiceName };
  if (opts.systemInstruction) body.systemInstruction = opts.systemInstruction;
  if (opts.languageCode) body.languageCode = opts.languageCode;
  if (opts.model) body.model = opts.model;

  const resp = await fetch(`${API_BASE}/voices/tts/stream`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
    signal: opts.signal,
  });

  if (!resp.ok) {
    const err = await resp.text();
    throw new Error(err || `HTTP ${resp.status}`);
  }

  const reader = resp.body?.getReader();
  if (!reader) throw new Error('ReadableStream not supported');

  const decoder = new TextDecoder();
  let buffer = '';

  while (true) {
    const { done, value } = await reader.read();
    if (done) break;

    buffer += decoder.decode(value, { stream: true });
    const lines = buffer.split('\n');
    buffer = lines.pop() || '';

    for (const line of lines) {
      if (!line.startsWith('data: ')) continue;
      const json = line.slice(6).trim();
      if (!json) continue;
      try {
        const parsed = JSON.parse(json);
        if (parsed.error) { opts.onError(parsed.error); return; }
        if (parsed.done) { opts.onDone(); return; }
        if (parsed.audioBase64) { opts.onChunk(parsed.audioBase64, parsed.index); }
      } catch {}
    }
  }
  opts.onDone();
}

// --- Export Profiles ---

/** List all export profiles (builtins first). */
export async function listExportProfiles(): Promise<ExportProfile[]> {
  return request<ExportProfile[]>('/export-profiles');
}

/** Fetch a single export profile by ID. */
export async function getExportProfile(id: number): Promise<ExportProfile> {
  return request<ExportProfile>(`/export-profiles/${id}`);
}

/** Create a new custom export profile. */
export async function createExportProfile(data: Omit<ExportProfile, 'id' | 'is_builtin' | 'created_at' | 'updated_at'>): Promise<ExportProfile> {
  return request<ExportProfile>('/export-profiles', {
    method: 'POST',
    body: JSON.stringify(data),
  });
}

/** Update a custom (non-builtin) export profile by ID. */
export async function updateExportProfile(id: number, data: Partial<Omit<ExportProfile, 'id' | 'is_builtin' | 'created_at' | 'updated_at'>>): Promise<ExportProfile> {
  return request<ExportProfile>(`/export-profiles/${id}`, {
    method: 'PUT',
    body: JSON.stringify(data),
  });
}

/** Delete a custom (non-builtin) export profile. */
export async function deleteExportProfile(id: number): Promise<void> {
  await request<void>(`/export-profiles/${id}`, { method: 'DELETE' });
}

// --- Stitch / Audio Export ---

/** Options for stitching a project or section into a single WAV. */
export interface StitchOptions {
  export_profile_id?: number;
  section_id?: number;
}

/**
 * Stitch all renderable segments (or a specific section) into one WAV file.
 * Returns the raw WAV as a Blob for download.
 */
export async function stitchProject(projectId: number, options: StitchOptions = {}): Promise<Blob> {
  const res = await fetch(`${API_BASE}/projects/${projectId}/stitch`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(options),
  });
  if (!res.ok) {
    let msg = 'Stitch failed';
    try { msg = ((await res.json()) as { error: string }).error; } catch { /* ignore */ }
    throw new Error(msg);
  }
  return res.blob();
}

// --- Performance Style Presets ---

/** List all performance styles. Pass projectId to include project-scoped styles. */
export async function listStyles(projectId?: number): Promise<PerformanceStyle[]> {
  const params = projectId !== undefined ? `?project_id=${projectId}` : '';
  return request<PerformanceStyle[]>(`/styles${params}`);
}

/** Create a new performance style. */
export async function createStyle(input: CreateStyleInput): Promise<PerformanceStyle> {
  return request<PerformanceStyle>('/styles', {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

/** Get a single performance style by ID. */
export async function getStyle(id: number): Promise<PerformanceStyle> {
  return request<PerformanceStyle>(`/styles/${id}`);
}

/** Update a performance style (creates a version snapshot). */
export async function updateStyle(id: number, input: UpdateStyleInput): Promise<PerformanceStyle> {
  return request<PerformanceStyle>(`/styles/${id}`, {
    method: 'PUT',
    body: JSON.stringify(input),
  });
}

/** Delete a user-created performance style. Fails for builtins. */
export async function deleteStyle(id: number): Promise<void> {
  await request<void>(`/styles/${id}`, { method: 'DELETE' });
}

/** List version history for a performance style. */
export async function listStyleVersions(id: number): Promise<PerformanceStyleVersion[]> {
  return request<PerformanceStyleVersion[]>(`/styles/${id}/versions`);
}

/** Revert a performance style to a specific version. */
export async function revertStyleVersion(id: number, versionId: number): Promise<PerformanceStyle> {
  return request<PerformanceStyle>(`/styles/${id}/versions/${versionId}/revert`, { method: 'POST' });
}

// ---------------------------------------------------------------------------
// QC / Review workflow
// ---------------------------------------------------------------------------

/** List all QC issues for a project. Pass status to filter (open|resolved|wont_fix). */
export async function listProjectQcIssues(projectId: number, status?: string): Promise<QcIssue[]> {
  const params = status ? `?status=${encodeURIComponent(status)}` : '';
  return request<QcIssue[]>(`/projects/${projectId}/qc${params}`);
}

/** Create a new QC issue for a project segment. */
export async function createQcIssue(projectId: number, input: CreateQcIssueInput): Promise<QcIssue> {
  return request<QcIssue>(`/projects/${projectId}/qc`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
}

/** Get a single QC issue by ID. */
export async function getQcIssue(issueId: number): Promise<QcIssue> {
  return request<QcIssue>(`/qc/${issueId}`);
}

/** Update a QC issue (severity, note, status, etc.). */
export async function updateQcIssue(issueId: number, input: UpdateQcIssueInput): Promise<QcIssue> {
  return request<QcIssue>(`/qc/${issueId}`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  });
}

/** Delete a QC issue permanently. */
export async function deleteQcIssue(issueId: number): Promise<void> {
  return request<void>(`/qc/${issueId}`, { method: 'DELETE' });
}

/** Resolve a QC issue. */
export async function resolveQcIssue(issueId: number): Promise<QcIssue> {
  return request<QcIssue>(`/qc/${issueId}/resolve`, { method: 'POST' });
}

/** Get aggregate open/resolved/wont_fix counts for a project. */
export async function getProjectQcRollup(projectId: number): Promise<QcRollup> {
  return request<QcRollup>(`/projects/${projectId}/qc/rollup`);
}

/** Export all QC issues as CSV or Markdown. Returns a Blob. */
export async function exportQcIssues(projectId: number, format: 'csv' | 'markdown' = 'csv'): Promise<Blob> {
  const res = await fetch(`${API_BASE}/projects/${projectId}/qc/export?format=${format}`);
  if (!res.ok) throw new Error(`Export failed: ${res.status}`);
  return res.blob();
}

/** Approve a take (sets status to "approved"). */
export async function approveTake(projectId: number, takeId: number): Promise<import('./types').SegmentTake> {
  return request(`/projects/${projectId}/takes/${takeId}/approve`, { method: 'POST' });
}

/** Flag a take for review (sets status to "flagged"). */
export async function flagTake(projectId: number, takeId: number): Promise<import('./types').SegmentTake> {
  return request(`/projects/${projectId}/takes/${takeId}/flag`, { method: 'POST' });
}

// ── Plan 09: Client & Brand Voiceover Workspaces ──────────────────────────────

/** List all clients. */
export async function listClients(): Promise<import('./types').Client[]> {
  return request('/clients');
}

/** Create a new client. */
export async function createClient(input: import('./types').CreateClientInput): Promise<import('./types').Client> {
  return request('/clients', { method: 'POST', body: JSON.stringify(input) });
}

/** Get a single client by ID. */
export async function getClient(id: number): Promise<import('./types').Client> {
  return request(`/clients/${id}`);
}

/** Update a client. */
export async function updateClient(id: number, input: import('./types').UpdateClientInput): Promise<import('./types').Client> {
  return request(`/clients/${id}`, { method: 'PUT', body: JSON.stringify(input) });
}

/** Delete a client. */
export async function deleteClient(id: number): Promise<void> {
  return request(`/clients/${id}`, { method: 'DELETE' });
}

/** List all assets linked to a client. */
export async function listClientAssets(clientId: number): Promise<import('./types').ClientAsset[]> {
  return request(`/clients/${clientId}/assets`);
}

/** Link an asset to a client. */
export async function addClientAsset(clientId: number, input: import('./types').CreateClientAssetInput): Promise<import('./types').ClientAsset> {
  return request(`/clients/${clientId}/assets`, { method: 'POST', body: JSON.stringify(input) });
}

/** Unlink an asset from a client. */
export async function removeClientAsset(clientId: number, assetId: number): Promise<void> {
  return request(`/clients/${clientId}/assets/${assetId}`, { method: 'DELETE' });
}

// ── Plan 10: Provider and Model Strategy ─────────────────────────────────────

/** List all registered TTS providers with key-configured status. */
export async function listProviders(): Promise<import('./types').ProviderInfo[]> {
  return request('/providers');
}

// ── Plan 11: Deliverable Packaging ───────────────────────────────────────────

/** Start a new export job for a project. Returns the created job (status: pending). */
export async function startExport(projectId: number, options?: { export_profile_id?: number }): Promise<import('./types').ExportJob> {
  return request<import('./types').ExportJob>(`/projects/${projectId}/exports`, {
    method: 'POST',
    body: JSON.stringify(options ?? {}),
  });
}

/** Poll a single export job by ID. */
export async function getExport(exportId: number): Promise<import('./types').ExportJob> {
  return request<import('./types').ExportJob>(`/exports/${exportId}`);
}

/** List all export jobs for a project. */
export async function listExports(projectId: number): Promise<import('./types').ExportJob[]> {
  return request<import('./types').ExportJob[]>(`/projects/${projectId}/exports`);
}

/**
 * Trigger a browser download of the completed export ZIP.
 * Creates an object URL from the streamed response, fires an anchor click,
 * then revokes the URL.
 */
export async function downloadExport(exportId: number): Promise<void> {
  const res = await fetch(`${API_BASE}/exports/${exportId}/download`);
  if (!res.ok) {
    let msg = 'Download failed';
    try { msg = ((await res.json()) as { error: string }).error; } catch { /* ignore */ }
    throw new Error(msg);
  }
  const blob = await res.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement('a');
  a.href = url;
  a.download = `export-${exportId}.zip`;
  document.body.appendChild(a);
  a.click();
  document.body.removeChild(a);
  URL.revokeObjectURL(url);
}

// ── AI Script Prep (Plan 12) ──────────────────────────────────────────────

/** Submit a raw script to AI prep and return the resulting job with result JSON. */
export async function prepareScript(
  projectId: number,
  rawScript: string,
  options?: import('./types').ScriptPrepOptions,
): Promise<import('./types').ScriptPrepJob> {
  return request<import('./types').ScriptPrepJob>(`/projects/${projectId}/prepare-script`, {
    method: 'POST',
    body: JSON.stringify({ raw_script: rawScript, options: options ?? {} }),
  });
}

/** Get the most recent AI prep job for a project. Returns null if none. */
export async function getLatestScriptPrep(
  projectId: number,
): Promise<import('./types').ScriptPrepJob | null> {
  try {
    return await request<import('./types').ScriptPrepJob>(`/projects/${projectId}/prepare-script`);
  } catch (err) {
    if (err instanceof Error && err.message.includes('404')) return null;
    throw err;
  }
}

/** Apply a reviewed AI prep result to a project. */
export async function applyScriptPrep(
  projectId: number,
  input: import('./types').ScriptPrepApplyInput,
): Promise<import('./types').ScriptPrepApplyResult> {
  return request<import('./types').ScriptPrepApplyResult>(`/projects/${projectId}/script-prep/apply`, {
    method: 'POST',
    body: JSON.stringify(input),
  });
}

