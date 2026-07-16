# Validate Command Parity — PROPOSED

> Related: [Interfaces](../framework/interfaces.md) | [Hooks Integration](../framework/hooks-integration.md) | [CI Detection](../framework/ci-detection.md) | [Implementation Status](../framework/implementation-status.md) | [Apply Command Parity](./apply-command-parity.md) (structural precedent)

## Status: PROPOSED

Nothing implemented yet. This PRD tracks a 6-PR sequence (PRD → line/column
plumbing → shared diagnostics model → one PR per `atmos validate` subcommand →
docs). Update this doc's status and the per-PR checklist below as each PR merges;
add a summary entry to [Implementation Status](../framework/implementation-status.md)
once fully shipped, following the existing phase-doc convention.

## Problem Statement

Atmos's native-CI framework (`pkg/ci/`) already provides provider-neutral GitHub
Actions inline annotations (`ci.Annotate`), SARIF upload to GitHub Code Scanning
(`ci.ReportSARIF`), collapsible log groups (`ci.StartLogGroup`), and CI-provider
auto-detection (`ci.IsCI`). It's consumed today by the security-scanner hooks
(checkov/trivy/kics, via `pkg/hooks/command_engine.go`) and by Terraform's `--ci`
mode (see [Apply Command Parity](./apply-command-parity.md)).

None of the four `atmos validate` subcommands use it:

| Subcommand | File | Current output |
|---|---|---|
| `validate component` | `cmd/validate_component.go` → `internal/exec/validate_component.go` | Spinner + `(bool, error)`; violation detail swallowed |
| `validate stacks` | `cmd/validate_stacks.go` → `internal/exec/validate_stacks.go` | `[]string` messages joined into one `errors.New(...)` |
| `validate schema` | `cmd/validate_schema.go` → `internal/exec/validate_schema.go` | `log.Error` per `gojsonschema.ResultError` (file + dotted JSON path, no line/column) |
| `validate editorconfig` | `cmd/validate_editorconfig.go` | Own `--format` (`default`/`gcc`, from the vendored editorconfig-checker library); already has file+line |

Validation failures are exactly the kind of finding that benefits most from
inline PR annotations and SARIF-based code-scanning integration — e.g. a red
squiggle on the exact line of a stack manifest that violates its JSON Schema —
but today a user only sees this in local CLI text output or a CI log dump.

`cmd/stack/validate.go` (added by the companion branch introducing
`atmos validate schema`) is a pure alias for `atmos validate stacks`
(`RunE` calls the same `exec.ExecuteValidateStacksCmd`) — it inherits whatever
`validate stacks` gets, plus its own `--format`/`--ci` flags for parity.

## Goals

1. All four `validate` subcommands support `--ci` (annotations + SARIF
   auto-upload when a CI provider is detected and `ci.enabled: true`) and
   `--format=sarif` (plain SARIF 2.1.0 file/stdout output, independent of CI).
2. `validate schema` and `validate stacks` annotations/SARIF results carry a
   real line + column, not just a file path.
3. Zero behavior change to the default text-mode output of any command.
4. Reuse existing framework primitives (`pkg/ci`, `pkg/provenance`) — no parallel
   infrastructure.

## Non-Goals

- Validating the *merged/effective* config (post-imports) instead of each file
  standalone. All four commands keep validating files independently; extending
  `pkg/merge`'s cross-file provenance tracking to `pkg/config` (currently used
  only for stack-manifest processing, gated by `atmosConfig.TrackProvenance`) is
  a separate, larger initiative — noted as a follow-up idea, not built here.
- Template-aware validation. `pkg/config` never renders Go templates for
  `atmos.yaml`, and `internal/exec/validate_stacks.go` already validates stack
  manifests pre-template (`ProcessTemplates: false`) and excludes `**/*.tmpl`
  files outright. This PRD doesn't change that trade-off — it's why line numbers
  can map 1:1 to the source file with no render-time shift to reconcile.
- Relocating `internal/exec/validate_*.go` business logic into `pkg/`. Three of
  the four commands' logic already lives in `internal/exec/`, which is
  pre-existing debt this initiative didn't create; only the new CI/SARIF code
  goes into purpose-built packages.

## Solution

### Line/column plumbing (PR 1)

`pkg/validator/schema_validator.go` converts parsed `yaml.Node` trees to a plain
`any` before `gojsonschema.Validate`, discarding `Node.Line`/`Node.Column`.
`gojsonschema.ResultError.Field()` gives a dotted path (e.g.
`components.terraform.vpc.vars.cidr`) but no position.

Rather than write a new `yaml.Node`-walking indexer, reuse
`pkg/provenance/yaml_parser.go`'s `buildYAMLPathMap` (already parses a single
YAML document into a line→path map, with existing test coverage in
`yaml_parser_arrays_test.go`/`yaml_parser_multiline_test.go`/`yaml_indent_test.go`).
Add an exported entry point:

```go
// pkg/provenance
func LookupPosition(yamlContent []byte, path string) (line, column int, ok bool)
```

And wrap `gojsonschema` results:

```go
// pkg/validator/schema_validator.go
type ValidationError struct {
    gojsonschema.ResultError
    Line, Column int // 0 = unknown, falls back to file-level annotation
}
func ValidateYAMLSchema(schema, sourceFile string) ([]ValidationError, error)
func ValidateYAMLContent(schema string, yamlContent []byte) ([]ValidationError, error)
```

**First task, before any wiring**: verify `YAMLLineInfo.Path`'s join format
(`pathSeparator = "."`, array indices via `utils.AppendJSONPathIndex` as
`parent[i]`) exactly matches `gojsonschema.Field()`'s join format. A mismatch
silently produces `Line=0` for every array-nested error.

`internal/exec/validate_stacks.go`'s manifest-schema path doesn't call
`pkg/validator` directly — it goes through `ProcessYAMLConfigFile`/
`ProcessStackConfig` and collapses straight to a joined string. This needs
tracing and updating to preserve `ValidationError`, mirroring the `validate
schema` approach.

We deliberately do **not** reach for `pkg/merge`'s cross-file provenance
tracking (`ProvenanceEntry{File, Line, Column}` — wired only into
`internal/exec/stack_processor_utils.go` today) — both `validate schema` and
`validate stacks` validate each file standalone (confirmed:
`internal/exec/validate_schema.go:printValidation` loops
`ValidateYAMLSchema(schema, file)` per file), so there's no merged value whose
source needs cross-file attribution.

### Shared diagnostics model (PR 2)

```go
// pkg/validate/diagnostic.go
type Severity string
const (
    SeverityError   Severity = "error"
    SeverityWarning Severity = "warning"
    SeverityNotice  Severity = "notice"
)

type Diagnostic struct {
    Source   string // "component" | "stacks" | "schema" | "editorconfig"
    RuleID   string
    Severity Severity
    Message  string
    File     string
    Line     int // 0 = file-level, no line anchor
    Column   int
    Field    string // gojsonschema-style dotted path, when applicable
}

type Report struct{ Diagnostics []Diagnostic }
func (r Report) HasErrors() bool
func (r Report) ToAnnotations() []ci.Annotation
```

Lives in a new `pkg/validate/` package — not `pkg/ci` (transport/wire-format
only: `ci.Annotation`/`ci.SARIFReport` are wire shapes, not a domain model) and
not `pkg/validator` (narrowly the JSON-Schema engine). Additive: no existing
text-output call site (`ui.Successf`, `log.Error`, spinner messages) changes;
each command's exec layer *also* appends `Diagnostic`s to a `Report`, rendered
only when `--format=sarif` or `--ci` is active.

**SARIF writer** (`pkg/validate/sarif.go`): a lean encoder mirroring
`pkg/aws/security/sarif.go`'s `SARIFLog`/`Run`/`Result`/`Location`/`Region`
shape — not importing those types directly, since that package is
AWS-finding-specific (ARNs, compliance standards) and reuse would mean either
empty AWS-only fields or a layering violation. `pkg/ci.SARIFReport` is just a
transport envelope (`{Body []byte, Category string}`); the document-building
necessarily lives with the producer.

### CI-mode gating (PR 2)

Two distinct decisions, both needed, easy to conflate:

1. **CI-mode output** — matches existing `--ci` semantics across
   `terraform`/`helmfile`/`kubernetes`/`helm`: `--ci` flag (if set) →
   `viper.GetBool("ci")` → `ci.IsCI()`. Extract
   `cmd/terraform/utils.go:terraformCIModeEnabled`'s logic into
   `pkg/ci.ModeEnabled(cmd *cobra.Command) bool`; make the terraform helper a
   thin wrapper so there's one implementation, not a 5th/6th/7th duplicate.
2. **CI hook firing** (annotations, SARIF auto-upload) — gated separately by
   `atmosConfig.CI.Enabled` (`ci.enabled` in `atmos.yaml`), the documented hard
   kill switch (see [CI Detection](../framework/ci-detection.md#ci-enabled-is-a-hard-kill-switch)),
   enforced today at `pkg/hooks/command_engine.go:567` before any call into
   `pkg/ci`. This exists because the generic `CI` env var is `true` on every CI
   runner — without this gate, `ci.IsCI()` auto-detection would silently start
   firing annotations/SARIF uploads on every CI run with no opt-out. Add
   `pkg/ci.HooksEnabled(atmosConfig *schema.AtmosConfiguration) bool` mirroring
   `command_engine.go:567`, and call it before every `ci.Annotate`/
   `ci.ReportSARIF` site added below. `--format=sarif`'s plain file/stdout
   output is unaffected by this gate — it's an output format choice, not a CI
   hook.

### Per-command wiring (PRs 3–6)

All four commands add, via `pkg/flags` (matching `cmd/terraform/plan.go`):
```go
flags.WithStringFlag("format", "", "text", "Output format: text, sarif"),
flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (annotations, SARIF upload)"),
flags.WithEnvVars("format", "ATMOS_VALIDATE_FORMAT"),
flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
```
Exception: `validate editorconfig` keeps its own `--format` (`default`/`gcc`,
unrelated semantics) — gets `--ci` only.

- **`validate component`**: `ValidateWithJsonSchema`/`ValidateWithOpa`/
  `ValidateWithOpaLegacy` (`internal/exec/validate_utils.go:36/83/175`) return
  `(bool, error)` today with violation detail swallowed — extend to return
  violation-level detail threaded into `[]validate.Diagnostic`.
- **`validate stacks`**: `ValidateStacks` returns `(*validate.Report, error)` in
  addition to keeping its existing joined-string error for the text path.
  `cmd/stack/validate.go` gets the same flags for parity.
- **`validate schema`**: `printValidation` already iterates
  `Field()/Type()/Description()` — append a `Diagnostic` per error (with
  Line/Column from PR 1) into an accumulated `*validate.Report`.
- **`validate editorconfig`**: adapt the vendored checker's
  `[]er.ValidationErrors` (already has file+line) into `[]validate.Diagnostic`.

### Documentation (PR 7-equivalent, folded into per-command PRs or its own pass)

Update `website/docs/cli/commands/validate/{validate-component,validate-stacks,
validate-schema,validate-editorconfig,usage}.mdx` and the `stack validate` alias
doc page with new `--format`/`--ci` flag entries and CI-integration examples
(e.g. `atmos validate schema --format=sarif > results.sarif` piped into
`github/codeql-action/upload-sarif`). Build via `cd website && npm run build`.

## Files Changed (planned)

| File | Change | Status |
|------|--------|--------|
| `pkg/provenance/yaml_parser.go` | Export `LookupPosition` | Planned |
| `pkg/validator/schema_validator.go` | `ValidationError` type with Line/Column | Planned |
| `internal/exec/validate_stacks.go` | Preserve position through manifest-schema path; return `*validate.Report` | Planned |
| `pkg/validate/diagnostic.go` | New: `Diagnostic`, `Severity`, `Report` | Planned |
| `pkg/validate/sarif.go` | New: SARIF 2.1.0 encoder | Planned |
| `pkg/ci/mode.go` (or similar) | New: `ModeEnabled`, `HooksEnabled` | Planned |
| `cmd/terraform/utils.go` | `terraformCIModeEnabled` → thin wrapper over `ci.ModeEnabled` | Planned |
| `internal/exec/validate_utils.go` | `ValidateWithJsonSchema`/`ValidateWithOpa*` return violation detail | Planned |
| `cmd/validate_schema.go`, `cmd/validate_stacks.go`, `cmd/validate_component.go`, `cmd/validate_editorconfig.go`, `cmd/stack/validate.go` | `--format`/`--ci` flags + dispatch | Planned |
| `website/docs/cli/commands/validate/*.mdx` | Flag docs + CI examples | Planned |

## Verification

- `go build ./... && atmos test` after each PR.
- `atmos validate schema --format=sarif` against this repo's own `atmos.yaml` →
  valid SARIF 2.1.0.
- `GITHUB_ACTIONS=true GITHUB_REPOSITORY=... atmos validate stacks --ci` against
  a stack with a known bad manifest → `::error file=...,line=...::` on stderr,
  gated correctly by `ci.enabled`.
- `atmos test --full` before opening each PR; `atmos lint --changed` before every
  commit.
