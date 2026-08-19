# Fix: packer build tests missed a Windows-only stdout-draining flake

**Date:** 2026-07-30

## Summary

`TestPackerBuildCmdWithDirectoryTemplate` failed on Windows CI (job 91001781247, commit
`f0c83bcc98`) with `Error: Failed loading manifest ... EOF` — text packer's own manifest
post-processor produces, never Atmos. The test's `packerRan` heuristic (checking the captured
output for `amazon-ebs`/`Build`/`credential`/`Packer`) didn't recognize this specific packer error
text, so it fell through to `t.Errorf` even though packer clearly ran. Added `"Failed loading
manifest"` to the recognized set in both `TestPackerBuildCmd` and
`TestPackerBuildCmdWithDirectoryTemplate`, which share the identical heuristic and are equally
exposed to the same flake.

## Context

The exact same commit (`8c46159629`, this PR's merge base) passed cleanly on `main`'s own CI run
about an hour earlier — confirming this is intermittent, not a deterministic regression. Both
tests run sequentially in `cmd/packer_build_test.go`, each temporarily swapping `os.Stdout`/
`os.Stderr` to an `os.Pipe()` to capture the real `packer` subprocess's output. On Windows,
subprocess stdout draining across that swap boundary can occasionally split a single build's
output: this run's raw CI log shows the credential-failure text (`No valid credential sources
found`, `Build 'amazon-ebs.al2023' errored...`) landing in the terminal around the *prior* test's
position, while only the downstream manifest post-processor failure (a direct consequence of the
build never producing an artifact) made it into `TestPackerBuildCmdWithDirectoryTemplate`'s own
captured buffer. `"Failed loading manifest"` citing a `main.pkr.hcl` line number is packer-HCL
diagnostic output that Atmos cannot produce, so its presence alone already satisfies both tests'
stated intent ("verify packer was invoked"), independent of whether the underlying environmental
cause was credentials or something else.

## Changes

- `cmd/packer_build_test.go`: added `strings.Contains(output, "Failed loading manifest")` to the
  `packerRan` OR-chain in both `TestPackerBuildCmd` and `TestPackerBuildCmdWithDirectoryTemplate`,
  with a comment explaining the Windows stdout-draining race and why this specific string is safe
  to treat as packer-only evidence.

## Validation

- `go build ./...` — clean.
- `go test ./cmd/...` — all pass; the two packer tests skip gracefully locally (no `packer`
  binary installed on this machine), confirming the change compiles and the skip path is
  unaffected.
- `atmos lint --changed` — 0 issues.
- Could not reproduce the Windows-specific stdout-draining race locally (requires Windows + a
  real `packer` binary + an environment without AWS credentials); confidence comes from directly
  reading the packer HCL source (`main.pkr.hcl:29`'s `post-processor "manifest"` block) and the
  raw CI log's line ordering, not a local repro.

## Follow-ups

None. If this specific flake recurs with a *different* unrecognized packer error string, extend
the same `packerRan` OR-chain rather than reaching for a broader/looser match.
