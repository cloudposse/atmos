// Framework-agnostic polling engine for the Atmos Pro cast-rendering service's
// status endpoint. Kept free of React so it can be unit-tested directly with
// `node --test`, a mocked `fetch`, and fake timers (see polling.test.mjs).
//
// The service's contract:
//   - GET the artifact URL with `Accept: application/json` to poll status
//     without transferring the rendered file.
//   - 202 + JSON body while queued/rendering — read `data.artifacts[0].status`
//     ("queued" | "processing" | "ready" | "failed") to distinguish queued vs.
//     actively rendering, and `data.artifacts[0].progress` (optional,
//     `{ percent, stage }`) when the service starts reporting it.
//   - 200 + JSON body once ready — `data.artifacts[0].status === "ready"` is
//     the readiness signal; `artifacts[0].url` is a tokened direct link to
//     the artifact (unused here — `useCastArtifact.ts` downloads via the
//     stable `?download=1` URL instead) and `blobUrl` is always `null`.
//     Some responses may still come back as the rendered artifact itself
//     (`Content-Type: video/mp4` etc., not JSON) rather than this JSON body —
//     never read that body (it can be tens of MB); abort the fetch
//     immediately once such a 200 status line is seen and treat it as ready.
//   - 400/404/500 + JSON `{ success: false, error }` on a terminal failure,
//     readable cross-origin — `error` is human-readable and safe to show
//     verbatim.

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

// Classifies a `data.artifacts[0]` entry (shared by both a 202 and a JSON
// 200 response) into what the poller/UI should do next.
function classifyArtifact(body) {
  const artifact = body?.data?.artifacts?.[0] ?? null;
  if (artifact?.status === 'ready') {
    return { kind: 'ready' };
  }
  if (artifact?.status === 'failed') {
    return { kind: 'error', message: artifact.error || body?.error || 'Render failed.' };
  }
  return {
    kind: 'rendering',
    phase: artifact?.status === 'queued' ? 'queued' : 'processing',
    progress: artifact?.progress ?? null,
  };
}

// Classifies a poll response body into what the poller/UI should do next.
// A 202 or a JSON 200 both carry the same `data.artifacts[0]` shape, so
// "ready" is a check of that field rather than an inference from the HTTP
// status code. Any other status (400/404/500/etc.) is a terminal error —
// `body.error` is already human-readable and CORS-exposed by the service.
// Pure and separately unit-tested from the timer-driven engine below.
export function interpretPollResponse(status, body) {
  if (status === 202 || status === 200) {
    return classifyArtifact(body);
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
// tick by tick; real callers never need to pass it. `now` (default
// `Date.now`) is the monotonic clock used to compute elapsed time — tests
// mock it (via `node:test`'s `Date` mock timer) so the ceiling can be
// exercised deterministically without a real 30-minute wait.
//
// Elapsed time is always derived from `now() - startedAt`, never accumulated
// as a nominal counter of poll intervals. A poll interval only tells the
// poller when to check again — it says nothing about how long that check
// itself takes. Deriving elapsed time from actual wall-clock time (rather
// than "polls so far * interval") is what makes MAX_WAIT_MS a real ceiling
// on total wait even when an individual round trip runs long: a slow poll
// can't quietly hand the next poll a fresh budget.
export function createPoller({ url, onUpdate, fetchImpl = fetch, initialElapsedMs = 0, now = Date.now }) {
  let cancelled = false;
  let timeoutHandle = null;
  let activeController = null;
  let startedAt = 0;

  function elapsed() {
    return now() - startedAt;
  }

  async function poll() {
    if (cancelled) return;

    const elapsedMs = elapsed();
    if (elapsedMs >= MAX_WAIT_MS) {
      onUpdate({ status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs: MAX_WAIT_MS });
      return;
    }

    const controller = new AbortController();
    activeController = controller;
    // Bound the *entire* round trip — the fetch AND the subsequent JSON body
    // read — to the remaining wall-clock budget, so neither a stalled
    // request nor a hanging body stream can hold the button disabled past
    // MAX_WAIT_MS. Aborting the controller cancels an in-flight
    // `response.json()` read too (the body reader is tied to the same
    // signal), so this deadline stays live until the body has been read.
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
          : { status: 'error', errorMessage: NETWORK_ERROR_MESSAGE, elapsedMs: elapsed() },
      );
      return;
    }

    // A 200 with a non-JSON content type is the legacy/cached shape: a ready
    // artifact served as the file itself, even though we asked for JSON.
    // Abort right away so this poll doesn't transfer the file — the caller
    // does a separate, explicit navigation to actually download.
    if (response.status === 200 && !(response.headers.get('content-type') || '').toLowerCase().includes('json')) {
      clearTimeout(deadline);
      controller.abort();
      onUpdate({ status: 'ready', elapsedMs: elapsed() });
      return;
    }

    let body = null;
    try {
      body = await response.json();
    } catch {
      // Ignore JSON parse errors — the classifier below falls back to a
      // generic message/rendering state.
    }
    clearTimeout(deadline);
    if (cancelled) return;

    if (controller.signal.aborted) {
      // The remaining budget ran out while the body was still being read —
      // report the ceiling rather than treating the aborted read as a
      // legitimate (if empty) response.
      onUpdate({ status: 'error', errorMessage: STILL_RENDERING_MESSAGE, elapsedMs: MAX_WAIT_MS });
      return;
    }

    const outcome = interpretPollResponse(response.status, body);
    const currentElapsed = elapsed();

    if (outcome.kind === 'ready') {
      onUpdate({ status: 'ready', elapsedMs: currentElapsed });
      return;
    }

    if (outcome.kind === 'rendering') {
      onUpdate({
        status: 'rendering',
        phase: outcome.phase,
        progress: outcome.progress,
        elapsedMs: currentElapsed,
        slow: isSlow(currentElapsed),
      });
      const interval = nextPollIntervalMs(currentElapsed);
      timeoutHandle = setTimeout(() => void poll(), interval);
      return;
    }

    onUpdate({ status: 'error', errorMessage: outcome.message, elapsedMs: currentElapsed });
  }

  return {
    start() {
      cancelled = false;
      startedAt = now() - initialElapsedMs;
      void poll();
    },
    cancel() {
      cancelled = true;
      if (timeoutHandle) clearTimeout(timeoutHandle);
      if (activeController) activeController.abort();
    },
  };
}
