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

Querying alerts requires a `gh` token with the `security_events` scope **and** at least maintain-level access to the repo. Most contributors — including anyone working from a fork — will not have this. That's expected and safe: `.claude/hooks/security-remediate-trigger.sh` treats a 403/404 exactly like "no alerts open" and silently does nothing. If you're invoking this skill manually and see auth/permission errors below, stop and report that instead of attempting fixes — do not try to work around missing scope.

Check first:
```bash
gh auth status
```
Look for `security_events` in the token scopes. If it's missing, run `gh auth refresh -s security_events` (requires you actually have access to the repo's security tab) or just stop here and tell the user remediation isn't possible with the current credentials.

## Workflow

1. **Resolve context.**
   ```bash
   owner_repo="$(gh repo view --json owner,name --jq '.owner.login + "/" + .name')"
   branch="$(git rev-parse --abbrev-ref HEAD)"
   ```

2. **Query both alert sources live** (the hook's cached alert set was only a trigger signal — treat it as stale):
   ```bash
   gh api "repos/${owner_repo}/dependabot/alerts" --paginate -f state=open
   gh api "repos/${owner_repo}/code-scanning/alerts" --paginate -f state=open
   ```
   If either call 403/404s, stop and report the permission gap — do not proceed partway.

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
