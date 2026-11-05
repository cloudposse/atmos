const assert = require('node:assert/strict');
const { test } = require('node:test');
const notify = require('./comment.cjs');

const repo = { owner: 'cloudposse', repo: 'atmos' };
const release = {
  id: 123, tag_name: 'v1.233.0', name: 'Atmos v1.233.0',
  html_url: 'https://github.com/cloudposse/atmos/releases/tag/v1.233.0',
};
const previous = { id: 122, tag_name: 'v1.232.0' };

function fixture({ releases = [release, previous], associations = [[41, 42], [42, 43]] } = {}) {
  const calls = { comments: [], comparisons: [], queries: [] };
  const github = {
    rest: {
      repos: {
        getRelease: async args => {
          assert.deepEqual(args, { ...repo, release_id: release.id });
          return { data: release };
        },
        listReleases: async args => {
          assert.deepEqual(args, repo);
          return { data: releases };
        },
        compareCommits: async args => {
          calls.comparisons.push(args);
          return { data: { commits: associations.map((_, i) => ({ sha: String(i).padStart(40, 'a') })) } };
        },
      },
      issues: { createComment: async args => { calls.comments.push(args); } },
    },
    graphql: async query => {
      const index = calls.queries.length;
      calls.queries.push(query);
      return { resource: { associatedPullRequests: { edges: associations[index].map(number => ({ node: { number } })) } } };
    },
  };
  return {
    calls, github,
    context: { repo, payload: { release: { id: release.id }, repository: { html_url: 'https://github.com/cloudposse/atmos' } } },
  };
}

test('notifies each included PR once with the exact published release link', async () => {
  const api = fixture({ releases: [{ tag_name: 'v1.234.0' }, release, previous] });
  await notify(api);
  assert.deepEqual(api.calls.comparisons, [{ ...repo, base: previous.tag_name, head: release.tag_name }]);
  assert.equal(api.calls.queries.length, 2);
  assert.match(api.calls.queries[0], /https:\/\/github.com\/cloudposse\/atmos\/commit\/a{39}0/);
  assert.deepEqual(api.calls.comments.map(c => c.issue_number), [41, 42, 43]);
  for (const comment of api.calls.comments) {
    assert.equal(comment.owner, repo.owner);
    assert.equal(comment.repo, repo.repo);
    assert.ok(comment.body.includes(`<!-- release-pr-comment:${release.id} -->`));
    assert.ok(comment.body.includes(`[${release.name}](${release.html_url})`));
  }
});

test('does not compare or notify without a previous release', async () => {
  for (const releases of [[], [release], [previous]]) {
    const api = fixture({ releases });
    await notify(api);
    assert.deepEqual(api.calls, { comments: [], comparisons: [], queries: [] });
  }
});

test('no commits or no associated PRs produces no notifications', async () => {
  for (const associations of [[], [[], []]]) {
    const api = fixture({ associations });
    await notify(api);
    assert.equal(api.calls.comparisons.length, 1);
    assert.equal(api.calls.queries.length, associations.length);
    assert.deepEqual(api.calls.comments, []);
  }
});

test('read failures propagate without posting incomplete notifications', async () => {
  for (const method of ['getRelease', 'listReleases', 'compareCommits', 'graphql']) {
    const api = fixture();
    const fail = async () => { throw new Error(`${method} failed`); };
    if (method === 'graphql') api.github.graphql = fail;
    else api.github.rest.repos[method] = fail;
    await assert.rejects(notify(api), new RegExp(`${method} failed`));
    assert.deepEqual(api.calls.comments, []);
  }
});

test('comment write failures propagate and stop later notifications', async () => {
  const api = fixture();
  api.github.rest.issues.createComment = async args => {
    api.calls.comments.push(args);
    throw new Error('comment forbidden');
  };
  await assert.rejects(notify(api), /comment forbidden/);
  assert.deepEqual(api.calls.comments.map(c => c.issue_number), [41]);
});
