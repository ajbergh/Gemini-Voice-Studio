/**
 * @license
 * SPDX-License-Identifier: Apache-2.0
 */

/**
 * ScriptReaderModal.tsx — Custom Script Testing Modal
 *
 * Modal or inline section for testing TTS with custom text. Users can type,
 * paste, or drag-and-drop a script and preview it with any stock or custom
 * voice using the embedded AiTtsPreview component. Supports a Stock / My Voices
 * tab toggle, an accent selector (16 world accents via Director's Notes),
 * multi-speaker dialogue mode, voice comparison, audio tag insertion with
 * syntax highlighting via ScriptHighlighter, script templates, and AI-powered
 * script formatting. Implements focus trap and Escape-to-close for accessibility.
 */

import React, { useState, useEffect, useRef, useMemo, useCallback } from 'react';
import { Voice, CustomPreset } from '../types';
import { X, FileText, Mic, User, ChevronDown, Users, Play, Square, Loader2, Download, BookOpen, Clock, Wand2, Save } from 'lucide-react';
import { VOICE_DATA } from '../constants';
import { generateMultiSpeakerTts, formatScript } from '../api';
import AiTtsPreview from './AiTtsPreview';
import AudioTagsToolbar from './AudioTagsToolbar';
import ScriptHighlighter from './ScriptHighlighter';
import VoiceCompare from './VoiceCompare';

/** Available TTS accents with Director's Notes prompts for Gemini 3.1. All accents produce English speech. */
const ACCENT_TRANSCRIPT_GUARD = 'Keep the wording exactly as written. Do not translate or paraphrase.';

const TTS_ACCENTS: { id: string; label: string; directorsNote: string; speechLanguageCode: string }[] = [
  { id: '', label: 'No Accent', directorsNote: '', speechLanguageCode: '' },
  { id: 'general-american', label: 'General American', directorsNote: `Read the transcript in English with a neutral General American accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-US' },
  { id: 'british-rp', label: 'British (RP)', directorsNote: `Read the transcript in English with a formal British Received Pronunciation accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-GB' },
  { id: 'london-brixton', label: 'London (Brixton)', directorsNote: `Read the transcript in English with a South London Brixton accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-GB' },
  { id: 'australian', label: 'Australian', directorsNote: `Read the transcript in English with a General Australian accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-AU' },
  { id: 'indian-english', label: 'Indian English', directorsNote: `Read the transcript in English with an Indian English accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-IN' },
  { id: 'canadian', label: 'Canadian', directorsNote: `Read the transcript in English with a Canadian accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-CA' },
  { id: 'irish', label: 'Irish', directorsNote: `Read the transcript in English with a Dublin Irish accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-IE' },
  { id: 'scottish', label: 'Scottish', directorsNote: `Read the transcript in English with an Edinburgh Scottish accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-GB' },
  { id: 'south-african', label: 'South African', directorsNote: `Read the transcript in English with a Cape Town South African accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en-ZA' },
  { id: 'castilian-spanish', label: 'Spanish (Castilian)', directorsNote: `Read the transcript in English with a Castilian Spanish accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en' },
  { id: 'mexican-spanish', label: 'Spanish (Mexican)', directorsNote: `Read the transcript in English with a Mexican Spanish accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en' },
  { id: 'parisian-french', label: 'French (Parisian)', directorsNote: `Read the transcript in English with a Parisian French accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en' },
  { id: 'quebecois-french', label: 'French (Québécois)', directorsNote: `Read the transcript in English with a Québécois French accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en' },
  { id: 'standard-german', label: 'German (Standard)', directorsNote: `Read the transcript in English with a standard German accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en' },
  { id: 'tokyo-japanese', label: 'Japanese (Tokyo)', directorsNote: `Read the transcript in English with a Tokyo Japanese accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en' },
  { id: 'mandarin', label: 'Mandarin Chinese', directorsNote: `Read the transcript in English with a Mandarin Chinese accent. ${ACCENT_TRANSCRIPT_GUARD}`, speechLanguageCode: 'en' },
];

/** Pre-loaded script templates for common use cases. */
const SCRIPT_TEMPLATES: { label: string; category: string; content: string }[] = [
  {
    label: 'Podcast Intro',
    category: 'Podcast',
    content: `## Audio Profile\nWarm, conversational podcast host with a welcoming tone\n\n## Scene\nRecording studio — relaxed morning energy\n\n## Transcript\nHey everyone, welcome back to the show! I'm so glad you're here today. We've got an incredible episode lined up — we're diving deep into a topic that I know a lot of you have been asking about. So grab your coffee, get comfortable, and let's get into it.`,
  },
  {
    label: 'Audiobook Narration',
    category: 'Audiobook',
    content: `## Audio Profile\nCalm, measured narrator with rich storytelling presence\n\n## Scene\nQuiet library atmosphere — intimate narration\n\n## Transcript\nThe morning light filtered through the curtains, casting long golden shadows across the wooden floor. She sat at the kitchen table, her hands wrapped around a mug of tea that had long since gone cold. Outside, the world carried on — birds sang, cars hummed in the distance — but in here, time had stopped.`,
  },
  {
    label: 'Commercial / Ad Read',
    category: 'Commercial',
    content: `## Audio Profile\nUpbeat, energetic announcer with clear enunciation\n\n## Scene\nBright, modern ad break\n\n## Transcript\nIntroducing the all-new SmartHome Pro — the only smart home system that learns your routine and adapts to your lifestyle. From adjusting the lights to brewing your morning coffee, SmartHome Pro does it all. Visit smarthomepro.com today and get 30% off your first setup. SmartHome Pro — living made effortless.`,
  },
  {
    label: 'News Broadcast',
    category: 'News',
    content: `## Audio Profile\nAuthoritative, clear news anchor with steady pacing\n\n## Scene\nProfessional newsroom — breaking news energy\n\n## Transcript\nGood evening. Tonight's top story — researchers at the Global Institute of Technology have announced a breakthrough in renewable energy storage that could revolutionize how we power our cities. The new battery technology, developed over five years of intensive research, stores three times more energy than current lithium-ion cells at half the cost. We'll have a full report coming up.`,
  },
  {
    label: 'Meditation Guide',
    category: 'Wellness',
    content: `## Audio Profile\nSoft, gentle, soothing voice with slow deliberate pacing\n\n## Scene\nPeaceful garden — birds and gentle breeze\n\n## Director's Notes\nSpeak slowly. Leave natural pauses between sentences. Use a warm, calming tone throughout.\n\n## Transcript\n[softly] Welcome. [pause: 2s] Find a comfortable position and gently close your eyes. [pause: 3s] Take a deep breath in through your nose... [pause: 2s] and slowly release it through your mouth. [pause: 3s] With each breath, feel your body becoming lighter, more relaxed. You are safe. You are present. You are here.`,
  },
  {
    label: 'Tutorial / How-To',
    category: 'Education',
    content: `## Audio Profile\nFriendly, patient instructor with clear step-by-step delivery\n\n## Scene\nOnline course — screen recording narration\n\n## Transcript\nAlright, let's walk through this step by step. First, open your terminal and navigate to your project directory. You should see a file called config.json — go ahead and open that up. Now, look for the line that says "apiKey" — this is where we'll paste in the key we generated in the previous step. Make sure you save the file when you're done. Perfect — you're all set!`,
  },
  {
    label: 'Character Voice (Villain)',
    category: 'Character',
    content: `## Audio Profile\nDeep, menacing voice with slow, deliberate delivery\n\n## Scene\nDark throne room — echoing stone walls\n\n## Director's Notes\nSpeak with calculated confidence. Each word should feel deliberate and weighted.\n\n## Transcript\n[in a low, menacing tone] You really thought you could stop me? How... [chuckles darkly] ...delightfully naive. I've been planning this for longer than you can possibly imagine. Every move you've made, every ally you've gathered — all part of my design. And now, here we are. At the end.`,
  },
  {
    label: 'Dialogue Example',
    category: 'Dialogue',
    content: `Speaker1: Hey, did you hear about the new restaurant that opened downtown?\nSpeaker2: Oh yeah! I've been meaning to check it out. Is it any good?\nSpeaker1: It's amazing. The pasta is incredible and the atmosphere is so cozy.\nSpeaker2: We should go this weekend! I'll make a reservation.\nSpeaker1: Perfect, let's do Saturday evening. I'll text the group.`,
  },
];

interface ScriptReaderModalProps {
  voices: Voice[];
  customPresets?: CustomPreset[];
  initialVoiceName?: string;
  onClose: () => void;
  /** When true, renders as inline section instead of modal overlay. */
  inline?: boolean;
}

interface DialogueCast {
  id: string;
  name: string;
  speakers: { label: string; voiceName: string }[];
}

const DIALOGUE_CASTS_KEY = 'gemini-voice-dialogue-casts';

/** Render a script-input modal for single- or multi-speaker TTS generation. */
const ScriptReaderModal: React.FC<ScriptReaderModalProps> = ({ voices, customPresets = [], initialVoiceName, onClose, inline = false }) => {
  const [script, setScript] = useState('Hello! I am ready to read your script. Type something here and click Listen.');
  const [showTemplates, setShowTemplates] = useState(false);
  const [isDragOver, setIsDragOver] = useState(false);
  const [recentScripts, setRecentScripts] = useState<string[]>(() => {
    try { return JSON.parse(localStorage.getItem('recentScripts') || '[]'); } catch { return []; }
  });
  const [showRecents, setShowRecents] = useState(false);
  const [isFormatting, setIsFormatting] = useState(false);
  const [voiceSource, setVoiceSource] = useState<'stock' | 'custom'>('stock');
  const [selectedAccentId, setSelectedAccentId] = useState('');
  const [mode, setMode] = useState<'single' | 'dialogue' | 'compare'>('single');
  const [speaker1Name, setSpeaker1Name] = useState('Speaker1');
  const [speaker2Name, setSpeaker2Name] = useState('Speaker2');
  const [speaker1Voice, setSpeaker1Voice] = useState(voices[0]?.name || '');
  const [speaker2Voice, setSpeaker2Voice] = useState(voices[1]?.name || voices[0]?.name || '');
  const [savedDialogueCasts, setSavedDialogueCasts] = useState<DialogueCast[]>(() => {
    try {
      const parsed = JSON.parse(localStorage.getItem(DIALOGUE_CASTS_KEY) || '[]');
      return Array.isArray(parsed) ? parsed : [];
    } catch { return []; }
  });
  const [newCastName, setNewCastName] = useState('');
  const [selectedCastId, setSelectedCastId] = useState('');
  const [dialogueLoading, setDialogueLoading] = useState(false);
  const [dialoguePlaying, setDialoguePlaying] = useState(false);
  const [dialogueError, setDialogueError] = useState<string | null>(null);
  const [dialogueAudio, setDialogueAudio] = useState<string | null>(null);
  const dialogueAudioCtxRef = useRef<AudioContext | null>(null);
  const dialogueSourceRef = useRef<AudioBufferSourceNode | null>(null);
  const isMountedRef = useRef(true);
  const modalRef = useRef<HTMLDivElement>(null);
  const textareaRef = useRef<HTMLTextAreaElement>(null);
  const [highlighterScrollTop, setHighlighterScrollTop] = useState(0);

  const scriptWordCount = useMemo(
    () => script.trim().split(/\s+/).filter(Boolean).length,
    [script],
  );
  const scriptEstimatedSeconds = Math.max(1, Math.round((scriptWordCount / 150) * 60));

  const persistDialogueCasts = (casts: DialogueCast[]) => {
    setSavedDialogueCasts(casts);
    try { localStorage.setItem(DIALOGUE_CASTS_KEY, JSON.stringify(casts)); } catch {}
  };

  const handleSaveDialogueCast = () => {
    const name = newCastName.trim();
    if (!name) return;
    const cast: DialogueCast = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`,
      name,
      speakers: [
        { label: speaker1Name.trim() || 'Speaker1', voiceName: speaker1Voice },
        { label: speaker2Name.trim() || 'Speaker2', voiceName: speaker2Voice },
      ],
    };
    persistDialogueCasts([cast, ...savedDialogueCasts.filter(existing => existing.name.toLowerCase() !== name.toLowerCase())].slice(0, 12));
    setSelectedCastId(cast.id);
    setNewCastName('');
  };

  const handleLoadDialogueCast = (castId: string) => {
    setSelectedCastId(castId);
    const cast = savedDialogueCasts.find(item => item.id === castId);
    if (!cast) return;
    const [first, second] = cast.speakers;
    if (first) {
      setSpeaker1Name(first.label);
      setSpeaker1Voice(first.voiceName);
    }
    if (second) {
      setSpeaker2Name(second.label);
      setSpeaker2Voice(second.voiceName);
    }
  };

  // Auto-resize textarea to fit content up to max-height
  const lastAutoHeightRef = useRef('');
  useEffect(() => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    // If the height no longer matches what we last set, the user manually
    // dragged the resize handle — respect that instead of overwriting it.
    if (textarea.style.height !== lastAutoHeightRef.current) {
      setHighlighterScrollTop(textarea.scrollTop);
      return;
    }
    const prevScrollTop = textarea.scrollTop;
    textarea.style.height = 'auto';
    const nextHeight = `${Math.min(textarea.scrollHeight, 384)}px`;
    textarea.style.height = nextHeight;
    lastAutoHeightRef.current = nextHeight;
    // Setting height to 'auto' collapses the box and resets scrollTop; restore it
    // so typing further down a long script doesn't snap the view back to the top.
    textarea.scrollTop = prevScrollTop;
    setHighlighterScrollTop(textarea.scrollTop);
  }, [script]);

  /**
   * Replace [rangeStart, rangeEnd) of the script with `text` as a native edit
   * (via execCommand) rather than only updating React state directly. Programmatic
   * `.value` writes are invisible to the browser's undo stack, so Templates,
   * Recents, Format, and tag-insertion would otherwise make Ctrl/Cmd+Z a no-op.
   */
  const applyScriptEdit = (text: string, rangeStart: number, rangeEnd: number) => {
    const textarea = textareaRef.current;
    if (!textarea) { setScript(script.slice(0, rangeStart) + text + script.slice(rangeEnd)); return; }
    textarea.focus();
    textarea.setSelectionRange(rangeStart, rangeEnd);
    if (!document.execCommand('insertText', false, text)) {
      setScript(script.slice(0, rangeStart) + text + script.slice(rangeEnd));
    }
  };

  const handleInsertTag = (tag: string) => {
    const textarea = textareaRef.current;
    if (!textarea) return;
    applyScriptEdit(`${tag} `, textarea.selectionStart, textarea.selectionEnd);
  };

  useEffect(() => {
    isMountedRef.current = true;
    return () => {
      isMountedRef.current = false;
      stopDialogueAudio();
      if (dialogueAudioCtxRef.current && dialogueAudioCtxRef.current.state !== 'closed') {
        dialogueAudioCtxRef.current.close().catch(console.error);
      }
    };
  }, []);

  useEffect(() => {
    modalRef.current?.focus();
  }, []);

  useEffect(() => {
    const handleKeyDown = (e: KeyboardEvent) => {
      if (e.key === 'Escape') {
        onClose();
      }
    };

    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  }, [onClose]);

  // Reset dialogue audio when script or voices change
  useEffect(() => {
    setDialogueAudio(null);
    stopDialogueAudio();
  }, [script, speaker1Voice, speaker2Voice, speaker1Name, speaker2Name]);

  const stopDialogueAudio = useCallback(() => {
    if (dialogueSourceRef.current) {
      try { dialogueSourceRef.current.stop(); } catch (e) {}
      dialogueSourceRef.current = null;
    }
    if (isMountedRef.current) setDialoguePlaying(false);
  }, []);

  /** Save current script to recents (max 5, deduplicated). */
  const saveToRecents = useCallback((text: string) => {
    if (text.trim().length < 10) return;
    setRecentScripts(prev => {
      const trimmed = text.trim();
      const next = [trimmed, ...prev.filter(s => s !== trimmed)].slice(0, 5);
      try { localStorage.setItem('recentScripts', JSON.stringify(next)); } catch {}
      return next;
    });
  }, []);

  /** Send current script to Gemini for TTS-optimised formatting. */
  const handleFormatScript = async () => {
    if (!script.trim() || isFormatting) return;
    setIsFormatting(true);
    try {
      const formatted = await formatScript(script);
      if (isMountedRef.current) applyScriptEdit(formatted, 0, script.length);
    } catch (err) {
      console.error('Format script error:', err);
    } finally {
      if (isMountedRef.current) setIsFormatting(false);
    }
  };

  const handleDialoguePlay = async () => {
    if (dialogueLoading || !script.trim()) return;
    if (dialoguePlaying) { stopDialogueAudio(); return; }

    setDialogueLoading(true);
    setDialogueError(null);
    saveToRecents(script);

    try {
      let audioData = dialogueAudio;
      if (!audioData) {
        audioData = await generateMultiSpeakerTts(script, [
          { speaker: speaker1Name, voiceName: speaker1Voice },
          { speaker: speaker2Name, voiceName: speaker2Voice },
        ]);
        if (!isMountedRef.current) return;
        setDialogueAudio(audioData);
      }

      if (!dialogueAudioCtxRef.current || dialogueAudioCtxRef.current.state === 'closed') {
        dialogueAudioCtxRef.current = new (window.AudioContext || (window as any).webkitAudioContext)({ sampleRate: 24000 });
      } else if (dialogueAudioCtxRef.current.state === 'suspended') {
        await dialogueAudioCtxRef.current.resume();
      }

      const binaryString = atob(audioData);
      const bytes = new Uint8Array(binaryString.length);
      for (let i = 0; i < binaryString.length; i++) bytes[i] = binaryString.charCodeAt(i);

      const dataInt16 = new Int16Array(bytes.buffer);
      const frameCount = dataInt16.length;
      const buffer = dialogueAudioCtxRef.current.createBuffer(1, frameCount, 24000);
      const channelData = buffer.getChannelData(0);
      for (let i = 0; i < frameCount; i++) channelData[i] = dataInt16[i] / 32768.0;

      const source = dialogueAudioCtxRef.current.createBufferSource();
      source.buffer = buffer;
      source.connect(dialogueAudioCtxRef.current.destination);
      source.onended = () => { if (isMountedRef.current) setDialoguePlaying(false); };
      dialogueSourceRef.current = source;
      source.start();
      setDialoguePlaying(true);
    } catch (err: any) {
      console.error('Dialogue TTS Error:', err);
      if (isMountedRef.current) setDialogueError('Failed to generate dialogue. Please try again.');
    } finally {
      if (isMountedRef.current) setDialogueLoading(false);
    }
  };

  const handleDialogueDownload = async () => {
    if (dialogueLoading || !script.trim()) return;
    setDialogueLoading(true);
    setDialogueError(null);

    try {
      let audioData = dialogueAudio;
      if (!audioData) {
        audioData = await generateMultiSpeakerTts(script, [
          { speaker: speaker1Name, voiceName: speaker1Voice },
          { speaker: speaker2Name, voiceName: speaker2Voice },
        ]);
        if (!isMountedRef.current) return;
        setDialogueAudio(audioData);
      }

      const binaryString = atob(audioData);
      const pcmData = new Uint8Array(binaryString.length);
      for (let i = 0; i < binaryString.length; i++) pcmData[i] = binaryString.charCodeAt(i);

      const wavBuffer = new ArrayBuffer(44 + pcmData.length);
      const view = new DataView(wavBuffer);
      const writeStr = (o: number, s: string) => { for (let i = 0; i < s.length; i++) view.setUint8(o + i, s.charCodeAt(i)); };
      writeStr(0, 'RIFF'); view.setUint32(4, 36 + pcmData.length, true); writeStr(8, 'WAVE');
      writeStr(12, 'fmt '); view.setUint32(16, 16, true); view.setUint16(20, 1, true);
      view.setUint16(22, 1, true); view.setUint32(24, 24000, true); view.setUint32(28, 48000, true);
      view.setUint16(32, 2, true); view.setUint16(34, 16, true);
      writeStr(36, 'data'); view.setUint32(40, pcmData.length, true);
      new Uint8Array(wavBuffer, 44).set(pcmData);

      const blob = new Blob([wavBuffer], { type: 'audio/wav' });
      const url = URL.createObjectURL(blob);
      const a = document.createElement('a');
      a.href = url; a.download = `dialogue-${Date.now()}.wav`;
      document.body.appendChild(a); a.click(); document.body.removeChild(a);
      URL.revokeObjectURL(url);
    } catch (err) {
      console.error('Download Error:', err);
      if (isMountedRef.current) setDialogueError('Failed to download audio.');
    } finally {
      if (isMountedRef.current) setDialogueLoading(false);
    }
  };

  // Re-order voices so the initialVoiceName is first, because AiTtsPreview defaults to voices[0]
  const sortedVoices = useMemo(() => [...voices].sort((a, b) => {
    if (a.name === initialVoiceName) return -1;
    if (b.name === initialVoiceName) return 1;
    return 0;
  }), [voices, initialVoiceName]);

  // Build voice list and display names for custom presets mode
  const presetVoicesForTts: Voice[] = useMemo(() => {
    const seen = new Set<string>();
    const result: Voice[] = [];
    for (const p of customPresets) {
      if (seen.has(p.voice_name)) continue;
      const v = VOICE_DATA.find(d => d.name === p.voice_name);
      if (v) { seen.add(p.voice_name); result.push(v); }
    }
    return result;
  }, [customPresets]);

  // Display names for custom voice presets in the dropdown (preset name instead of voice name)
  const presetDisplayNames: Record<string, string> = useMemo(() => {
    const map: Record<string, string> = {};
    for (const p of customPresets) {
      // If multiple presets use the same voice, show the first preset's name
      if (!map[p.voice_name]) map[p.voice_name] = p.name;
    }
    return map;
  }, [customPresets]);

  // Track which custom voice is currently selected to derive system instruction
  const [selectedCustomVoiceName, setSelectedCustomVoiceName] = useState<string>(customPresets[0]?.voice_name ?? '');
  const selectedCustomPreset = useMemo(
    () => customPresets.find(p => p.voice_name === selectedCustomVoiceName) ?? customPresets[0],
    [customPresets, selectedCustomVoiceName]
  );

  // Derive accent system instruction for stock voices
  const selectedAccent = useMemo(
    () => TTS_ACCENTS.find(a => a.id === selectedAccentId) ?? TTS_ACCENTS[0],
    [selectedAccentId]
  );
  const accentLanguageCode = selectedAccent.speechLanguageCode || undefined;
  const accentSystemInstruction = selectedAccent.directorsNote
    ? `## Director's Notes\n${selectedAccent.directorsNote}`
    : undefined;

  /** Accent options for the AiTtsPreview dropdown. */
  const accentOptionsForTts = useMemo(
    () => TTS_ACCENTS.map(a => ({ id: a.id, label: a.label })),
    []
  );

  const sharedContent = (
    <>
      {/* Mode Toggle */}
      <div className="flex bg-zinc-100 dark:bg-zinc-800 p-0.5 rounded-lg border border-zinc-200 dark:border-zinc-700 w-fit" role="tablist" aria-label="Script Reader mode">
        <button
          role="tab"
          aria-selected={mode === 'single'}
          onClick={() => setMode('single')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${mode === 'single' ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-white shadow-sm' : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'}`}
            >
              <Mic size={12} />
              Single Speaker
            </button>
            <button
              role="tab"
              aria-selected={mode === 'dialogue'}
              onClick={() => setMode('dialogue')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${mode === 'dialogue' ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-white shadow-sm' : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'}`}
            >
              <Users size={12} />
              Dialogue (2 Speakers)
            </button>
            <button
              role="tab"
              aria-selected={mode === 'compare'}
              onClick={() => setMode('compare')}
              className={`flex items-center gap-1.5 px-3 py-1.5 rounded-md text-xs font-medium transition-all ${mode === 'compare' ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-white shadow-sm' : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'}`}
            >
              <FileText size={12} />
              Compare A/B
            </button>
          </div>

          {/* Dialogue Speaker Config — shown only in dialogue mode */}
          {mode === 'dialogue' && (
            <div className="space-y-3">
              <div className="grid grid-cols-1 sm:grid-cols-2 gap-3">
                <div className="space-y-1.5">
                  <label className="block text-xs font-medium text-zinc-500 dark:text-zinc-400">Speaker 1</label>
                  <input
                    type="text"
                    value={speaker1Name}
                    onChange={(e) => setSpeaker1Name(e.target.value)}
                    className="w-full bg-zinc-50 dark:bg-zinc-800/50 border border-zinc-200 dark:border-zinc-700 rounded-lg px-3 py-1.5 text-sm text-zinc-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
                    placeholder="Speaker name"
                  />
                  <div className="relative">
                    <select
                      value={speaker1Voice}
                      onChange={(e) => setSpeaker1Voice(e.target.value)}
                      className="appearance-none w-full bg-zinc-50 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-700 text-zinc-700 dark:text-zinc-200 py-1.5 pl-3 pr-8 rounded-lg text-sm font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/50 cursor-pointer"
                    >
                      {voices.map(v => <option key={v.name} value={v.name}>{v.name} ({v.analysis.gender})</option>)}
                    </select>
                    <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2 text-zinc-400"><ChevronDown size={12} /></div>
                  </div>
                </div>
                <div className="space-y-1.5">
                  <label className="block text-xs font-medium text-zinc-500 dark:text-zinc-400">Speaker 2</label>
                  <input
                    type="text"
                    value={speaker2Name}
                    onChange={(e) => setSpeaker2Name(e.target.value)}
                    className="w-full bg-zinc-50 dark:bg-zinc-800/50 border border-zinc-200 dark:border-zinc-700 rounded-lg px-3 py-1.5 text-sm text-zinc-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
                    placeholder="Speaker name"
                  />
                  <div className="relative">
                    <select
                      value={speaker2Voice}
                      onChange={(e) => setSpeaker2Voice(e.target.value)}
                      className="appearance-none w-full bg-zinc-50 dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-700 text-zinc-700 dark:text-zinc-200 py-1.5 pl-3 pr-8 rounded-lg text-sm font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/50 cursor-pointer"
                    >
                      {voices.map(v => <option key={v.name} value={v.name}>{v.name} ({v.analysis.gender})</option>)}
                    </select>
                    <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2 text-zinc-400"><ChevronDown size={12} /></div>
                  </div>
                </div>
              </div>

              <div className="flex flex-col gap-2 rounded-xl border border-zinc-200 dark:border-zinc-700 bg-zinc-50/70 dark:bg-zinc-800/30 p-3 sm:flex-row sm:items-center">
                <div className="relative min-w-0 flex-1">
                  <select
                    value={selectedCastId}
                    onChange={(e) => handleLoadDialogueCast(e.target.value)}
                    className="appearance-none w-full bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-700 text-zinc-700 dark:text-zinc-200 py-1.5 pl-3 pr-8 rounded-lg text-xs font-medium focus:outline-none focus:ring-2 focus:ring-indigo-500/50 cursor-pointer"
                    aria-label="Saved casts"
                  >
                    <option value="">Saved casts</option>
                    {savedDialogueCasts.map(cast => <option key={cast.id} value={cast.id}>{cast.name}</option>)}
                  </select>
                  <div className="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2 text-zinc-400"><ChevronDown size={12} /></div>
                </div>
                <input
                  type="text"
                  value={newCastName}
                  onChange={(e) => setNewCastName(e.target.value)}
                  className="min-w-0 flex-1 bg-white dark:bg-zinc-900 border border-zinc-200 dark:border-zinc-700 rounded-lg px-3 py-1.5 text-xs text-zinc-900 dark:text-white focus:outline-none focus:ring-2 focus:ring-indigo-500/50"
                  placeholder="Cast name"
                  aria-label="Dialogue cast name"
                />
                <button
                  type="button"
                  onClick={handleSaveDialogueCast}
                  disabled={!newCastName.trim()}
                  className="inline-flex h-8 items-center justify-center gap-1.5 rounded-lg bg-zinc-900 dark:bg-indigo-600 px-3 text-xs font-semibold text-white hover:bg-zinc-800 dark:hover:bg-indigo-500 transition-colors disabled:opacity-40 disabled:cursor-not-allowed"
                >
                  <Save size={12} />
                  Save cast
                </button>
              </div>
            </div>
          )}

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label htmlFor="script-input" className="block text-sm font-medium text-zinc-700 dark:text-zinc-300">
                {mode === 'dialogue' ? `Enter dialogue (prefix lines with "${speaker1Name}:" or "${speaker2Name}:")` : 'Enter your script'}
              </label>
              <div className="flex items-center gap-1.5">
                <button
                  onClick={handleFormatScript}
                  disabled={!script.trim() || isFormatting}
                  className="flex items-center gap-1.5 px-2.5 py-1 bg-indigo-50 dark:bg-indigo-900/30 hover:bg-indigo-100 dark:hover:bg-indigo-900/50 rounded-lg text-xs font-medium text-indigo-600 dark:text-indigo-400 transition-colors border border-indigo-200 dark:border-indigo-800 disabled:opacity-40 disabled:cursor-not-allowed"
                  title="AI-format script for optimal TTS output"
                >
                  {isFormatting ? <Loader2 size={12} className="animate-spin" /> : <Wand2 size={12} />}
                  Format
                </button>
                {recentScripts.length > 0 && (
                  <div className="relative">
                    <button
                      onClick={() => { setShowRecents(!showRecents); setShowTemplates(false); }}
                      className="flex items-center gap-1.5 px-2.5 py-1 bg-zinc-100 dark:bg-zinc-800 hover:bg-zinc-200 dark:hover:bg-zinc-700 rounded-lg text-xs font-medium text-zinc-600 dark:text-zinc-400 transition-colors border border-zinc-200 dark:border-zinc-700"
                    >
                      <Clock size={12} />
                      Recent
                    </button>
                    {showRecents && (
                      <div className="absolute right-0 top-full mt-1 w-72 bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded-xl shadow-xl z-50 overflow-hidden animate-slide-down">
                        <div className="max-h-48 overflow-y-auto">
                          {recentScripts.map((s, idx) => (
                            <button
                              key={idx}
                              onClick={() => { applyScriptEdit(s, 0, script.length); setShowRecents(false); }}
                              className="w-full text-left px-3 py-2 hover:bg-zinc-50 dark:hover:bg-zinc-700 transition-colors border-b border-zinc-100 dark:border-zinc-700 last:border-0"
                            >
                              <span className="text-xs text-zinc-700 dark:text-zinc-300 line-clamp-2">{s.slice(0, 120)}{s.length > 120 ? '...' : ''}</span>
                            </button>
                          ))}
                        </div>
                      </div>
                    )}
                  </div>
                )}
                <div className="relative">
                  <button
                    onClick={() => { setShowTemplates(!showTemplates); setShowRecents(false); }}
                    className="flex items-center gap-1.5 px-2.5 py-1 bg-zinc-100 dark:bg-zinc-800 hover:bg-zinc-200 dark:hover:bg-zinc-700 rounded-lg text-xs font-medium text-zinc-600 dark:text-zinc-400 transition-colors border border-zinc-200 dark:border-zinc-700"
                  >
                    <BookOpen size={12} />
                    Templates
                    <ChevronDown size={10} className={`transition-transform ${showTemplates ? 'rotate-180' : ''}`} />
                  </button>
                  {showTemplates && (
                    <div className="absolute right-0 top-full mt-1 w-64 bg-white dark:bg-zinc-800 border border-zinc-200 dark:border-zinc-700 rounded-xl shadow-xl z-50 overflow-hidden animate-slide-down">
                      <div className="max-h-60 overflow-y-auto">
                        {SCRIPT_TEMPLATES.map((tmpl, idx) => (
                          <button
                            key={idx}
                            onClick={() => {
                              applyScriptEdit(tmpl.content, 0, script.length);
                              setShowTemplates(false);
                              if (tmpl.category === 'Dialogue' && mode !== 'dialogue') setMode('dialogue');
                            }}
                            className="w-full text-left px-3 py-2 hover:bg-zinc-50 dark:hover:bg-zinc-700 transition-colors border-b border-zinc-100 dark:border-zinc-700 last:border-0"
                          >
                            <span className="text-sm font-medium text-zinc-900 dark:text-white">{tmpl.label}</span>
                            <span className="block text-[10px] text-zinc-400 dark:text-zinc-500">{tmpl.category}</span>
                          </button>
                        ))}
                      </div>
                    </div>
                  )}
                </div>
              </div>
            </div>
            <AudioTagsToolbar onInsertTag={handleInsertTag} />
            <div
              className={`relative rounded-xl transition-colors border border-zinc-200 dark:border-zinc-700 bg-zinc-50 dark:bg-zinc-800/50 ${isDragOver ? 'ring-2 ring-indigo-400 dark:ring-indigo-500 bg-indigo-50/50 dark:bg-indigo-900/20' : ''}`}
              onDragOver={(e) => { e.preventDefault(); setIsDragOver(true); }}
              onDragLeave={() => setIsDragOver(false)}
              onDrop={(e) => {
                e.preventDefault();
                setIsDragOver(false);
                const file = e.dataTransfer.files?.[0];
                if (file && (file.type === 'text/plain' || file.name.endsWith('.txt') || file.name.endsWith('.md'))) {
                  const reader = new FileReader();
                  reader.onload = (ev) => {
                    const text = ev.target?.result;
                    if (typeof text === 'string') setScript(text);
                  };
                  reader.readAsText(file);
                }
              }}
            >
              {isDragOver && (
                <div className="absolute inset-0 flex items-center justify-center bg-indigo-50/80 dark:bg-indigo-900/40 rounded-xl z-10 pointer-events-none">
                  <span className="text-sm font-medium text-indigo-600 dark:text-indigo-400">Drop file to import</span>
                </div>
              )}
              <ScriptHighlighter text={script} scrollTop={highlighterScrollTop} />
              <textarea
                id="script-input"
                ref={textareaRef}
                value={script}
                onChange={(e) => setScript(e.target.value)}
                onScroll={(e) => setHighlighterScrollTop(e.currentTarget.scrollTop)}
                className="relative w-full min-h-[10rem] max-h-[24rem] p-3 bg-transparent border-0 rounded-xl text-sm leading-normal text-transparent caret-zinc-900 dark:caret-white placeholder-zinc-400 focus:outline-none focus:ring-0 resize-y z-[1] overflow-y-auto"
                placeholder="Type the script you want the voice to read, or drop a .txt / .md file..."
              />
            </div>
            <div className="flex items-center justify-between text-[11px] text-zinc-400 dark:text-zinc-500 px-1">
              <span>
                {scriptWordCount.toLocaleString()} word{scriptWordCount !== 1 ? 's' : ''} · {script.length.toLocaleString()} character{script.length !== 1 ? 's' : ''}
              </span>
              <span>~{scriptEstimatedSeconds} sec estimated duration</span>
            </div>
          </div>

          <div className="space-y-2">
            <div className="flex items-center justify-between">
              <label className="block text-sm font-medium text-zinc-700 dark:text-zinc-300">
                Preview
              </label>

              {/* Stock / Custom Voice Toggle — single speaker only */}
              {mode === 'single' && customPresets.length > 0 && (
                <div className="flex bg-zinc-100 dark:bg-zinc-800 p-0.5 rounded-lg border border-zinc-200 dark:border-zinc-700">
                  <button
                    onClick={() => setVoiceSource('stock')}
                    className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium transition-all ${voiceSource === 'stock' ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-white shadow-sm' : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'}`}
                  >
                    <Mic size={11} />
                    Stock
                  </button>
                  <button
                    onClick={() => setVoiceSource('custom')}
                    className={`flex items-center gap-1.5 px-2.5 py-1 rounded-md text-xs font-medium transition-all ${voiceSource === 'custom' ? 'bg-white dark:bg-zinc-700 text-zinc-900 dark:text-white shadow-sm' : 'text-zinc-500 dark:text-zinc-400 hover:text-zinc-700 dark:hover:text-zinc-200'}`}
                  >
                    <User size={11} />
                    My Voices
                    <span className={`text-[10px] font-bold px-1 py-0.5 rounded-full ${voiceSource === 'custom' ? 'bg-indigo-100 dark:bg-indigo-900/50 text-indigo-600 dark:text-indigo-300' : 'bg-zinc-200 dark:bg-zinc-600 text-zinc-600 dark:text-zinc-300'}`}>
                      {customPresets.length}
                    </span>
                  </button>
                </div>
              )}
            </div>

            {/* Custom preset selector — removed; presets now appear in the AiTtsPreview voice dropdown */}
            
            {/* Single speaker preview */}
            {mode === 'single' && (
              voiceSource === 'stock' ? (
                <>
                  <div className="bg-white dark:bg-zinc-800 p-1 rounded-2xl shadow-sm border border-zinc-100 dark:border-zinc-700">
                    <AiTtsPreview
                      text={script}
                      voices={sortedVoices}
                      systemInstruction={accentSystemInstruction}
                      accentOptions={accentOptionsForTts}
                      selectedAccentId={selectedAccentId}
                      onAccentChange={setSelectedAccentId}
                      forceLanguageCode={accentLanguageCode}
                    />
                  </div>
                </>
              ) : presetVoicesForTts.length > 0 ? (
                <div className="bg-white dark:bg-zinc-800 p-1 rounded-2xl shadow-sm border border-zinc-100 dark:border-zinc-700">
                  <AiTtsPreview
                    text={script}
                    voices={presetVoicesForTts}
                    voiceDisplayNames={presetDisplayNames}
                    systemInstruction={selectedCustomPreset?.system_instruction || undefined}
                    onVoiceChange={(name) => setSelectedCustomVoiceName(name)}
                  />
                </div>
              ) : (
                <div className="bg-zinc-50 dark:bg-zinc-800/50 rounded-xl border border-zinc-200 dark:border-zinc-700 p-6 text-center">
                  <p className="text-sm text-zinc-500 dark:text-zinc-400">No custom voice presets yet. Save a preset from the AI Casting Director first.</p>
                </div>
              )
            )}

            {/* Dialogue preview */}
            {mode === 'dialogue' && (
              <div className="bg-white dark:bg-zinc-800 rounded-2xl shadow-sm border border-zinc-100 dark:border-zinc-700 overflow-hidden">
                <div className="p-4 flex items-center justify-between border-b border-zinc-100 dark:border-zinc-700">
                  <span className="text-xs text-zinc-500 dark:text-zinc-400 font-medium">
                    {speaker1Voice} + {speaker2Voice}
                  </span>
                  <div className="flex items-center gap-2">
                    <button
                      onClick={handleDialogueDownload}
                      disabled={dialogueLoading || !script.trim()}
                      className="flex items-center justify-center w-9 h-9 rounded-full bg-zinc-100 dark:bg-zinc-700 text-zinc-600 dark:text-zinc-300 hover:bg-zinc-200 dark:hover:bg-zinc-600 transition-colors disabled:opacity-50 disabled:cursor-not-allowed"
                      title="Download Dialogue WAV"
                    >
                      {dialogueLoading && !dialoguePlaying ? <Loader2 size={14} className="animate-spin" /> : <Download size={14} />}
                    </button>
                    <button
                      onClick={handleDialoguePlay}
                      disabled={dialogueLoading || !script.trim()}
                      className={`flex items-center gap-2 px-4 py-2 rounded-full text-sm font-medium transition-all ${
                        dialoguePlaying
                          ? 'bg-zinc-100 dark:bg-zinc-700 text-zinc-900 dark:text-white border border-zinc-200 dark:border-zinc-600'
                          : 'bg-zinc-900 dark:bg-indigo-600 text-white hover:bg-zinc-800 dark:hover:bg-indigo-500 shadow-md'
                      } disabled:opacity-50 disabled:cursor-not-allowed`}
                    >
                      {dialogueLoading ? <Loader2 size={14} className="animate-spin" /> : dialoguePlaying ? <Square size={14} className="fill-current" /> : <Play size={14} className="fill-current" />}
                      <span>{dialogueLoading ? 'Generating...' : dialoguePlaying ? 'Stop' : 'Listen'}</span>
                    </button>
                  </div>
                </div>
                {dialogueError && (
                  <div className="px-4 py-2 text-xs text-red-500 dark:text-red-400 bg-red-50 dark:bg-red-900/20">
                    {dialogueError}
                  </div>
                )}
              </div>
            )}

            {/* Voice comparison A/B mode */}
            {mode === 'compare' && (
              <VoiceCompare text={script} voices={sortedVoices} />
            )}
          </div>
    </>
  );

  // Inline mode: render as a full-height section without modal overlay
  if (inline) {
    return (
      <div ref={modalRef} tabIndex={-1} data-testid="script-reader-panel" className="flex-1 flex flex-col bg-white dark:bg-zinc-900 overflow-hidden outline-none">
        <div className="flex items-center justify-between p-4 pr-28 border-b border-zinc-100 dark:border-zinc-800">
          <div className="flex items-center gap-2 text-zinc-900 dark:text-white">
            <div className="p-2 bg-indigo-100 dark:bg-indigo-900/30 rounded-lg text-indigo-600 dark:text-indigo-400">
              <FileText size={18} />
            </div>
            <h2 id="script-reader-title" className="text-lg font-bold">Script Reader</h2>
          </div>
        </div>
        <div className="p-6 space-y-6 flex-1 overflow-y-auto">
          {sharedContent}
        </div>
      </div>
    );
  }

  // Modal mode: full-screen overlay
  return (
    <div 
      className="fixed inset-0 z-[70] flex items-center justify-center p-4 bg-zinc-900/60 backdrop-blur-sm animate-fade-in"
      role="dialog"
      aria-modal="true"
      aria-labelledby="script-reader-title"
    >
      <div className="absolute inset-0" onClick={onClose}></div>
      <div 
        ref={modalRef}
        tabIndex={-1}
        className="relative w-full max-w-2xl bg-white dark:bg-zinc-900 rounded-2xl shadow-2xl border border-zinc-200 dark:border-zinc-800 overflow-hidden flex flex-col outline-none animate-slide-up"
      >
        <div className="flex items-center justify-between p-4 border-b border-zinc-100 dark:border-zinc-800">
          <div className="flex items-center gap-2 text-zinc-900 dark:text-white">
            <div className="p-2 bg-indigo-100 dark:bg-indigo-900/30 rounded-lg text-indigo-600 dark:text-indigo-400">
              <FileText size={18} />
            </div>
            <h2 id="script-reader-title" className="text-lg font-bold">Test Script</h2>
          </div>
          <button 
            onClick={onClose}
            className="p-2 text-zinc-400 hover:text-zinc-900 dark:hover:text-white rounded-full hover:bg-zinc-100 dark:hover:bg-zinc-800 transition-colors"
          >
            <X size={18} />
          </button>
        </div>
        <div className="p-6 space-y-6 max-h-[70vh] overflow-y-auto">
          {sharedContent}
        </div>
      </div>
    </div>
  );
};

export default ScriptReaderModal;
