# Fix: Homebrew formula bump failed - action broke on Homebrew 5.x safe_system

**Date:** 2026-09-06

## Summary

Publishing `v1.228.0` fired `build.yml`'s `homebrew` job ("Bump Homebrew
formula"), which failed:

```
action-homebrew-bump-formula/main.rb:32:in 'Homebrew.git':
  undefined method 'safe_system' for module Homebrew (NoMethodError)
```

`brew install atmos` therefore stayed on the previous version.

## Root cause

The job used `dawidd6/action-homebrew-bump-formula@v7`. Its Ruby helper
`Homebrew.git` calls `safe_system`, which the Homebrew 5.x now shipped on GitHub
runners no longer exposes in the module scope the action runs in. This is an
unresolved upstream incompatibility, not a stale pin: `v8` has byte-identical
`git`/`safe_system` code (verified against the tag's `main.rb`), and the
upstream Homebrew-5 fix was reverted (the pinned `v7` commit is literally
"Revert 'Fix dev-cmd loading on GitHub Actions with Homebrew 5.0'"). So bumping
the action does not help.

atmos lives in `Homebrew/homebrew-core` (not a cloudposse tap); the formula is a
source build from `github.com/cloudposse/atmos/archive/refs/tags/vX.Y.Z.tar.gz`.

## Fix

**`.github/workflows/build.yml`** - drop the broken action and call the command
it wrapped directly. `brew bump-formula-pr` itself is fine; only the action's git
wrapper hit `safe_system`:

```yaml
- name: "Bump Homebrew formula"
  env:
    HOMEBREW_GITHUB_API_TOKEN: ${{ secrets.GH_BOT_TOKEN }}
    HOMEBREW_NO_AUTO_UPDATE: "1"
    HOMEBREW_NO_INSTALL_FROM_API: "1"
    TAG: ${{ github.event.release.tag_name }}
  run: |
    set -euo pipefail
    brew bump-formula-pr --no-browse --no-audit --version="${TAG#v}" atmos
```

`HOMEBREW_NO_INSTALL_FROM_API=1` forces brew to use the cloned homebrew-core tap
so `bump-formula-pr` can edit the formula file. The command forks homebrew-core
under the bot and opens the version-bump PR.

## Verification / caveats

- Cannot be fully verified until the next release triggers `build.yml` on a real
  `release: published` event. If the direct command needs additional plumbing,
  the fallback is unchanged.
- **Immediate, release-independent fallback** (what unblocks a current version
  without waiting for CI): run the same command manually - it bypasses the action
  entirely:
  ```
  HOMEBREW_GITHUB_API_TOKEN=$(gh auth token) \
    brew bump-formula-pr --no-browse --version=<version> atmos
  ```
