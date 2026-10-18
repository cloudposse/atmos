import assert from "node:assert/strict";
import test from "node:test";

import { findPullRequestByHead, type HeadPullRequest, type PullRequestLister } from "./pull-request";

const REPO = "/repos/cloudposse/atmos";
const HEAD_SHA = "497b3a2d43371e3a98912e38a162f39b6c5d2d96";

/** Creates a fake lister that records requested paths and returns one page of pull requests. */
function listerFor(pullRequests: HeadPullRequest[]): { lister: PullRequestLister; paths: string[] } {
  const paths: string[] = [];
  const lister: PullRequestLister = {
    async paginate<T>(pathForPage: (page: number) => string, unwrap: (response: unknown) => T[]): Promise<T[]> {
      paths.push(pathForPage(1));
      return unwrap(pullRequests);
    },
  };
  return { lister, paths };
}

test("fork fallback returns the open pull request whose head SHA matches", async () => {
  const { lister, paths } = listerFor([
    { number: 3380, state: "open", head: { sha: "0000000000000000000000000000000000000000" } },
    { number: 3381, state: "open", head: { sha: HEAD_SHA } },
  ]);

  const number = await findPullRequestByHead(lister, REPO, "jorrite", "hp/atmos/t-0004-fix", HEAD_SHA);

  assert.equal(number, 3381);
  assert.deepEqual(paths, [`${REPO}/pulls?state=open&head=jorrite%3Ahp%2Fatmos%2Ft-0004-fix&per_page=100&page=1`]);
});

test("fork fallback ignores an open pull request with a stale head SHA", async () => {
  const { lister } = listerFor([
    { number: 3381, state: "open", head: { sha: "1111111111111111111111111111111111111111" } },
  ]);

  assert.equal(await findPullRequestByHead(lister, REPO, "jorrite", "branch", HEAD_SHA), undefined);
});

test("fork fallback ignores pull requests that are not open", async () => {
  const { lister } = listerFor([{ number: 3381, state: "closed", head: { sha: HEAD_SHA } }]);

  assert.equal(await findPullRequestByHead(lister, REPO, "jorrite", "branch", HEAD_SHA), undefined);
});

test("fork fallback returns undefined without listing when head owner or branch is missing", async () => {
  const { lister, paths } = listerFor([{ number: 3381, state: "open", head: { sha: HEAD_SHA } }]);

  assert.equal(await findPullRequestByHead(lister, REPO, undefined, "branch", HEAD_SHA), undefined);
  assert.equal(await findPullRequestByHead(lister, REPO, "jorrite", null, HEAD_SHA), undefined);
  assert.equal(await findPullRequestByHead(lister, REPO, "", "", HEAD_SHA), undefined);
  assert.deepEqual(paths, []);
});

test("fork fallback rejects an unexpected response shape", async () => {
  const lister: PullRequestLister = {
    async paginate<T>(_pathForPage: (page: number) => string, unwrap: (response: unknown) => T[]): Promise<T[]> {
      return unwrap({ message: "not an array" });
    },
  };

  await assert.rejects(
    findPullRequestByHead(lister, REPO, "jorrite", "branch", HEAD_SHA),
    /unexpected paginated response/,
  );
});
