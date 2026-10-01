# Fix: exclude AWS SSO Portal API reference page from link checking (CI connection resets)

**Date:** 2026-08-18

## Summary

CI's "Check Markdown Links" job failed with 2 errors, both against the same URL
(`https://docs.aws.amazon.com/singlesignon/latest/PortalAPIReference/API_ListAccountRoles.html`),
referenced from `docs/prd/sso-role-auto-discovery.md:1454` and
`docs/prd/tags-and-labels-standard.md:1408`. The log showed `Network error: Connection reset by peer
(os error 104)` for the first occurrence and `Error (cached)` for the second (lychee caches
per-URL results within a run). The URL resolves fine outside CI (`curl -sIL` returns `200 OK`,
served via CloudFront). Excluded this one page from `lychee.toml`, following the repo's extensive
existing precedent for this exact class of CI-runner-specific connection-reset flakiness against
CloudFront-fronted hosts (e.g. `docs.docker.com`, `otelic.com`, `goo.gle`, `playwright.dev`).

## Context

Neither affected file was part of any in-flight change in this workspace — both are pre-existing
PRD documents unrelated to the current branch's work. The flagged link:

- `docs/prd/sso-role-auto-discovery.md:1454`
- `docs/prd/tags-and-labels-standard.md:1408`
- `https://docs.aws.amazon.com/singlesignon/latest/PortalAPIReference/API_ListAccountRoles.html`

`curl -sIL` against the URL returned `HTTP/1.1 200 OK` (`X-Cache: Miss from cloudfront`), confirming
the page is live and reachable; the failure is CI-runner-specific (CloudFront intermittently
resetting connections from automated/CI traffic), matching the documented rationale already
attached to several other `lychee.toml` excludes for CDN-fronted hosts.

## Changes

- `lychee.toml`: added one narrow `exclude` regex entry
  (`docs\.aws\.amazon\.com/singlesignon/latest/PortalAPIReference/API_ListAccountRoles\.html`)
  scoped to this single page, not the whole `docs.aws.amazon.com` domain — the rest of that
  domain's links (including sibling `PortalAPIReference`/`APIReference` pages in the same two
  files) check fine, so a domain-wide exclude would hide genuinely broken links elsewhere on AWS's
  docs site.

## Validation

- `curl -sIL https://docs.aws.amazon.com/singlesignon/latest/PortalAPIReference/API_ListAccountRoles.html`
  — `HTTP/1.1 200 OK`.
- `lychee --config lychee.toml docs/prd/sso-role-auto-discovery.md docs/prd/tags-and-labels-standard.md`
  — `🔍 14 Total ✅ 12 OK 🚫 0 Errors 👻 2 Excluded` (both occurrences now show `[EXCLUDED]`).

## Follow-ups

None. This URL is excluded from automated checking going forward, consistent with how the other
connection-reset-prone hosts in `lychee.toml` are handled.
