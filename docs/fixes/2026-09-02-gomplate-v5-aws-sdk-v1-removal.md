# Fix: Remove AWS SDK for Go v1 from the build by migrating templates to gomplate v5

**Date:** 2026-09-02

## Summary

OpenSSF Scorecard's Vulnerabilities check flagged four Go advisories against `go.mod`. Three were fixed by
upgrading the template engine dependency from `gomplate/v3` to `gomplate/v5` (which removes
`github.com/aws/aws-sdk-go` v1 from the module graph entirely) and bumping `golang.org/x/crypto` to v0.56.0.
The fourth (`GO-2026-5932`, the `x/crypto/openpgp` "unsafe by design" notice) has no fix in any version and
is documented here as accepted.

## Context

Scorecard runs osv-scanner over `go.mod` and matches module versions; it does no call-graph analysis, so a
vulnerable module anywhere in the graph counts. `govulncheck` (call-graph aware, `.github/workflows/codeql.yml`)
flagged none of these, confirming nothing reachable was affected. The findings:

| Advisory | Module | Resolution |
|---|---|---|
| GO-2026-6354 / GO-2026-6355 (SSH mux DoS) | `golang.org/x/crypto` v0.55.0 | Bumped to v0.56.0 (direct dependency, used by `pkg/store/providers/github_actions_client.go` for `nacl/box`). |
| GO-2022-0635 / GO-2022-0646 (`s3crypto`) | `github.com/aws/aws-sdk-go` v1 | No v1 release fixes them (the fix is SDK v2, a different module). Removed the module from the build. |
| GO-2026-5932 (`openpgp` unsafe by design) | `golang.org/x/crypto`, all versions | Not fixable: the notice covers every version, `x/crypto` is a required direct dependency and is pulled in by many others; Atmos never imports `openpgp`. Accepted. |

`go list -deps ./...` showed exactly which packages imported SDK v1, and all of them entered through gomplate:
`gomplate/v3/aws` and `gomplate/v3/data` (imported directly by `internal/exec` and `pkg/generator/engine`),
`gomplate/v4/aws` (only because of a dead blank import of `gomplate/v4` in `internal/exec/template_utils.go`
with zero API usage), `hashicorp/go-secure-stdlib/awsutil` v0.3.0 and `hashicorp/vault/api/auth/aws` v0.12.0
(gomplate's Vault datasource auth), and `gocloud.dev/aws` + `gocloud.dev/blob/s3blob` at the v0.41.0 that
`go.mod` had pinned for gomplate/v3. `versent/saml2aws` requires SDK v1 only at the module level and imports
none of its packages, so module-graph pruning drops it.

Moving to gomplate v4 would not have helped: v4.3.3 requires SDK v1 directly. gomplate v5.0.0 replaced SDK v1
with v2 in its own code, but v5's Vault datasource auth still imports `hashicorp/vault/api/auth/aws`, whose
latest tag (v0.12.0) imports SDK v1. HashiCorp `main` has already ported that nested module to `awsutil/v2`
and `aws-sdk-go-v2` but has not tagged it, so `go.mod` requires the ported revision as the pseudo-version
`v0.12.1-0.20260828212235-e96dd83d431a`. Minimal version selection picks it over gomplate's v0.12.0 require,
so no `replace` directive is needed (a `replace` would break the documented `go install` path and is rejected
by `tools/gomodcheck`). Dependabot's `gomod` configuration only ignores semver-major updates and covers
`// indirect` requires, so it will propose the bump as soon as HashiCorp tags a version above the
pseudo-version (or gomplate releases against one); the comment on that `go.mod` line says to delete it then.

gomplate v5's library API differs from v3: `CreateFuncs(ctx)` no longer takes a datasource object and no longer
includes the datasource functions (`datasource`, `ds`, `include`, `defineDatasource`, ...), which exist only
inside gomplate's renderer, and that renderer only accepts datasource-backed template contexts and updates a
package-global metrics map (not safe for concurrent use, while Atmos renders stack files concurrently).

## Changes

- New `pkg/templating` package, the only importer of gomplate. `Engine.Render` parses with `text/template`
  first and executes there (lock-free, in-memory `.` data, unchanged error text) unless the template calls a
  renderer-only function or `atmos.GomplateDatasource`; those renders go through gomplate's renderer,
  serialized by a mutex, via a one-action wrapper that uses gomplate's public `tmpl.Inline` to execute the
  user's template with the in-memory data (so struct data and integer types are preserved, no temp-file
  round-trip). `Engine.Datasource` serves `atmos.GomplateDatasource` from the live render, with a cache keyed
  by alias and arguments (the old cache ignored arguments). Datasource URLs are parsed the way gomplate's CLI
  parses them (bare absolute paths, Windows drive/UNC paths, `-` for stdin).
- `pkg/template.UsesFunctions` inspects a parsed template for function calls; `walkAST` now descends into
  `ChainNode` expressions such as `(ds "cfg").name`, which it previously skipped.
- `internal/exec/template_utils.go`, `template_funcs.go` and `pkg/generator/engine/templating.go` are thin
  call sites; `template_funcs_gomplate_datasource.go` and the dead `gomplate/v4` import are gone.
  `ProcessTmplWithDatasourcesGomplate` now honors its `ignoreMissingTemplateValues` argument.
- `go.mod`: `gomplate/v3` and `/v4` dropped, `gomplate/v5` v5.2.0 added, `hashicorp/vault/api/auth/aws`
  pinned to the ported pseudo-version (with an explanatory comment), `golang.org/x/crypto` v0.56.0. The
  `gocloud.dev`/`go-fsimpl` pins that existed for gomplate/v3 are removed. gomplate v5 requires
  `invopop/jsonschema` v0.14.0, which was pinned to v0.13.0 until `anthropic-sdk-go` migrated to
  `pb33f/ordered-map`; `anthropic-sdk-go` v1.69.0 has, so it was bumped and `pkg/project/config` switched to
  the same ordered-map module. `NOTICE` regenerated.
- `tools/gomodcheck` now also fails when `go.mod` requires a forbidden module (`github.com/aws/aws-sdk-go`
  exactly; `-v2` never matches), with tests and a CI step in `.github/workflows/test.yml`.
- Docs: datasource behavior notes and the removed/renamed gomplate function list
  (`website/docs/templates/datasources.mdx`, `website/docs/cli/configuration/templates.mdx`,
  `website/docs/functions/template/atmos.GomplateDatasource.mdx`), changelog post
  `website/blog/2026-09-02-gomplate-v5.mdx`, roadmap milestone.

## Validation

```bash
go list -deps ./... | grep -c 'github.com/aws/aws-sdk-go/'   # 0
grep -c 'github.com/aws/aws-sdk-go ' go.mod                   # 0
go build ./... && CGO_ENABLED=0 go build .                    # ok
go vet ./pkg/templating/ ./pkg/template/ ./internal/exec/ ./pkg/generator/engine/ ./pkg/project/config/
go test -race ./pkg/templating/ ./pkg/template/               # ok
go test ./internal/exec/ -run 'Template|Tmpl|Sprig|Gomplate|Datasource|DocsGenerate|Locals|Delimit|Env'
go test ./pkg/project/config/... ./pkg/config/schema/... ./pkg/ai/agent/anthropic/... ./pkg/generator/engine/...
go test ./pkg/stack/... ./pkg/tags/... ./pkg/vendoring/component/... ./pkg/hooks/... ./pkg/describe/...
go test -short ./...
atmos lint --changed                                          # includes tools/gomodcheck on go.mod
go tool mage notice:generate
cd website && npm run build
```

The atmos.yaml schema drift test (`pkg/config/schema`) passed without regeneration after the
`invopop/jsonschema` bump.

## Follow-ups

None. Removing the `hashicorp/vault/api/auth/aws` pseudo-version pin is driven by Dependabot (see Context);
no manual tracking is required.
