# Fix: retry `go mod download` before `go run .` in the website workflows

**Date:** 2026-08-31

## Summary

The `website-deploy-preview` job (`.github/workflows/website-preview-build.yml`) failed with
`stream error: stream ID <n>; INTERNAL_ERROR; received from peer` on many unrelated modules
(`google.golang.org/genproto`, `github.com/jwalton/go-supportscolor`,
`github.com/jfrog/jfrog-client-go`, `github.com/updatecli/updatecli`, ...) during its "Generate
atmos-manifest schema" step, which runs `go run . stack schema ...`. This is the same transient
proxy.golang.org HTTP/2 mid-stream reset already fixed once for `go mod download` in
`scripts/build-atmos.sh`/`magefiles/build.go` (see
`docs/fixes/2026-08-25-build-atmos-go-mod-download-retry.md`), but the website workflows' `go run`
steps have no module cache warm-up before them and no retry protection at all -- a single reset
during any one of the ~1000+ transitive module downloads `go run .` triggers aborts the whole
step.

## Context

Reading the attached failure log (GitHub Job ID 99475416637): the "Generate atmos-manifest
schema" step ran `go run . stack schema website/static/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json`
directly, with no prior `go mod download` step, so Go had to resolve and download the entire
module graph inline. Around 40 `go: downloading ...` lines in, a batch of `##[error]` lines fired
simultaneously across completely unrelated modules -- `cloud.google.com/go/iam`,
`cloud.google.com/go/storage`, `cloud.google.com/go/monitoring`, `cloud.google.com/go/secretmanager`,
`github.com/jwalton/go-supportscolor`, `github.com/jfrog/jfrog-client-go`,
`github.com/updatecli/updatecli` -- all with the identical `stream error: stream ID <n>;
INTERNAL_ERROR; received from peer` message, several sharing the same underlying
`google.golang.org/genproto` transitive dependency. That breadth (many unrelated modules failing
identically, at the same instant) is the same signature as the two prior incidents in the
2026-08-25 fix doc: a CDN-side proxy.golang.org hiccup, not a dependency or code problem.

`.github/workflows/website-preview-build.yml` and `.github/workflows/website-deploy-prod.yml` both
run `go run . stack schema ...` / `go run . config schema ...` directly after `Set up Go`, with no
`go mod download` step first and no retry wrapper -- unlike the native CI build path, which now
goes through `magefiles/build.go`'s `runGoModDownload` (ported from `scripts/build-atmos.sh` per
`docs/fixes/2026-08-26-merge-main-go-mod-download-retry-port.md`). These two website workflows
never got that protection because they don't build the `atmos` binary via the mage target at all
-- they're standalone `go run` invocations that happened to predate the original fix.

## Changes

- `.github/actions/go-mod-download-retry/action.yml` (new): a composite action that runs
  `go mod download`, retrying up to 3 times with a 15s cooldown, mirroring the exact convention
  in `magefiles/build.go`'s `runGoModDownload` and `.github/actions/download-artifact-retry`.
  Extracted as a reusable action (rather than duplicating the shell loop) since both website
  workflows need it identically, and a future workflow doing a bare `go run`/`go build` without
  going through the mage build target can reuse it too.
- `.github/workflows/website-preview-build.yml`: added a "Download Go modules (with retry)" step
  right after "Set up Go", before the "Generate atmos-manifest schema" / "Generate atmos-config
  schema" steps -- both `go run .` invocations in this job share the one module cache warm-up.
- `.github/workflows/website-deploy-prod.yml`: same addition, covering this job's four `go run .`
  invocations (two schema kinds, each run twice for the `1.0` path and the version-specific path).

## Verification

- `python3 -c "import yaml; yaml.safe_load(open(...))"` parses all three changed/added files
  cleanly.
- `actionlint .github/workflows/website-preview-build.yml .github/workflows/website-deploy-prod.yml`
  reports no issues.
