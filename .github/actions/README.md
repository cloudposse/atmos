# Testing local actions

[`local-action-tests.yml`](../workflows/local-action-tests.yml) runs on relevant
pull requests, pushes to `main`, and manual dispatch. Script tests run on Linux,
macOS, and Windows with a read-only job token and no application secrets.

Run the same Starlark-backed custom commands locally (Atmos **1.233.0**,
Node.js **24**, Python **3.13** (as `python`), and npm must be on `PATH`):

```sh
atmos test local-actions
# Or run one category:
atmos test local-actions javascript
atmos test local-actions python
atmos test local-actions packages
```

Discovery and execution live in [`.atmos.d/test.yaml`](../../.atmos.d/test.yaml).
Regression tests exercise those Starlark steps with temporary filesystem fixtures
and a mock process runner, including empty discovery and failed test commands:

```sh
go test -race ./magefiles/localactions
```

Test discovery follows these conventions:

| Implementation | Discovered suite | Command |
| --- | --- | --- |
| Standalone JavaScript | `*.test.cjs` and `*.test.mjs` under `.github/actions/` and `.github/scripts/` | `node --test --experimental-test-coverage` |
| Python | Each action directory containing `test_*.py` | `python -m unittest discover` |
| npm/TypeScript action | Each action directory containing `package.json` | `npm ci`, `npm test`, and optional `npm run typecheck` |

New suites using these conventions join CI automatically. Generated `dist/`
and installed `node_modules/` directories are excluded. Discovery fails if a
suite category is unexpectedly empty. The authenticated SHA-pinning smoke
test named `test.mjs` is intentionally excluded; its offline suites run here.
This workflow runs test suites, not production action entrypoints or external
service mutations. Actions that only compose other actions have no standalone
script suite to discover.

The release rehearsal job installs GoReleaser **v2.18.3**, validates the
production configuration, and runs the Go release guards with the race detector.
The integration test creates tags only in temporary local repositories and
builds stable, nightly and feature fixture releases across all ten supported
platforms. A higher conflicting tag must not change their version. It exercises
real package generation, public asset names, staging, checksums and binary
verification; signing and SBOM files are explicitly unsigned test fixtures.

To run that rehearsal locally:

```sh
ATMOS_TEST_GORELEASER=/path/to/goreleaser-v2.18.3 \
  go test -tags=mage -race -count=1 ./magefiles/release ./internal/releaseidentity ./pkg/version
```

To rehearse actual GitHub signing, uploads and immutable publication, use an
isolated repository. Offline tests cannot establish GitHub App permissions,
OIDC acceptance, environment protections, or GitHub's behavior at publication.

## References to local actions

Use `$/` for actions from this repository. GitHub resolves these references to
[the exact workflow commit](https://github.blog/changelog/2026-07-30-reference-same-repository-actions-with-self-repository-syntax/),
including nested composite actions, without a workspace checkout.
The upstream Semgrep mutable-tag rule currently recognizes only `./` as local.
The rule-specific `nosemgrep` comments on `$/` references suppress this false
positive; third-party action references still require full commit SHA pins.

Semgrep retains these suppressed findings in SARIF with `inSource` suppression
metadata, which [GitHub code scanning does not consume](https://docs.github.com/en/code-security/reference/code-scanning/sarif-files/sarif-support).
The Semgrep job filters those findings before upload using
[the local filter-semgrep-sarif action](filter-semgrep-sarif/README.md), preserving the raw report and all
unsuppressed findings. Its regression tests run with the JavaScript suites.
