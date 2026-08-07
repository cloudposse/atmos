# Fix: Release Docker build no longer pulls buildkit/binfmt images from AWS public ECR

**Date:** 2026-08-06

## Summary

The release `docker` job in `.github/workflows/build.yml` was rate-limited pulling its buildx
builder (`moby/buildkit`) and QEMU binfmt images from `public.ecr.aws`. Bumped
`cloudposse/github-action-docker-build-push` to v3.1.0 (Google-mirrored buildkit image by
default) and explicitly overrode the action's `binfmt-image` input to the equivalent
Google-mirrored `tonistiigi/binfmt` image, since that input still defaults to `public.ecr.aws`
even at v3.1.0.

## Context

CI run for `cloudposse/atmos` release build (job at
`https://github.com/cloudposse/atmos/actions/runs/31040133433/job/92444923915`) failed with
`jq: error (at inspect.json:78): Cannot iterate over null (null)` in the action's post-build
summary step, which turned out to be a separate, already-tracked upstream bug
(`cloudposse/github-action-docker-build-push#102`, cosmetic — the image itself built and pushed
fine) and not something fixable from this repo. On the next attempted run, the job failed again,
this time on rate limiting while pulling the buildx builder image from `public.ecr.aws`.

## Changes

- `.github/workflows/build.yml`: bumped
  `uses: cloudposse/github-action-docker-build-push@...` from v3.0.0
  (`f06d0f4bd286898b613412d2fcc6622e5b68bbdc`) to v3.1.0
  (`02993d675b44dcc7082e6de7485c1ff8740bce9d`), which changes the action's `driver-opts` default
  from `image=public.ecr.aws/vend/moby/buildkit:buildx-stable-1` to
  `image=mirror.gcr.io/moby/buildkit:buildx-stable-1`.
- `.github/workflows/build.yml`: added an explicit `binfmt-image:
  mirror.gcr.io/tonistiigi/binfmt:qemu-v7.0.0` input, since the action's `binfmt-image` default
  (`public.ecr.aws/eks-distro-build-tooling/binfmt-misc:qemu-v7.0.0`) still pulls from AWS public
  ECR and has no upstream fix yet. `binfmt-image` passes straight through to
  `docker/setup-qemu-action`'s `image` input, so it can be overridden directly without waiting on
  upstream.

## Validation

- Diffed `cloudposse/github-action-docker-build-push` v3.0.0...v3.1.0 upstream: only the
  `driver-opts` default change and an unrelated arm64 `jq` install fix; no input/output contract
  changes to any input this workflow uses (`registry`, `organization`, `repository`, `login`,
  `password`, `platforms`, `file`, `build-args`).
- Confirmed `public.ecr.aws/eks-distro-build-tooling/binfmt-misc:qemu-v7.0.0` is an AWS rebuild of
  upstream `tonistiigi/binfmt`, which publishes the identical `qemu-v7.0.0` tag on Docker Hub.
- Verified live against the registries: `docker buildx imagetools inspect
  mirror.gcr.io/tonistiigi/binfmt:qemu-v7.0.0` and the `docker.io/tonistiigi/binfmt:qemu-v7.0.0`
  equivalent both resolve to the same digest
  (`sha256:66e11bea77a5ea9d6f0fe79b57cd2b189b5d15b93a2bdb925be22949232e4e55`) across all 7
  published platforms, and `docker pull` of the mirrored tag succeeds.
- This is a `release`-triggered workflow (`on.release.types: [published]`), so it cannot be
  exercised by a normal PR run; verification here is by inspection plus the live registry checks
  above. The next actual release's Docker build job should be watched once to confirm no more
  rate-limit failures.
- `./custom-gcl run --new-from-rev=origin/main` — 0 issues.

## Follow-ups

None. The action's `binfmt-image` default itself is still AWS-ECR-backed upstream with no fix in
flight; if it starts rate-limiting independently of this override, no further action is needed
here since this repo already pins its own Google-mirrored value.
