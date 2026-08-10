---
name: security-remediate
description: "Fix open Dependabot and CodeQL/code-scanning alerts directly on the current branch. Triggered automatically by the security-remediate-trigger PostToolUse hook after a git push where GitHub reports open vulnerabilities; can also be invoked manually. Never opens a new PR or issue - commits land on the branch that's already open."
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Security Remediate (Atmos)

Proactively fix GitHub-reported security alerts on the branch you're already working on, instead of letting them accumulate until a manual sweep (see commit `0e32451913` for what that manual sweep looked like).

## Fork / permission prerequisite (read first)

This repo is public, so a plain `gh` token with `repo` scope (the default from `gh auth login`) is **sufficient** to read both alert endpoints — `security_events` is only required on **private** repos (see GitHub's Dependabot alerts API docs: private repos need `security_events`, public repos only need `public_repo`/`repo`). Don't chase a `security_events` scope refresh for this repo; it's unnecessary and, per `cli/cli#12073` and independent reports (e.g. `microsoft/sarif-sdk#3081`), `gh auth refresh -s security_events` can report success while GitHub silently drops the scope anyway — a dead end that's easy to mistake for a real permission gap.

If `gh api` calls below still 404/403 even with `-X GET` present, that's a genuine permission gap (e.g. a fork contributor without repo access, or a future private-repo use of this skill lacking `security_events`) — stop and report it instead of attempting a workaround. `.claude/hooks/security-remediate-trigger.sh` treats that case exactly like "no alerts open" and silently does nothing.

## Workflow

1. **Resolve context.**
   ```bash
   owner_repo="$(gh repo view --json owner,name --jq '.owner.login + "/" + .name')"
   branch="$(git rev-parse --abbrev-ref HEAD)"
   ```

2. **Query both alert sources live** (the hook's cached alert set was only a trigger signal — treat it as stale):
   ```bash
   gh api "repos/${owner_repo}/dependabot/alerts" -X GET --paginate -f state=open
   gh api "repos/${owner_repo}/code-scanning/alerts" -X GET --paginate -f state=open
   ```
   The `-X GET` is required: `gh api` implicitly sends a `POST` when `-f`/`--paginate`
   params are given without an explicit method, and these list endpoints 404 on `POST`
   — indistinguishable from a real permission error unless you notice the verb. If
   either call still 403/404s with `-X GET` present, stop and report the permission gap
   — do not proceed partway.

3. **Filter to high-confidence fixes**, following the precedent set by `0e32451913`:
   - **Dependabot alerts**: only fix if a patched version exists *within* what `.github/dependabot.yml`'s `ignore` rules allow (that file currently ignores all major-version bumps across `gomod`, `github-actions`, and `npm`/website). If the only fix requires a major bump the policy blocks, do not bump it — report it as "blocked by dependabot ignore policy" instead.
   - **CodeQL alerts**: only fix rule IDs with an established safe pattern in this repo. Currently that's `go/allocation-size-overflow`, fixed by avoiding summed `len()` capacity hints (e.g., replacing `make([]T, len(a)+len(b))` with a pattern that doesn't sum lengths as a capacity hint). Extend this list cautiously — an unfamiliar rule ID with an ambiguous fix goes in the "not auto-fixable" bucket, not a guessed fix.
   - **GitHub Action pins**: bump to the patched SHA/tag referenced by the alert.
   - **npm/pnpm alerts** (in `website/`): bump via `pnpm.overrides` in `website/package.json`, then regenerate `NOTICE`:
     ```bash
     ./scripts/generate-notice.sh
     ```

4. **Apply fixes in the working tree**, then verify before committing anything:
   ```bash
   make lint
   go test ./... # or scope to the touched packages
   ```
   If either fails for a given fix, drop that fix from the commit and move it to the "not auto-fixable" report — don't commit something that doesn't pass.

5. **Commit directly onto the current branch.** No new branch, no new PR, no issue — this branch already has an open PR, and the fix belongs on it:
   ```bash
   git add <only the files you actually changed>
   git commit -m "fix(security): remediate N CodeQL/Dependabot alerts"
   ```

6. **Report a summary back in chat** (not as a GitHub comment/issue): which alerts were fixed (numbers + URLs), and which weren't, with a reason for each (major-bump blocked by policy, ambiguous/unfamiliar rule ID, no patch available yet, permission gap). Never silently drop an alert without accounting for it in this summary.

## What this skill must never do

- Open a new pull request.
- Open a new GitHub issue (per standing project guidance: never open issues without explicit per-issue user authorization).
- Push past a dependabot.yml `ignore` rule.
- Commit a fix that fails `make lint` or the targeted tests.
