/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

/** Project-level render defaults backed by the server provider registry. */
import React, { useEffect, useMemo, useState } from 'react';
import { Check, Loader2, X } from 'lucide-react';
import { listProviders } from '../api';
import { CustomPreset, PerformanceStyle, ProviderModel, ScriptProject, Voice } from '../types';
import StylePresetPicker from './StylePresetPicker';

const FALLBACK_MODELS: ProviderModel[] = [
  { id: 'gemini-3.1-flash-tts-preview', display_name: 'Gemini 3.1 Flash TTS', is_default: true },
  { id: 'gemini-2.5-flash-preview-tts', display_name: 'Gemini 2.5 Flash TTS' },
  { id: 'gemini-2.5-pro-preview-tts', display_name: 'Gemini 2.5 Pro TTS' },
];

interface ProjectSettingsPanelProps {
  selectedProject: ScriptProject;
  voices: Voice[];
  customPresets?: CustomPreset[];
  styles: PerformanceStyle[];
  settingsVoice: string;
  settingsLang: string;
  settingsModel: string;
  settingsStyleId: number | null;
  savingSettings: boolean;
  onChangeVoice: (value: string) => void;
  onChangeLang: (value: string) => void;
  onChangeModel: (value: string) => void;
  onChangeStyleId: (id: number | null) => void;
  onStyleCreated: (style: PerformanceStyle) => void;
  onSave: () => void;
  onClose: () => void;
  mobile?: boolean;
  showHeaderClose?: boolean;
  dirty?: boolean;
}

/** Render editable defaults for the selected script project. */
const ProjectSettingsPanel: React.FC<ProjectSettingsPanelProps> = ({
  selectedProject,
  voices,
  customPresets = [],
  styles,
  settingsVoice,
  settingsLang,
  settingsModel,
  settingsStyleId,
  savingSettings,
  onChangeVoice,
  onChangeLang,
  onChangeModel,
  onChangeStyleId,
  onStyleCreated,
  onSave,
  onClose,
  mobile = false,
  showHeaderClose = true,
  dirty = true,
}) => {
  const [models, setModels] = useState<ProviderModel[]>(FALLBACK_MODELS);

  useEffect(() => {
    let active = true;
    listProviders()
      .then(providers => {
        const gemini = providers.find(provider => provider.id === 'gemini');
        if (active && gemini?.models.length) setModels(gemini.models);
      })
      .catch(() => {
        // The fallback catalogue keeps project settings usable during backend startup.
      });
    return () => { active = false; };
  }, []);

  const modelOptions = useMemo(() => {
    if (!settingsModel || models.some(model => model.id === settingsModel)) return models;
    return [{ id: settingsModel, display_name: `${settingsModel} (legacy)` }, ...models];
  }, [models, settingsModel]);

  return (
    <div className={mobile ? 'space-y-4' : 'rounded-xl border border-zinc-200 dark:border-zinc-800 bg-zinc-50 dark:bg-zinc-900/50 p-4 space-y-4'}>
      <div className="flex items-center justify-between gap-2">
        <p className="text-sm font-semibold text-zinc-800 dark:text-zinc-100">Project defaults</p>
        {showHeaderClose && (
          <button
            type="button"
            onClick={onClose}
            aria-label="Close project defaults"
            className="shrink-0 inline-flex h-7 w-7 items-center justify-center rounded-md text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-100 transition-colors"
          >
            <X size={14} />
          </button>
        )}
      </div>

      <div className="space-y-4">
        <section className="space-y-2">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">Voice defaults</p>
          <div className="space-y-1">
            <label className="block text-[11px] font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
              Default voice
            </label>
            <select
              value={settingsVoice}
              onChange={event => onChangeVoice(event.target.value)}
              className="h-9 w-full rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2.5 text-sm text-zinc-700 dark:text-zinc-200 focus:outline-none focus:ring-2 focus:ring-[var(--accent-100)]"
            >
              <option value="">— None —</option>
              {voices.length > 0 && (
                <optgroup label="Stock voices">
                  {voices.map(voice => <option key={voice.name} value={voice.name}>{voice.name}</option>)}
                </optgroup>
              )}
              {customPresets.length > 0 && (
                <optgroup label="My voices">
                  {customPresets.map(preset => (
                    <option key={`preset:${preset.id}`} value={`preset:${preset.id}`}>{preset.name}</option>
                  ))}
                </optgroup>
              )}
            </select>
          </div>
        </section>

        <section className="grid grid-cols-1 gap-3 sm:grid-cols-2">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400 sm:col-span-2">Language and model</p>
          <div className="space-y-1">
            <label className="block text-[11px] font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
              Language code
            </label>
            <input
              value={settingsLang}
              onChange={event => onChangeLang(event.target.value)}
              placeholder="e.g. en-US"
              className="h-9 w-full rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-3 text-sm text-zinc-900 dark:text-white placeholder:text-zinc-400 focus:outline-none focus:ring-2 focus:ring-[var(--accent-100)]"
            />
          </div>
          <div className="space-y-1">
            <label className="block text-[11px] font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">
              Model
            </label>
            <select
              value={settingsModel}
              onChange={event => onChangeModel(event.target.value)}
              className="h-9 w-full rounded-lg border border-zinc-200 dark:border-zinc-800 bg-white dark:bg-zinc-950 px-2.5 text-sm text-zinc-700 dark:text-zinc-200 focus:outline-none focus:ring-2 focus:ring-[var(--accent-100)]"
            >
              {modelOptions.map(model => (
                <option key={model.id} value={model.id}>{model.display_name}</option>
              ))}
            </select>
            <p className="text-[11px] text-zinc-500 dark:text-zinc-400">
              Model availability is loaded from the backend provider registry.
            </p>
          </div>
        </section>

        <section className="space-y-1">
          <p className="text-[11px] font-semibold uppercase tracking-wide text-zinc-500 dark:text-zinc-400">Performance style</p>
          <StylePresetPicker
            styles={styles}
            value={settingsStyleId}
            onChange={onChangeStyleId}
            projectId={selectedProject.id}
            onStyleCreated={onStyleCreated}
          />
        </section>
      </div>

      <div className={mobile ? 'sticky bottom-0 -mx-4 flex justify-end gap-2 border-t border-zinc-200 dark:border-zinc-800 bg-white/95 dark:bg-zinc-950/95 px-4 py-3 backdrop-blur' : 'flex justify-end gap-2 pt-1'}>
        <button
          type="button"
          onClick={onClose}
          className="inline-flex h-9 items-center gap-1.5 rounded-lg border border-zinc-200 dark:border-zinc-800 px-3 text-xs font-semibold text-zinc-600 dark:text-zinc-300 hover:bg-zinc-50 dark:hover:bg-zinc-900 transition-colors"
        >
          Cancel
        </button>
        <button
          type="button"
          disabled={savingSettings || !dirty}
          onClick={onSave}
          className="inline-flex h-9 items-center gap-1.5 rounded-lg bg-zinc-900 dark:bg-[var(--accent-600)] px-4 text-xs font-semibold text-white hover:bg-zinc-700 dark:hover:bg-[var(--accent-500)] transition-colors disabled:opacity-50"
        >
          {savingSettings ? <Loader2 size={13} className="animate-spin" /> : <Check size={13} />}
          Save settings
        </button>
      </div>
    </div>
  );
};

export default ProjectSettingsPanel;
