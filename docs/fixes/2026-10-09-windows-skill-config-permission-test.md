# Fix: Windows skill configuration permission regression test

**Date:** 2026-10-09

## Summary

Make the configuration-edit permission preservation test compare the file's actual permissions before and after editing.

## Context

Windows acceptance shard 7/10 failed in `TestConfigEditPreservesMode` at commit `a44f46dec7`. The test requested mode `0640` and expected that literal value after editing; Windows returned `0666`. Go's Windows `os.Chmod` implementation only uses the owner-write bit to control the read-only attribute, so Unix group and other permission bits are not represented.

The attached log contained only runner cleanup. The full [job log](https://github.com/cloudposse/atmos/actions/runs/38009745295/job/114089334179) identified the assertion failure.

## Changes

Capture the mode returned by `os.Stat` after fixture setup, then require the same mode after editing. This retains the Unix permission preservation regression and also runs on Windows. Production behavior is unchanged.

## Validation

- Full source package tests: `go test ./pkg/ai/skills/source -count=1`.
- Windows amd64 test cross-compilation: `GOOS=windows GOARCH=amd64 go test -c -o .context/ci-windows-3352/source.test.exe ./pkg/ai/skills/source`.
- Repository changed-code lint and fix-document validation.
- Windows runtime execution remains a CI check; local testing runs on macOS.

## Follow-ups

None.
