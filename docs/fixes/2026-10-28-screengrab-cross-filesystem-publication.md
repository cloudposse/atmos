# Fix: Publish screengrabs on the destination filesystem

**Date:** 2026-10-28

## Summary

Record temporary CLI screengrabs beside their destination so atomic publication works when the system temporary directory is on another filesystem.

## Context

The Screengrabs CI workflow failed with `OSError: [Errno 18] Invalid cross-device link` while replacing a cast from `/tmp` into the checkout. `os.replace` cannot rename files across filesystems.

## Changes

- Create the recording temporary directory inside the screengrab output directory; retain atomic replacement and automatic cleanup.
- Run recorder regression tests in the Screengrabs workflow and trigger it when the recorder or its tests change.
- Quote the workflow output-file path to satisfy shell validation.

## Validation

- The regression reproduced the cross-device failure twice before the fix by rejecting replacements from outside the output filesystem.
- `python3 -m unittest discover -s demo/casts -p test_screengrabs.py -v` passes. It checks publication, sanitization, reruns, cleanup, and preservation of an existing cast when recording fails.
- Real Atmos recordings of `policy check --help` succeed twice in a disposable project and pass the existing cast error/path validators.
- `actionlint .github/workflows/screengrabs.yaml` passes.

## Follow-ups

None.
