const API_VERSION = "2022-11-28";
const DEFAULT_REQUEST_TIMEOUT_MS = 30_000;
const DEFAULT_MAX_GET_ATTEMPTS = 3;
const MAX_RETRY_DELAY_MS = 30_000;

export interface GitHubClientOptions {
  baseUrl?: string;
  fetch?: typeof fetch;
  sleep?: (milliseconds: number) => Promise<void>;
  requestTimeoutMs?: number;
  maxGetAttempts?: number;
}

/** Returns whether an unsuccessful GET response is safe and useful to retry. */
function isRetryableResponse(response: Response): boolean {
  if (response.status >= 500 || response.status === 429) {
    return true;
  }

  return response.status === 403 && (
    response.headers.has("retry-after") || response.headers.get("x-ratelimit-remaining") === "0"
  );
}

/** Calculates a bounded retry delay from GitHub headers or exponential backoff. */
function retryDelayMilliseconds(response: Response | undefined, attempt: number): number {
  const fallback = 1_000 * 2 ** (attempt - 1);
  const retryAfter = response?.headers.get("retry-after");
  if (retryAfter !== null && retryAfter !== undefined) {
    const seconds = Number(retryAfter);
    if (Number.isFinite(seconds)) {
      return Math.min(MAX_RETRY_DELAY_MS, Math.max(0, seconds * 1_000));
    }

    const retryAt = Date.parse(retryAfter);
    if (Number.isFinite(retryAt)) {
      return Math.min(MAX_RETRY_DELAY_MS, Math.max(0, retryAt - Date.now()));
    }
  }

  if (response?.headers.get("x-ratelimit-remaining") === "0") {
    const resetAt = Number(response.headers.get("x-ratelimit-reset"));
    if (Number.isFinite(resetAt)) {
      return Math.min(MAX_RETRY_DELAY_MS, Math.max(0, resetAt * 1_000 - Date.now()));
    }
  }

  return Math.min(MAX_RETRY_DELAY_MS, fallback);
}

/** Waits for a retry delay. */
function sleep(milliseconds: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, milliseconds));
}

/** Minimal GitHub REST client with pagination and bounded retries for safe GETs. */
export class GitHubClient {
  readonly #baseUrl: string;
  readonly #fetch: typeof fetch;
  readonly #maxGetAttempts: number;
  readonly #requestTimeoutMs: number;
  readonly #sleep: (milliseconds: number) => Promise<void>;
  readonly #token: string;

  /** Creates a client whose transport options can be replaced by unit tests. */
  constructor(token: string, options: GitHubClientOptions = {}) {
    this.#token = token;
    this.#baseUrl = (options.baseUrl ?? process.env.GITHUB_API_URL ?? "https://api.github.com").replace(/\/$/, "");
    this.#fetch = options.fetch ?? fetch;
    this.#sleep = options.sleep ?? sleep;
    this.#requestTimeoutMs = options.requestTimeoutMs ?? DEFAULT_REQUEST_TIMEOUT_MS;
    this.#maxGetAttempts = options.maxGetAttempts ?? DEFAULT_MAX_GET_ATTEMPTS;
  }

  /** Sends one GitHub REST request, retrying only idempotent GET failures. */
  async request<T>(method: string, path: string, body?: unknown): Promise<T> {
    const maxAttempts = method === "GET" ? this.#maxGetAttempts : 1;

    for (let attempt = 1; attempt <= maxAttempts; attempt += 1) {
      let response: Response;
      try {
        response = await this.#fetch(`${this.#baseUrl}${path}`, {
          method,
          headers: {
            Accept: "application/vnd.github+json",
            Authorization: `Bearer ${this.#token}`,
            "Content-Type": "application/json",
            "User-Agent": "atmos-stopwatch",
            "X-GitHub-Api-Version": API_VERSION,
          },
          body: body === undefined ? undefined : JSON.stringify(body),
          signal: AbortSignal.timeout(this.#requestTimeoutMs),
        });
      } catch (error: unknown) {
        if (method !== "GET" || attempt === maxAttempts) {
          throw error;
        }

        const delay = retryDelayMilliseconds(undefined, attempt);
        console.log(`GitHub API GET ${path} failed; retrying in ${delay}ms (${attempt + 1}/${maxAttempts})`);
        await this.#sleep(delay);
        continue;
      }

      if (response.ok) {
        return await response.json() as T;
      }

      if (method === "GET" && attempt < maxAttempts && isRetryableResponse(response)) {
        const delay = retryDelayMilliseconds(response, attempt);
        console.log(`GitHub API GET ${path} returned ${response.status}; retrying in ${delay}ms (${attempt + 1}/${maxAttempts})`);
        await this.#sleep(delay);
        continue;
      }

      const responseBody = (await response.text()).slice(0, 2_000);
      throw new Error(`GitHub API ${method} ${path} returned ${response.status}: ${responseBody}`);
    }

    throw new Error(`GitHub API ${method} ${path} exhausted its retry attempts`);
  }

  /** Fetches every page from a GitHub REST collection. */
  async paginate<T>(pathForPage: (page: number) => string, unwrap: (response: unknown) => T[]): Promise<T[]> {
    const results: T[] = [];
    for (let page = 1; ; page += 1) {
      const response = await this.request<unknown>("GET", pathForPage(page));
      const pageResults = unwrap(response);
      results.push(...pageResults);
      if (pageResults.length < 100) {
        return results;
      }
    }
  }
}
