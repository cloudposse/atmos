// Framework-agnostic polling engine for the Atmos Pro cast-rendering service's
// status endpoint. Kept free of React so it can be unit-tested directly with
// `node --test`, a mocked `fetch`, and fake timers (see polling.test.mjs).
//
// The service's contract (confirmed against a live probe, not just docs):
//   - GET the artifact URL with `Accept: application/json` to poll status
//     without transferring the rendered file.
//   - 202 + JSON body while queued/rendering — read `data.artifacts[0].status`
//     ("queued" | "processing" | "ready" | "failed") to distinguish queued vs.
//     actively rendering, and `data.artifacts[0].progress` (optional,
//     `{ percent, stage }`) when the service starts reporting it.
//   - 200 once ready — but with the *rendered artifact itself* as the body,
//     even though we asked for JSON. Never read that body (it can be tens of
//     MB); abort the fetch immediately once the 200 status line is seen.
//   - 400/500 + JSON `{ success: false, error }` on a terminal failure.

// Poll every 3s while rendering — cheap enough on the JSON path that there's
// no reason to wait out the service's 30s Retry-After.
export const POLL_INTERVAL_MS = 3_000;

// Long casts can legitimately take minutes to render. Past this much total
// wait, slow the poll cadence down (no point hammering every 3s for renders
// that are already running long) and let the UI say so.
export const SLOWDOWN_THRESHOLD_MS = 13 * 60 * 1000;
export const SLOW_POLL_INTERVAL_MS = 10_000;

// Hard ceiling on total wait. Reaching it doesn't mean the render failed —
// the service could still be working — so the message must say so rather
// than reading as an error.
export const MAX_WAIT_MS = 30 * 60 * 1000;

export const STILL_RENDERING_MESSAGE =
  'Still rendering after 30 minutes. The render may still finish — please try again in a few minutes.';
export const NETWORK_ERROR_MESSAGE = 'Network error while checking render status.';

export function isSlow(elapsedMs) {
  return elapsedMs >= SLOWDOWN_THRESHOLD_MS;
}

export function nextPollIntervalMs(elapsedMs) {
  return isSlow(elapsedMs) ? SLOW_POLL_INTERVAL_MS : POLL_INTERVAL_MS;
}

export function formatElapsed(ms) {
  const totalSeconds = Math.max(0, Math.round(ms / 1000));
  const minutes = Math.floor(totalSeconds / 60);
  const seconds = totalSeconds % 60;
  return `${minutes}:${String(seconds).padStart(2, '0')}`;
}

// Renders a short status label for the busy states. Progress (when the
// service starts sending it) is shown separately by the caller as a bar, not
// folded into this text.
export function describeStatus({ status, phase, elapsedMs, slow }) {
  if (status === 'checking') return 'Checking…';
  if (status !== 'rendering') return '';
  const label = phase === 'queued' ? 'Queued…' : 'Rendering…';
  const suffix = slow ? ' — taking longer than usual' : '';
  return `${label} ${formatElapsed(elapsedMs)}${suffix}`;
}

// Classifies a 202 (or any non-200 status other than the ones the caller
// already special-cased) response body into what the poller/UI should do
// next. Pure and separately unit-tested from the timer-driven engine below.
export function interpretPollResponse(status, body) {
  if (status === 202) {
    const artifact = body?.data?.artifacts?.[0] ?? null;
    if (artifact?.status === 'failed') {
      return { kind: 'error', message: artifact.error || body?.error || 'Render failed.' };
    }
    return {
      kind: 'rendering',
      phase: artifact?.status === 'queued' ? 'queued' : 'processing',
      progress: artifact?.progress ?? null,
    };
  }
  return {
    kind: 'error',
    message: body?.error || body?.message || `Render failed (HTTP ${status})`,
  };
}

// Creates a poller for a single artifact URL. `onUpdate` is called with a
// plain state object every time something changes:
//   { status: 'rendering', phase, progress, elapsedMs, slow }
//   { status: 'ready', elapsedMs }
//   { status: 'error', errorMessage, elapsedMs }
//
// `initialElapsedMs` (default 0) lets tests start a poller already close to
// the slowdown/ceiling thresholds instead of simulating the real timeline
// tick by tick; real callers never need to pass it.
export function createPoller({ url, onUpdate, fetchImpl = fetch, initialElapsedMs = 0 }) {
  let cancelled = false;
  let timeoutHandle = null;
  let activeController = null;

  async function poll(elapsedMs) {
    if (cancelled) return;

    if (elapsedMs >= MAX_WAIT_MS) {
      onUpdate({ status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs });
      return;
    }

    const controller = new AbortController();
    activeController = controller;
    // Bound this fetch to the remaining wall-clock budget so a request that
    // never settles can't hold the button disabled past MAX_WAIT_MS.
    const deadline = setTimeout(() => controller.abort(), MAX_WAIT_MS - elapsedMs);

    let response;
    try {
      response = await fetchImpl(url, { signal: controller.signal, headers: { Accept: 'application/json' } });
    } catch {
      clearTimeout(deadline);
      if (cancelled) return;
      onUpdate(
        controller.signal.aborted
          ? { status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs: MAX_WAIT_MS }
          : { status: 'error', errorMessage: NETWORK_ERROR_MESSAGE, elapsedMs },
      );
      return;
    }
    clearTimeout(deadline);
    if (cancelled) return;

    // Any 200 means ready, full stop — per the service's contract, a ready
    // artifact is served as the file itself (not JSON) even though we asked
    // for JSON. Abort right away so this poll doesn't transfer the file;
    // the caller does a separate, explicit navigation to actually download.
    if (response.status === 200) {
      controller.abort();
      onUpdate({ status: 'ready', elapsedMs });
      return;
    }

    let body = null;
    try {
      body = await response.json();
    } catch {
      // Ignore JSON parse errors — the classifier below falls back to a
      // generic message/rendering state.
    }
    if (cancelled) return;

    const outcome = interpretPollResponse(response.status, body);

    if (outcome.kind === 'rendering') {
      onUpdate({
        status: 'rendering',
        phase: outcome.phase,
        progress: outcome.progress,
        elapsedMs,
        slow: isSlow(elapsedMs),
      });
      const interval = nextPollIntervalMs(elapsedMs);
      timeoutHandle = setTimeout(() => void poll(elapsedMs + interval), interval);
      return;
    }

    onUpdate({ status: 'error', errorMessage: outcome.message, elapsedMs });
  }

  return {
    start() {
      cancelled = false;
      void poll(initialElapsedMs);
    },
    cancel() {
      cancelled = true;
      if (timeoutHandle) clearTimeout(timeoutHandle);
      if (activeController) activeController.abort();
    },
  };
}
