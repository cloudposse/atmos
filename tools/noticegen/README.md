# noticegen

Generates the repository's `NOTICE` file from Go dependency licenses.
Replaces the previous `scripts/generate-notice.sh`.

## Usage

Normally invoked via `go tool mage notice:generate` (see
`magefiles/mage_notice_generate.go`), which is what `atmos dev generate
notices` and CI both run. It can also be run directly:

```bash
go run . <repo-root> [output-path]
```

`repo-root` defaults to `.`; `output-path` defaults to `<repo-root>/NOTICE`.

## How it works

1. Installs [`go-licenses`](https://github.com/google/go-licenses) on demand
    (via `go install`) if it isn't already on `PATH`.
2. Runs `go-licenses report .` to get a `module,url,license` CSV for every
    Go dependency.
3. Rewrites the URL for modules `go-licenses` can't resolve reliably --
    vanity import paths (`dario.cat/mergo`, `inet.af/netaddr`, ...) and
    split-module repos (`gopkg.in/*.vN`, `cloud.google.com/go/*`) -- to a
    deterministic, version-pinned GitHub blob URL. See `overrides.go` for the
    per-module table and `googlecloud.go` for the generic
    `cloud.google.com/go[/*]` rule (one rule covers every submodule, since
    they all share a predictable tag/URL shape).
4. Fetches the repository's description live from the GitHub API
    (`GET /repos/cloudposse/atmos`) for NOTICE's tagline, and stamps the
    copyright line with the current year -- both regenerated fresh every run
    instead of hand-maintained, so they can't drift or go stale the way a
    literal string does. See `repo.go`.
5. Renders `NOTICE`, grouped by license family: Apache-2.0 and BSD sections
    are always present (even if empty); MPL-2.0 and MIT sections only appear
    when there's at least one matching dependency. Other license families
    (ISC, ...) are counted in the summary but not rendered as their own
    section -- this matches the file's historical format.

## Env knobs

Same as the previous shell script, for parity with any existing local
overrides:

| Variable               | Default | Purpose                                        |
|-------------------------|---------|-------------------------------------------------|
| `GO_LICENSES_VERSION`   | `v1.6.0`| `go-licenses` version installed if missing      |
| `LICENSE_GOOS`          | `linux` | Build target `go-licenses`/`go list` scan under |
| `LICENSE_GOARCH`        | `amd64` | ditto                                            |
| `LICENSE_CGO_ENABLED`   | `1`     | ditto                                            |
| `GH_TOKEN` / `GITHUB_TOKEN` (optional) | unset | Authenticates the GitHub API description fetch, avoiding the unauthenticated 60 req/hour rate limit; `GH_TOKEN` takes precedence, matching the `gh` CLI |

## Tests

`go test ./...` covers the pure logic (URL override computation, CSV
parsing, NOTICE rendering byte-for-byte) without any subprocess, plus an
end-to-end `Generate` test against a throwaway stub `go-licenses` binary
built at test time (kept a real compiled binary, not a shell script, so it
also runs on Windows).
