# PRD: CI Review Labels in Stack Configuration

## Problem

Preview infrastructure needs to follow labels on the current pull request or merge request.
Today, stack authors must inject that state through workflow-specific environment variables or
external commands. Atmos has a CI provider abstraction, but no YAML function for review labels.

Add `!ci.labels` so the same declaration can enable Atmos Pro and select affected components:

```yaml
components:
  terraform:
    preview:
      metadata:
        labels:
          preview: !ci.labels [preview]
      settings:
        pro:
          enabled: !ci.labels [preview]
```

```shell
atmos describe affected --labels preview:true
```

`--labels` narrows the affected set; it does not make an otherwise unaffected component affected.
Atmos labels live under `metadata.labels`, including stack-wide defaults inherited by components.

## Contract

| Expression | Result |
| --- | --- |
| `!ci.labels` | Sorted, unique list of current review label names |
| `!ci.labels [preview]` | Boolean: the review has `preview` |
| `!ci.labels [preview, deploy]` | Boolean: the review has both labels |
| `!ci.labels ["needs review"]` | Literal label names may contain spaces or punctuation |

Matching is exact and case-sensitive. Duplicated requested names do not change the result.
Arguments are a nonempty YAML sequence of literal strings; block sequences also work.
Empty sequences, mappings, scalar arguments, non-string elements, aliases, and nested function
arguments are rejected. Quote label names such as `"true"` and `"123"` to keep them strings.
Bare `!ci.labels` takes no arguments and returns a list, never null.

Without a review context (local execution, push events, or generic CI), list mode returns `[]`
and matching returns `false`, without credentials or API calls. If a review context exists but
its identity is incomplete, its provider lacks label support, or lookup fails, report an error.
Do not silently substitute event-payload labels, an empty list, or false for a failed live lookup.

## Provider and Evaluation Design

- Add an optional `LabelReader` capability to the existing provider interfaces, publicly aliased
  by `pkg/ci`. `ci.Labels(context.Context)` resolves the current provider and review and returns
  normalized labels. Ordinary provider detection and `Context()` remain free of label API reads.
- Initially implement GitHub Actions only. Use the existing CI context for PR identity, including
  supported `pull_request_target` and `workflow_run` contexts. Do not search for a local branch PR
  or infer reviews for push events. An unresolved review number is an error when a review is present.
- Support GitHub Actions `merge_group` runs by resolving the individual PR number from
  `merge_group.head_ref`, or `GITHUB_REF` / `GITHUB_REF_NAME` when absent, using the queue ref
  `refs/heads/gh-readonly-queue/<base-branch>/pr-<number>-<sha>`. Support base branches with slashes.
  Read only that PR's labels, even when earlier queued PRs contribute code to the checkout.
  Preserve the synthetic queue commit context separately from ordinary pull-request context.
  Unrecognized queue refs are errors during label evaluation; context detection makes no API calls.
- Read all pages using GitHub's Issues labels endpoint for the identified PR. Reuse the existing
  GitHub client, token precedence, API URL overrides, and Enterprise support. Label fetching is
  read-only and does not require CI reporting or Atmos Pro to be enabled.
- Cache successful lookups by provider, server, repository, and review number for one Atmos CLI
  invocation. Concurrent evaluations share the first complete live snapshot. Copy results so
  callers cannot mutate it. Clear the cache at CLI entry and exit; expose reset for embedded callers.
  Never cache partial responses or failures. Bound the API read to 30 seconds and honor cancellation.
- Register the function in the canonical tag catalog, function registry, YAML loader, legacy
  dispatcher, and deferred merge classification. Evaluate after merging. Preserve sequence
  arguments as a quoted flow sequence inside the deferred scalar, since generic scalar deferral
  loses sequence content. Return `[]any` for merge compatibility or a native boolean.
- Respect `--skip=ci.labels` and `--process-functions=false`: preserve the expression without
  fetching labels. Preserve normal inheritance and explicit overrides.

## Atmos Label Selectors

Boolean values in `metadata.labels` are valid. Keep them boolean in resolved stack output; the
shared label conversion formats them as `"true"` or `"false"` for selectors and label accessors.
Existing string labels retain their behavior, and other non-string values remain unsupported.
Both `preview:true` and `preview=true` select a boolean true value. Update the component and
global metadata schemas accordingly.

`!ci.labels` is an explicit exception to selector restrictions on authenticated external reads.
It is permitted because CI state is the requested input to selection and the invocation snapshot
keeps repeated comparisons consistent. The early scope gate treats unresolved expressions as
undecidable and permits normal evaluation; it must not discard them as absent labels. Existing
restrictions on shell execution, cloud lookups, secrets, stores, and arbitrary scripts remain.

The name follows CI platforms' label terminology even though the returned collection is a list.
This is distinct from Atmos's map-shaped `metadata.labels` and existing `!labels` function.

## Acceptance Criteria

- Unit coverage verifies parsing, exact/all-label matching, native return types, missing context,
  unsupported capability, incomplete identity, failure propagation, and defensive copies.
- Mock HTTP coverage verifies pagination, Enterprise API paths, empty responses, missing tokens,
  cancellation, non-success statuses, and failure after a successful first page.
- Concurrent and repeated evaluation performs one successful lookup per review per invocation;
  the next invocation can observe changed labels.
- Stack fixtures import and inherit `metadata.labels.preview` and `settings.pro.enabled`, retain
  explicit false overrides, and expose a sorted label list. Affected filtering with
  `--labels preview:true` includes only affected components whose resolved label is true.
- Merge-queue tests verify event-payload precedence, environment fallbacks, nested base branches,
  malformed refs, per-PR cache isolation, paginated Enterprise reads, and the inherited preview
  settings and affected-selector flow. Queue-looking push refs do not discover a review.
- Skipping the function or disabling function processing issues no label requests.
- Build, focused tests, repository short tests, and changed-code lint pass.

## Non-goals

No local review discovery, label mutations, additional CI providers, event-payload fallback,
aggregation across queued PRs, any-label matching mode, wildcard matching, new flags, or `!pr.labels` / `!ci.pr.labels` aliases.
