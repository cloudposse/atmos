import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { createPoller } from './polling.mjs';
import { buildArtifactUrl, CAST_FORMATS } from './url.mjs';

export type CastFormat = (typeof CAST_FORMATS)[number];
export type ArtifactStatus = 'idle' | 'checking' | 'rendering' | 'ready' | 'error';
export type RenderPhase = 'queued' | 'processing' | null;

export interface CastArtifactProgress {
  percent: number;
  stage: string;
}

export interface UseCastArtifactParams {
  owner?: string;
  repo?: string;
  ref: string;
  path: string;
  format: CastFormat;
  ttlSeconds?: number;
  soundtrack?: string;
}

export interface UseCastArtifactResult {
  status: ArtifactStatus;
  phase: RenderPhase;
  progress: CastArtifactProgress | null;
  elapsedMs: number;
  slow: boolean;
  errorMessage: string | null;
  url: string;
  // Checks the render status and, once ready, navigates the browser to the
  // artifact (forcing a download). While rendering, polls the JSON status
  // endpoint (see polling.mjs) at a fixed cadence that slows down past a
  // threshold, up to a hard wall-clock ceiling before surfacing a
  // "still rendering" message.
  start: () => void;
}

interface PollerState {
  status: ArtifactStatus;
  phase: RenderPhase;
  progress: CastArtifactProgress | null;
  elapsedMs: number;
  slow: boolean;
  errorMessage: string | null;
}

const IDLE_STATE: PollerState = {
  status: 'idle',
  phase: null,
  progress: null,
  elapsedMs: 0,
  slow: false,
  errorMessage: null,
};

export function useCastArtifact({
  owner,
  repo,
  ref,
  path,
  format,
  ttlSeconds,
  soundtrack,
}: UseCastArtifactParams): UseCastArtifactResult {
  const [state, setState] = useState<PollerState>(IDLE_STATE);
  const pollerRef = useRef<ReturnType<typeof createPoller> | null>(null);
  // A generation counter (rather than a single shared boolean) isolates each
  // request: a boolean reset synchronously by the next effect run would
  // un-cancel an older in-flight poller as soon as `url` changes, letting it
  // still update state or navigate to the stale artifact. The poller's own
  // `cancel()` already stops it from scheduling further polls, but this is
  // kept as a second guard against a stale `onUpdate` landing after a newer
  // poller has started.
  const generationRef = useRef(0);

  const url = useMemo(
    () => buildArtifactUrl({ owner, repo, ref, path, format, ttlSeconds, soundtrack }),
    [owner, repo, ref, path, format, ttlSeconds, soundtrack],
  );

  useEffect(() => {
    generationRef.current += 1;
    // A previous poller (for the old `url`) may have left `state.status`
    // stuck at 'checking'/'rendering' — its cleanup below cancels it, but
    // cancelling doesn't reset state. Without this, the new url would start
    // with `busy` still true and no active poller to ever clear it, leaving
    // the download button permanently disabled.
    setState(IDLE_STATE);
    return () => {
      generationRef.current += 1;
      pollerRef.current?.cancel();
    };
  }, [url]);

  const start = useCallback(() => {
    pollerRef.current?.cancel();
    // Bump the generation here too, not just on `url` change — otherwise a
    // repeated start() for the same url (e.g. a double click before the
    // control disables) would share a generation with a still-live prior
    // poller and let it apply its result after this one begins.
    generationRef.current += 1;
    const generation = generationRef.current;
    setState({ ...IDLE_STATE, status: 'checking' });

    const poller = createPoller({
      url,
      onUpdate: (update: Partial<PollerState> & { status: ArtifactStatus }) => {
        if (generationRef.current !== generation) return;
        // Terminal updates ('ready'/'error') omit `phase`/`progress`/`slow` —
        // merging them onto `prev` would leave the last rendering progress
        // showing beside a ready/error state. Drop back to IDLE_STATE first
        // for terminal updates so those transient fields clear.
        setState((prev) =>
          update.status === 'rendering' ? { ...prev, ...update } : { ...IDLE_STATE, ...update },
        );
        if (update.status === 'ready') {
          const downloadUrl = new URL(url);
          downloadUrl.searchParams.set('download', '1');
          window.location.href = downloadUrl.toString();
        }
      },
    });
    pollerRef.current = poller;
    poller.start();
  }, [url]);

  return { ...state, url, start };
}
