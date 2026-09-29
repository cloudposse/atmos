# Fix: Homebrew formula bump failed - `brew: command not found` (exit 127)

**Date:** 2026-09-28

## Summary

Publishing `v1.230.0` fired `build.yml`'s `homebrew` job ("Bump Homebrew
formula"), which failed:

```text
/home/runner/work/_temp/....sh: line 2: brew: command not found
##[error]Process completed with exit code 127.
```

`brew install atmos` therefore stayed on the previous version. The same failure
occurred on `v1.229.0` and `v1.228.0` (Docker and attest jobs succeeded each
time), so the formula was bumped manually on those releases.

## Root cause

The [previous fix](./2026-09-06-homebrew-bump-safe-system.md) dropped the broken
`dawidd6/action-homebrew-bump-formula` action and called `brew bump-formula-pr`
directly. The dropped action had implicitly put Homebrew on `PATH`; the direct
`run:` step does not.

GitHub Actions runs an unspecified-`shell` Linux `run:` step under `bash -e {0}`
(a non-login, non-interactive shell; setting `shell: bash` explicitly would use
`bash --noprofile --norc -eo pipefail {0}`). Neither form sources
`/etc/profile.d/*`, which is exactly what adds the runner-preinstalled
Homebrew's `bin` (`/home/linuxbrew/.linuxbrew/bin`) to `PATH`. So `brew` is not
found and the step exits 127. This is the "additional plumbing" the prior fix's
verification caveat anticipated ("If the direct command needs additional
plumbing...").

## Fix

**`.github/workflows/build.yml`** - load Homebrew explicitly at the top of the
step, before calling `brew`:

```yaml
run: |
  set -euo pipefail
  eval "$(/home/linuxbrew/.linuxbrew/bin/brew shellenv)"
  brew bump-formula-pr --no-browse --no-audit --version="${TAG#v}" atmos
```

`brew shellenv` prepends the Homebrew `bin` directories to `PATH` (and sets
`HOMEBREW_*` cellar/repository vars). Loading it from the absolute path avoids
depending on a login shell or on `/etc/profile.d` being sourced.

## Verification / caveats

- Fully verified only when the next release triggers `build.yml` on a real
  `release: published` event, since `/home/linuxbrew/.linuxbrew` only exists on
  the GitHub-hosted Ubuntu runner.
- **Immediate, release-independent fallback** (what unblocks a current version
  without waiting for CI): open the homebrew-core bump manually. On a workstation
  where local `brew` works:
  ```bash
  HOMEBREW_GITHUB_API_TOKEN=$(gh auth token) \
  HOMEBREW_NO_AUTO_UPDATE=1 HOMEBREW_NO_INSTALL_FROM_API=1 \
    brew bump-formula-pr --no-browse --no-audit --version=<version> atmos
  ```
  If local `brew` cannot run (e.g. its dev-gem bundle fails to build), edit
  `Formula/a/atmos.rb` on a `Homebrew/homebrew-core` fork branch - change only
  `url` (new `vX.Y.Z` tag) and `sha256` (of the source tarball), commit
  `atmos X.Y.Z`, and open the PR; BrewTestBot rebuilds the bottle block. This is
  what unblocked `v1.230.0` (homebrew-core PR #314102).
