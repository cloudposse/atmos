# Fix: `!include https://raw.githubusercontent.com/...` uses GitHub token auth when available

**Date:** 2026-07-09

## Problem

The `!include <url>` YAML function downloads remote files through
`pkg/downloader`'s go-getter-based client. For plain `https://` sources
(e.g. `raw.githubusercontent.com`), the underlying `getter.HttpGetter` had no
`Client` set, so the request went out fully unauthenticated regardless of
whether a GitHub token was configured or available in the environment
(`GITHUB_TOKEN` is exported into every GitHub Actions job).

`pkg/downloader/file_downloader.go` already special-cased these URLs
(`isGitHubHTTPURL`) to pre-check GitHub's rate limit via an authenticated API
client before fetching, but that check and the actual download were
disconnected: the pre-check used `github.GetGitHubToken()`, while the fetch
itself never attached that token, so it was still subject to the much lower
anonymous per-IP limit on `raw.githubusercontent.com`. In CI this
intermittently surfaced as `bad response code: 429` in tests that exercise
`!include` against a real GitHub raw URL (e.g.
`tests/yaml_functions_include_test.go`'s
`TestYAMLFunctionInclude/include_with_YQ_expression_from_JSON_file`).

Atmos already had a mature GitHub-token-authenticated HTTP transport
(`pkg/http.GitHubAuthenticatedTransport` + `WithGitHubToken`, host-allowlisted
to `api.github.com`/`raw.githubusercontent.com`/`uploads.github.com`, with
redirect-stripping) used by several other downloaders (Terraform registry
cache, toolchain registry/installer) — it just wasn't wired into this one.

## Fix

`pkg/downloader/gogetter_downloader.go`'s `goGetterClientFactory.NewClient`
now attaches a token-authenticated `*http.Client` to the `http`/`https`
getter when the source is a GitHub raw/archive URL (`isGitHubHTTPURL`) and a
token is available (`github.GetGitHubToken()`), reusing the existing
transport. Added `(*http.DefaultClient).HTTPClient()` to `pkg/http` so
callers needing the concrete `*http.Client` (like `go-getter`'s
`HttpGetter.Client` field) can get one without duplicating the transport/
redirect-stripping setup. Behavior for non-GitHub sources, or when no token
is configured, is unchanged — the getter's `Client` stays nil, exactly as
before.

## Tests

```shell
go test ./pkg/downloader/... ./pkg/http/... -run 'GitHubTokenAuth|HTTPClient' -v -count=1
```
