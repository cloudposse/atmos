const nonRetryableStatuses = new Set([400, 401, 403, 404, 422]);
const maxRateLimitWait = 65 * 60 * 1000;

function isRateLimitError(err) {
  const headers = err.response?.headers || {};
  const message = err.response?.data?.message;
  const rateLimitMessage = typeof message === 'string' && /(?:secondary rate limit|API rate limit exceeded)/i.test(message);
  return err.status === 429 || (err.status === 403 &&
    (headers['x-ratelimit-remaining'] === '0' || headers['retry-after'] !== undefined || rateLimitMessage));
}

// Only the documented organization IP-policy denial is eligible for the
// separately reviewed repository allowlist. Unknown 403s must fail closed.
function isAccessBlockedError(err, owner) {
  if (err.status !== 403 || isRateLimitError(err)) return false;
  return err.response?.data?.message ===
    `Although you appear to have the correct authorization credentials, the \`${owner}\` organization has an IP allow list enabled, and your IP address is not permitted to access this resource.`;
}

function rateLimitDelay(err, now, attempt) {
  const headers = err.response?.headers || {};
  const seconds = value => typeof value === 'string' && /^\d+$/.test(value) ? Number(value) : NaN;
  const retryAfter = seconds(headers['retry-after']);
  const reset = seconds(headers['x-ratelimit-reset']);
  const delays = [];
  if (Number.isFinite(retryAfter)) delays.push(retryAfter * 1000);
  if (headers['x-ratelimit-remaining'] === '0' && Number.isFinite(reset)) {
    delays.push(reset * 1000 - now + 1000);
  }
  if (delays.length) return Math.max(1000, ...delays);
  // A primary limit without a usable reset cannot be retried safely.
  if (headers['x-ratelimit-remaining'] === '0') return null;
  return 60000 * (2 ** (attempt - 1));
}

function createRetry({ warning, sleep = ms => new Promise(resolve => setTimeout(resolve, ms)), now = Date.now }) {
  let waited = 0;
  let exhaustedRateLimitError;
  return async function withRetries(operation, label, maxAttempts = 3) {
    // Once recovery is exhausted, do not spend more requests on every remaining ref.
    if (exhaustedRateLimitError) throw exhaustedRateLimitError;
    let lastErr;
    for (let attempt = 1; attempt <= maxAttempts; attempt++) {
      try {
        return await operation();
      } catch (err) {
        const rateLimited = isRateLimitError(err);
        if (!rateLimited && nonRetryableStatuses.has(err.status)) throw err;
        lastErr = err;
        const delay = rateLimited ? rateLimitDelay(err, now(), attempt) : 250 * attempt;
        if (rateLimited && (attempt === maxAttempts || delay === null || waited + delay > maxRateLimitWait)) {
          exhaustedRateLimitError = err;
          throw err;
        }
        if (attempt < maxAttempts) {
          warning(`${label} failed on attempt ${attempt}/${maxAttempts}: ${err.message}; retrying in ${delay}ms`);
          if (rateLimited) waited += delay;
          await sleep(delay);
        }
      }
    }
    throw lastErr;
  };
}

module.exports = { createRetry, isRateLimitError, isAccessBlockedError };
