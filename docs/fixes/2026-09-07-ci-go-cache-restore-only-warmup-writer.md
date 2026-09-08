# Fix: CI Go caches were hollow or unreachable; warmup is now the sole writer

**Date:** 2026-09-07

## Summary

The Tests workflow's Go caches worked at the storage layer (writes succeeded, no LRU thrash) but
were functionally broken in several ways: the nightly warmup saved caches with an empty GOCACHE
that then froze (setup-go entries are immutable per go.sum hash), any go.sum change cold-started
every OS (no restore-keys), the Linux build job saved its warm cache to RunsOn's S3 where the 10
GitHub-hosted Linux shards could never read it, the kubernetes-e2e job nullified its own restored
cache with a `GOCACHE` override, and native-ci's ~700 MB provider-mirror cache only ever existed
on per-PR refs. The result was measured live: `go build` took 504 s and the race job 27 minutes
on every run despite "warm" caches, and each PR re-saved 1-1.8 GB per platform after any
dependency merge.

## Context

Investigated from the live GitHub cache API (92 entries / ~99 GB active on the repo) plus job/step
timings from run 34179823625. Root causes, in order of impact:

- `actions/setup-go`'s built-in cache is exact-key-only and immutable per key. Whichever job saved
  first froze the entry for that go.sum; when `setup-go-cache-warmup.yml` (which ran only
  `go mod download`) won that race, the frozen entry had a full module cache but an empty build
  cache, and every later build compiled from scratch while logging a cache hit.
- The warmup had no Linux leg at all, and no fallback existed for go.sum changes.
- The Build job's Linux leg ran with `extras=s3-cache` (RunsOn S3 backend), so the warmest cache
  in the run — and the only writer of the `atmos-toolchain-linux-amd64-v2` key — was invisible to
  every `ubuntu-latest` job restoring from GitHub's cache backend.
- `kubernetes-e2e` restored the Go cache and then set `GOCACHE: runner.temp/go-build` on the test
  step, compiling into an empty directory.
- `native-ci.yml` ran only on pull_request, so its cache key was saved on per-PR refs that no
  other PR can read; every PR re-mirrored providers from the network and saved its own copy.
- Multi-GB setup-go saves also landed on throwaway `gh-readonly-queue/*` merge-queue refs, and
  no-Go-work jobs (dependency-review) saved thin archives.

## Changes

- `.github/actions/setup-go-cache/action.yml`: replaced setup-go's built-in cache with an explicit
  `actions/cache/restore` (restore-only) using a custom key
  (`go-cache-<OS>-<arch>-go<version>-<go.sum hash>`) with restore-keys fallback (same-go.sum
  prefix first, then any same-OS/arch/Go-version entry). The key excludes ImageOS so RunsOn
  ubuntu22 legs and ubuntu24 hosted jobs share one lineage. GOCACHE/GOMODCACHE are pinned to
  deterministic paths on all platforms (the Windows D:-work-disk relocation is unchanged). No job
  using the composite ever saves.
- `.github/workflows/setup-go-cache-warmup.yml`: now the sole writer. Added a Linux leg; each leg
  runs the real compiles (`go build ./...` with CGO_ENABLED=0 + GOFIPS140=latest,
  `mage acceptance:precompile`, and on Linux `go build -race ./...` with CGO_ENABLED=1 to warm
  the race job's namespace) and saves under a run-unique key via `actions/cache/save`. Added a
  `push: main` trigger on go.mod/go.sum changes to close the post-dependency-merge cold window.
- `.github/workflows/test.yml`: Build linux leg and race job dropped `extras=s3-cache` (unifying
  on the GitHub cache backend); race job upgraded `runner=large` → `runner=xlarge` and switched
  to the composite plus a restore-only toolchain cache; coverage job switched from setup-go v6 to
  the composite; kubernetes-e2e's `GOCACHE` override removed; restore-only Atmos toolchain cache
  added to the `lint` matrix, `hooks-tflint`, `floci`, `floci-go`, and `race` jobs; stale cache
  comments rewritten.
- `.github/workflows/native-ci.yml`: PR cache steps are now restore-only; a new push-to-main
  `warm-cache` job (validate + provider mirror only, no PR-oriented plan/apply reporting) is the
  key's single writer on main's scope.
- `.github/workflows/codeql.yml`: the custom-gcl binary (a full golangci-lint build with the
  lintroller plugin) is cached keyed on `.custom-gcl.yml`, `tools/lintroller/**`, and the module
  graph; build steps are skipped on a hit.
- `.github/workflows/website-preview-build.yml`, `website-deploy-prod.yml`: pnpm setup moved
  before setup-node and the pnpm store is cached (`cache: pnpm`).
- `.github/workflows/dependency-review.yml`: `cache: false` (no Go work; the default restored
  ~1 GB it never read and saved a thin archive).
- `.github/actions/setup-atmos-build/action.yml`: switched from raw setup-go v6 to the composite.
- `atmos.yaml`: the `ci.cache` toolchain key now includes `hashFiles ".tool-versions"` so tool-pin
  bumps roll the key automatically instead of requiring a manual `-v2` → `-v3` bump.

Deliberately not done: relocating atmos's own cache root (the toolchain directory) to the Windows
work disk. The acceptance shards assert XDG-default paths (see the comment on the shards'
toolchain step), and the cache's saver and restorers must agree on the root, so the writer can't
move without moving every consumer.

## Validation

- Every edited workflow/action file parses with PyYAML (`python3 -c "import yaml; yaml.safe_load(open(f))"`
  looped over the files) and `actionlint` passes on every edited workflow.
- `go build ./...` and `go build -o build/atmos .` succeed; `./build/atmos ci cache paths
  --format=github` renders the new toolchain key
  (`atmos-toolchain-darwin-arm64-v2-5f174099f5f046d3`) with both restore-keys.
- `atmos test` (short suite) run locally.
- Dispatching the warmup on the PR branch (`gh workflow run ... --ref <branch>`) proved the
  workflow end-to-end (all four legs saved 1.7-2.5 GB entries) but does NOT warm that PR's own
  runs: a `pull_request` run reads only its merge ref and the base branch's scope, never the head
  branch's, so the PR's Tests run still logged "Cache not found" on every platform. The design is
  unaffected (main's scope is readable by every PR); it just cannot be measured pre-merge.
- The same cold run also showed the Build linux leg's compile at 721 s on the 2-core RunsOn
  `terraform` runner versus ~350 s on the 4-core hosted legs; that leg is the critical path every
  acceptance shard waits on, so it now runs on `large` (4 cores).
- Post-merge verification (requires main's cache scope, cannot be validated from a PR):
  dispatch `setup-go-cache-warmup.yml` and confirm multi-GB `go-cache-*` entries on
  `refs/heads/main` via the caches API, then compare Build(linux) `go build` time (expect
  ~504 s → well under 2 minutes) and race-job duration (expect ~27 m → ~10-15 m) on the next PRs.

## Follow-ups

None.
