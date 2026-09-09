import { useCallback, useEffect, useMemo, useRef, useState } from 'react';

import { buildArtifactUrl, CAST_FORMATS } from './url.mjs';

export type CastFormat = (typeof CAST_FORMATS)[number];
export type ArtifactStatus = 'idle' | 'checking' | 'rendering' | 'ready' | 'error';

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
  errorMessage: string | null;
  url: string;
  // Checks the render status and, once ready, navigates the browser to the
  // artifact (forcing a download). While rendering, polls on Retry-After up
  // to a capped total wait before surfacing an error.
  start: () => void;
}

const MAX_WAIT_MS = 60_000;
const DEFAULT_RETRY_SECONDS = 2;

export function useCastArtifact({
  owner,
  repo,
  ref,
  path,
  format,
  ttlSeconds,
  soundtrack,
}: UseCastArtifactParams): UseCastArtifactResult {
  const [status, setStatus] = useState<ArtifactStatus>('idle');
  const [errorMessage, setErrorMessage] = useState<string | null>(null);
  const timeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // A generation counter (rather than a single shared boolean) isolates each
  // request: a boolean reset synchronously by the next effect run would
  // un-cancel an older in-flight request as soon as `url` changes, letting it
  // still update state or navigate to the stale artifact.
  const generationRef = useRef(0);

  const url = useMemo(
    () => buildArtifactUrl({ owner, repo, ref, path, format, ttlSeconds, soundtrack }),
    [owner, repo, ref, path, format, ttlSeconds, soundtrack],
  );

  useEffect(() => {
    generationRef.current += 1;
    return () => {
      generationRef.current += 1;
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, [url]);

  const check = useCallback(
    async (elapsedMs: number, generation: number) => {
      if (generationRef.current !== generation) return;

      // Bound each fetch to the remaining wall-clock budget so a request that
      // never settles can't hold the format control disabled past MAX_WAIT_MS.
      const controller = new AbortController();
      const deadline = setTimeout(() => controller.abort(), Math.max(MAX_WAIT_MS - elapsedMs, 0));

      let response: Response;
      try {
        response = await fetch(url, { signal: controller.signal });
      } catch {
        clearTimeout(deadline);
        if (generationRef.current !== generation) return;
        setStatus('error');
        setErrorMessage(
          controller.signal.aborted
            ? 'Still rendering after 60s. Please try again in a moment.'
            : 'Network error while checking render status.',
        );
        return;
      }
      clearTimeout(deadline);
      if (generationRef.current !== generation) return;

      if (response.ok) {
        setStatus('ready');
        const downloadUrl = new URL(url);
        downloadUrl.searchParams.set('download', '1');
        window.location.href = downloadUrl.toString();
        return;
      }

      if (response.status === 202) {
        const retryAfterHeader = response.headers.get('Retry-After');
        const retrySeconds = retryAfterHeader ? Number(retryAfterHeader) : NaN;
        const retryMs =
          (Number.isFinite(retrySeconds) && retrySeconds > 0 ? retrySeconds : DEFAULT_RETRY_SECONDS) * 1000;
        const nextElapsedMs = elapsedMs + retryMs;

        if (nextElapsedMs > MAX_WAIT_MS) {
          setStatus('error');
          setErrorMessage('Still rendering after 60s. Please try again in a moment.');
          return;
        }

        setStatus('rendering');
        timeoutRef.current = setTimeout(() => void check(nextElapsedMs, generation), retryMs);
        return;
      }

      let message = `Render failed (HTTP ${response.status})`;
      try {
        const body = await response.json();
        message = body?.error || body?.message || message;
      } catch {
        // Ignore JSON parse errors — fall back to the generic HTTP message.
      }
      if (generationRef.current !== generation) return;
      setStatus('error');
      setErrorMessage(message);
    },
    [url],
  );

  const start = useCallback(() => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    // Bump the generation here too, not just on `url` change — otherwise a
    // repeated start() for the same url (e.g. a double click before the
    // control disables) would share a generation with a still-live prior
    // request and let it apply its result after this one begins.
    generationRef.current += 1;
    setErrorMessage(null);
    setStatus('checking');
    void check(0, generationRef.current);
  }, [check]);

  return { status, errorMessage, url, start };
}
