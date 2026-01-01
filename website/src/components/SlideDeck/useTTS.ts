import { useState, useCallback, useRef, useEffect } from 'react';

export type TTSVoice = 'alloy' | 'echo' | 'fable' | 'onyx' | 'nova' | 'shimmer';

interface UseTTSOptions {
  deckName: string;
  onEnded?: () => void;  // Callback when audio finishes.
}

export interface UseTTSReturn {
  // State.
  isPlaying: boolean;
  isLoading: boolean;
  isPaused: boolean;
  isMuted: boolean;
  error: string | null;
  progress: number;        // 0-100.
  duration: number;        // seconds.
  currentTime: number;     // seconds.
  voice: TTSVoice;
  playbackRate: number;

  // Actions.
  play: (slideNumber: number) => Promise<void>;
  pause: () => void;
  resume: () => void;
  stop: () => void;
  seek: (time: number) => void;
  toggleMute: () => void;
  setVoice: (voice: TTSVoice) => void;
  setPlaybackRate: (rate: number) => void;
}

const TTS_PREFS_KEY = 'slide-deck-tts-preferences';

interface TTSPrefs {
  voice: TTSVoice;
  rate: number;
  muted: boolean;
}

const defaultPrefs: TTSPrefs = { voice: 'nova', rate: 1, muted: false };

/**
 * Custom hook for Text-to-Speech playback of slide notes.
 *
 * Uses the Cloud Posse TTS API to convert slide notes to speech.
 * Supports voice selection, speed control, muting, and progress tracking.
 *
 * IMPORTANT: This hook reuses a single Audio element to maintain user-activation
 * state on iOS. Creating new Audio elements breaks autoplay on mobile Safari.
 */
export function useTTS({ deckName, onEnded }: UseTTSOptions): UseTTSReturn {
  // Load saved preferences.
  const loadPrefs = (): TTSPrefs => {
    if (typeof window === 'undefined') return defaultPrefs;
    try {
      const stored = localStorage.getItem(TTS_PREFS_KEY);
      return stored ? { ...defaultPrefs, ...JSON.parse(stored) } : defaultPrefs;
    } catch {
      return defaultPrefs;
    }
  };

  const [voice, setVoiceState] = useState<TTSVoice>(defaultPrefs.voice);
  const [playbackRate, setPlaybackRateState] = useState(defaultPrefs.rate);
  const [isMuted, setIsMuted] = useState(defaultPrefs.muted);
  const [isPlaying, setIsPlaying] = useState(false);
  const [isPaused, setIsPaused] = useState(false);
  const [isLoading, setIsLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [progress, setProgress] = useState(0);
  const [duration, setDuration] = useState(0);
  const [currentTime, setCurrentTime] = useState(0);

  // Persistent audio element - reused across plays to maintain iOS user-activation.
  const audioRef = useRef<HTMLAudioElement | null>(null);
  const onEndedRef = useRef(onEnded);
  onEndedRef.current = onEnded;

  // Get or create the persistent audio element.
  const getAudioElement = useCallback(() => {
    if (!audioRef.current && typeof window !== 'undefined') {
      const audio = new Audio();
      audio.onloadedmetadata = () => setDuration(audio.duration);
      audio.ontimeupdate = () => {
        setCurrentTime(audio.currentTime);
        if (audio.duration > 0) {
          setProgress((audio.currentTime / audio.duration) * 100);
        }
      };
      audio.onended = () => {
        setIsPlaying(false);
        setIsPaused(false);
        setProgress(100);
        onEndedRef.current?.();
      };
      audio.onerror = () => {
        setError('Playback failed');
        setIsPlaying(false);
        setIsLoading(false);
      };
      audioRef.current = audio;
    }
    return audioRef.current;
  }, []);

  // Load prefs on mount.
  useEffect(() => {
    const prefs = loadPrefs();
    setVoiceState(prefs.voice);
    setPlaybackRateState(prefs.rate);
    setIsMuted(prefs.muted);
  }, []);

  // Save preferences.
  const savePrefs = useCallback((v: TTSVoice, r: number, m: boolean) => {
    if (typeof window === 'undefined') return;
    localStorage.setItem(TTS_PREFS_KEY, JSON.stringify({ voice: v, rate: r, muted: m }));
  }, []);

  const setVoice = useCallback((v: TTSVoice) => {
    setVoiceState(v);
    savePrefs(v, playbackRate, isMuted);
  }, [playbackRate, isMuted, savePrefs]);

  const setPlaybackRate = useCallback((r: number) => {
    setPlaybackRateState(r);
    const audio = audioRef.current;
    if (audio) audio.playbackRate = r;
    savePrefs(voice, r, isMuted);
  }, [voice, isMuted, savePrefs]);

  const toggleMute = useCallback(() => {
    const newMuted = !isMuted;
    setIsMuted(newMuted);
    const audio = audioRef.current;
    if (audio) audio.muted = newMuted;
    savePrefs(voice, playbackRate, newMuted);
  }, [isMuted, voice, playbackRate, savePrefs]);

  const getTextUrl = useCallback((slideNumber: number) => {
    const origin = typeof window !== 'undefined' ? window.location.origin : '';
    return `${origin}/slides/${deckName}/slide${slideNumber}.txt`;
  }, [deckName]);

  const play = useCallback(async (slideNumber: number) => {
    const audio = getAudioElement();
    if (!audio) return;

    // Stop current playback.
    audio.pause();
    audio.currentTime = 0;

    setIsLoading(true);
    setError(null);
    setProgress(0);
    setCurrentTime(0);
    setDuration(0);

    try {
      const textUrl = getTextUrl(slideNumber);
      const apiUrl = `https://cloudposse.com/api/tts?url=${encodeURIComponent(textUrl)}&voice=${voice}`;

      const response = await fetch(apiUrl);
      if (!response.ok) {
        let errorMsg = 'TTS failed';
        try {
          const err = await response.json();
          errorMsg = err.error || errorMsg;
        } catch {
          // Ignore JSON parse errors.
        }
        throw new Error(errorMsg);
      }

      const data = await response.json();

      // Update the existing audio element's source instead of creating a new one.
      // This preserves the user-activation state on iOS.
      audio.src = `data:${data.mimeType};base64,${data.audio}`;
      audio.playbackRate = playbackRate;
      audio.muted = isMuted;

      // Wait for audio to be ready.
      await new Promise<void>((resolve, reject) => {
        const onCanPlay = () => {
          audio.removeEventListener('canplaythrough', onCanPlay);
          audio.removeEventListener('error', onError);
          resolve();
        };
        const onError = () => {
          audio.removeEventListener('canplaythrough', onCanPlay);
          audio.removeEventListener('error', onError);
          reject(new Error('Failed to load audio'));
        };
        audio.addEventListener('canplaythrough', onCanPlay);
        audio.addEventListener('error', onError);
        audio.load();
      });

      setIsLoading(false);
      setIsPlaying(true);
      setIsPaused(false);
      await audio.play();
    } catch (err) {
      setIsLoading(false);
      setIsPlaying(false);
      setError(err instanceof Error ? err.message : 'Unknown error');
    }
  }, [getAudioElement, getTextUrl, voice, playbackRate, isMuted]);

  const pause = useCallback(() => {
    const audio = audioRef.current;
    if (audio) {
      audio.pause();
      setIsPaused(true);
      setIsPlaying(false);
    }
  }, []);

  const resume = useCallback(() => {
    const audio = audioRef.current;
    if (audio && isPaused) {
      audio.play();
      setIsPaused(false);
      setIsPlaying(true);
    }
  }, [isPaused]);

  const stop = useCallback(() => {
    const audio = audioRef.current;
    if (audio) {
      audio.pause();
      audio.currentTime = 0;
      audio.src = ''; // Clear the source.
    }
    setIsPlaying(false);
    setIsPaused(false);
    setProgress(0);
    setCurrentTime(0);
    setDuration(0);
  }, []);

  const seek = useCallback((time: number) => {
    const audio = audioRef.current;
    if (audio) {
      audio.currentTime = time;
    }
  }, []);

  // Cleanup on unmount.
  useEffect(() => {
    return () => {
      const audio = audioRef.current;
      if (audio) {
        audio.pause();
        audio.src = '';
        audioRef.current = null;
      }
    };
  }, []);

  return {
    isPlaying,
    isLoading,
    isPaused,
    isMuted,
    error,
    progress,
    duration,
    currentTime,
    voice,
    playbackRate,
    play,
    pause,
    resume,
    stop,
    seek,
    toggleMute,
    setVoice,
    setPlaybackRate,
  };
}
