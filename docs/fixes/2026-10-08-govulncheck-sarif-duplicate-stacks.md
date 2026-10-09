# Fix: Deduplicate govulncheck SARIF stacks before upload

**Date:** 2026-10-08

## Summary

Normalize exact duplicate stack objects in govulncheck's SARIF output so GitHub
code scanning accepts the report, preserving every finding and distinct trace.

## Context

[Job 113647222434](https://github.com/cloudposse/atmos/actions/runs/37876824022/job/113647222434)
completed the scan but failed when uploading `govulncheck.sarif`: results 5, 11,
12, 13, and 15 contained duplicate `stacks` entries. The SARIF schema requires
unique objects in that array. This was a report-format failure, not a scanner
execution failure or a reason to suppress vulnerability findings.

## Changes

- Run a jq filter after a successful scan and before SARIF upload, retaining the
  first occurrence of each complete stack object in its original order.
- Preserve result metadata, frame order, distinct traces, and results without
  stacks; leave scanner failures fatal and replace the report only after jq
  succeeds.
- Run a shell regression test in the govulncheck job covering duplicates,
  reordered object keys, distinct metadata and frame order, multiple runs,
  absent or empty stacks/results, idempotence, and malformed JSON.

## Validation

- `bash scripts/test-dedupe-sarif-stacks.sh` passed.
- `shellcheck scripts/test-dedupe-sarif-stacks.sh` passed.
- `actionlint .github/workflows/codeql.yml` passed.
- Extracted the five failing stack arrays from the full CI job log and applied
  the filter: result 11 decreased from 25 to 15 stacks, and each other affected
  result decreased from 247 to 145 stacks. Compared the complete reconstructed
  result objects to confirm that only exact duplicates were removed.
- The full govulncheck scan and GitHub upload require the updated CI run; the
  local reproduction validates the reported upload rejection without rerunning
  the scan's expensive call-graph analysis.

## Follow-ups

None.
