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
  const cancelledRef = useRef(false);

  const url = useMemo(
    () => buildArtifactUrl({ owner, repo, ref, path, format, ttlSeconds, soundtrack }),
    [owner, repo, ref, path, format, ttlSeconds, soundtrack],
  );

  useEffect(() => {
    cancelledRef.current = false;
    return () => {
      cancelledRef.current = true;
      if (timeoutRef.current) clearTimeout(timeoutRef.current);
    };
  }, [url]);

  const check = useCallback(
    async (elapsedMs: number) => {
      if (cancelledRef.current) return;
      try {
        const response = await fetch(url);
        if (cancelledRef.current) return;

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
          timeoutRef.current = setTimeout(() => void check(nextElapsedMs), retryMs);
          return;
        }

        let message = `Render failed (HTTP ${response.status})`;
        try {
          const body = await response.json();
          message = body?.error || body?.message || message;
        } catch {
          // Ignore JSON parse errors — fall back to the generic HTTP message.
        }
        setStatus('error');
        setErrorMessage(message);
      } catch {
        if (cancelledRef.current) return;
        setStatus('error');
        setErrorMessage('Network error while checking render status.');
      }
    },
    [url],
  );

  const start = useCallback(() => {
    if (timeoutRef.current) clearTimeout(timeoutRef.current);
    setErrorMessage(null);
    setStatus('checking');
    void check(0);
  }, [check]);

  return { status, errorMessage, url, start };
}
