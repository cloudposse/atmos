/** Minimal pull-request shape returned by the GitHub pull-request list API. */
export interface HeadPullRequest {
  number: number;
  state: string;
  head: { sha: string };
}

/** The pagination capability needed to list pull requests, satisfied by GitHubClient. */
export interface PullRequestLister {
  paginate<T>(pathForPage: (page: number) => string, unwrap: (response: unknown) => T[]): Promise<T[]>;
}

/** Validates a top-level paginated pull-request array. */
function unwrapPullRequests(response: unknown): HeadPullRequest[] {
  if (!Array.isArray(response)) {
    throw new Error("GitHub API returned an unexpected paginated response");
  }
  return response as HeadPullRequest[];
}

/**
 * Finds the open pull request for a workflow run whose head lives on a fork.
 *
 * GitHub leaves `workflow_run.pull_requests` empty and `commits/{sha}/pulls`
 * returns nothing for fork-only commits, so the open pull requests are listed
 * by `head=<owner>:<branch>` instead. Only a pull request whose head SHA equals
 * the triggering run's SHA is returned; callers must still re-verify the pull
 * request before trusting it.
 */
export async function findPullRequestByHead(
  lister: PullRequestLister,
  encodedRepo: string,
  headOwner: string | null | undefined,
  headBranch: string | null | undefined,
  headSha: string,
): Promise<number | undefined> {
  if (!headOwner || !headBranch) {
    return undefined;
  }

  const head = encodeURIComponent(`${headOwner}:${headBranch}`);
  const candidates = await lister.paginate<HeadPullRequest>(
    (page) => `${encodedRepo}/pulls?state=open&head=${head}&per_page=100&page=${page}`,
    unwrapPullRequests,
  );
  return candidates.find((pullRequest) => pullRequest.state === "open" && pullRequest.head.sha === headSha)?.number;
}
