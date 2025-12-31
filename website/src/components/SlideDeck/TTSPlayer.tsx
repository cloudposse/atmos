import React from 'react';
import { motion, AnimatePresence } from 'framer-motion';
import {
  RiPlayLine,
  RiPauseLine,
  RiStopLine,
  RiLoader4Line,
  RiVolumeMuteLine,
  RiVolumeUpLine,
} from 'react-icons/ri';
import type { TTSVoice, UseTTSReturn } from './useTTS';
import { Tooltip } from './Tooltip';
import './TTSPlayer.css';

interface TTSPlayerProps {
  tts: UseTTSReturn;
  currentSlide: number;
}

const VOICES: { value: TTSVoice; label: string }[] = [
  { value: 'alloy', label: 'Alloy' },
  { value: 'echo', label: 'Echo' },
  { value: 'fable', label: 'Fable' },
  { value: 'nova', label: 'Nova' },
  { value: 'onyx', label: 'Onyx' },
  { value: 'shimmer', label: 'Shimmer' },
];

const SPEEDS = [0.5, 0.75, 1, 1.25, 1.5, 2];

/**
 * TTSPlayer - A full-featured audio player bar for TTS playback.
 *
 * Features:
 * - Play/Pause/Stop controls
 * - Mute toggle
 * - Progress bar with seek
 * - Speed selector
 * - Voice selector
 */
export function TTSPlayer({ tts, currentSlide }: TTSPlayerProps) {
  const {
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
  } = tts;

  const handlePlayPause = () => {
    if (isPlaying) {
      pause();
    } else if (isPaused) {
      resume();
    } else {
      play(currentSlide);
    }
  };

  const handleProgressClick = (e: React.MouseEvent<HTMLDivElement>) => {
    if (duration <= 0) return;
    const rect = e.currentTarget.getBoundingClientRect();
    const pct = (e.clientX - rect.left) / rect.width;
    seek(pct * duration);
  };

  const handleProgressKeyDown = (e: React.KeyboardEvent<HTMLDivElement>) => {
    if (duration <= 0) return;
    const step = 5; // Seek 5 seconds per key press.
    if (e.key === 'ArrowRight') {
      e.preventDefault();
      seek(Math.min(currentTime + step, duration));
    } else if (e.key === 'ArrowLeft') {
      e.preventDefault();
      seek(Math.max(currentTime - step, 0));
    } else if (e.key === 'Home') {
      e.preventDefault();
      seek(0);
    } else if (e.key === 'End') {
      e.preventDefault();
      seek(duration);
    }
  };

  const formatTime = (s: number) => {
    if (!isFinite(s) || s < 0) return '0:00';
    const mins = Math.floor(s / 60);
    const secs = Math.floor(s % 60);
    return `${mins}:${secs.toString().padStart(2, '0')}`;
  };

  return (
    <AnimatePresence>
      <motion.div
        className="tts-player"
        initial={{ opacity: 0, y: 20 }}
        animate={{ opacity: 1, y: 0 }}
        exit={{ opacity: 0, y: 20 }}
        transition={{ duration: 0.2 }}
      >
        {/* Play/Pause Button */}
        <Tooltip content={isPlaying ? 'Pause' : isPaused ? 'Resume' : 'Play'} position="top">
          <button
            className="tts-player__btn tts-player__btn--play"
            onClick={handlePlayPause}
            disabled={isLoading}
            aria-label={isPlaying ? 'Pause' : 'Play'}
          >
            {isLoading ? (
              <RiLoader4Line className="tts-player__spin" />
            ) : isPlaying ? (
              <RiPauseLine />
            ) : (
              <RiPlayLine />
            )}
          </button>
        </Tooltip>

        {/* Stop Button */}
        {(isPlaying || isPaused) && (
          <Tooltip content="Stop" position="top">
            <button
              className="tts-player__btn"
              onClick={stop}
              aria-label="Stop"
            >
              <RiStopLine />
            </button>
          </Tooltip>
        )}

        {/* Mute Button */}
        <Tooltip content={isMuted ? 'Unmute (M)' : 'Mute (M)'} position="top">
          <button
            className={`tts-player__btn ${isMuted ? 'tts-player__btn--muted' : ''}`}
            onClick={toggleMute}
            aria-label={isMuted ? 'Unmute' : 'Mute'}
          >
            {isMuted ? <RiVolumeMuteLine /> : <RiVolumeUpLine />}
          </button>
        </Tooltip>

        {/* Progress Bar */}
        <div
          className="tts-player__progress"
          onClick={handleProgressClick}
          onKeyDown={handleProgressKeyDown}
          role="slider"
          aria-valuemin={0}
          aria-valuemax={100}
          aria-valuenow={progress}
          aria-label="Playback progress"
          tabIndex={0}
        >
          <div
            className="tts-player__progress-fill"
            style={{ width: `${progress}%` }}
          />
        </div>

        {/* Time Display */}
        <span className="tts-player__time">
          {formatTime(currentTime)} / {formatTime(duration)}
        </span>

        {/* Speed Selector */}
        <Tooltip content="Playback Speed" position="top">
          <select
            className="tts-player__select"
            value={playbackRate}
            onChange={(e) => setPlaybackRate(Number(e.target.value))}
            aria-label="Playback speed"
          >
            {SPEEDS.map((s) => (
              <option key={s} value={s}>{s}x</option>
            ))}
          </select>
        </Tooltip>

        {/* Voice Selector */}
        <Tooltip content="Voice" position="top">
          <select
            className="tts-player__select"
            value={voice}
            onChange={(e) => setVoice(e.target.value as TTSVoice)}
            aria-label="Voice"
          >
            {VOICES.map((v) => (
              <option key={v.value} value={v.value}>{v.label}</option>
            ))}
          </select>
        </Tooltip>

        {/* Error Display */}
        {error && <span className="tts-player__error">{error}</span>}
      </motion.div>
    </AnimatePresence>
  );
}

export default TTSPlayer;
