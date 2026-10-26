# Fix: exclude k3s.io from link checking (CI connection resets)

**Date:** 2026-09-08

## Summary

CI's "Check Markdown Links" job (run 34241617451, job 102112946723, on branch
`osterman/json-manager-format-field`) failed with 1 error against
`https://k3s.io/`, referenced from `examples/emulator-k8s/README.md:14`. The log showed
`Network error: Connection reset by peer (os error 104)`, surviving lychee's configured 3 retries.
The site resolves fine outside CI (`curl` returns `200 OK`, served via GitHub Pages). Excluded the
domain from `lychee.toml`, following the repo's extensive existing precedent for this exact class
of CI-runner-specific connection-reset flakiness (e.g. `taskfile.dev`, `playwright.dev`,
`asdf-vm.com`, `docs.docker.com`).

## Context

The attached CI failure log (`.context/attachments/.../Check_Markdown_Links_102112946723.log`)
only contained the job's last ~1000 lines, which was entirely StepSecurity harden-runner
network-egress telemetry with no lychee output at all. Fetched the full job log directly
(`gh api repos/cloudposse/atmos/actions/jobs/102112946723` for the run ID, then
`gh run view 34241617451 --job=102112946723 --log`) to find the actual failure: lychee's summary
reported `🚫 Errors: 1`, scoped to a single link:

- `examples/emulator-k8s/README.md:14`
- `https://k3s.io/`
- `Network error: Connection reset by peer (os error 104)`

`curl -sS -o /dev/null -w "%{http_code}"` against the URL returned `200` on two consecutive
attempts, confirming the site is live and reachable outside CI; the failure is CI-runner-specific.

## Changes

- `lychee.toml`: added one narrow `exclude` regex entry (`k3s\.io`) for the whole domain — it's
  referenced from only two files (`examples/emulator-k8s/README.md`,
  `examples/local-gitops/README.md`), both simple citation links to the project homepage, so a
  narrower single-URL exclude would offer no more precision than the domain-wide one and both
  references benefit from the fix.

## Validation

- `curl -sS -o /dev/null -w "HTTP %{http_code}" https://k3s.io/` — `HTTP 200` (run twice).
- `lychee --config lychee.toml examples/emulator-k8s/README.md` — `🔍 3 Total ✅ 1 OK 🚫 0 Errors
  👻 2 Excluded`, with `https://k3s.io/` now shown as `[EXCLUDED]`.
- Local lychee is v0.22.0 vs. CI's pinned v0.24.2; version drift is why local runs weren't used to
  validate the rest of the ~1900-link full-repo check, only the specific fix.

## Follow-ups

None. This domain is excluded from automated checking going forward, consistent with how the other
connection-reset-prone hosts in `lychee.toml` are handled.
