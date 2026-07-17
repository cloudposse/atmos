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
   `validate editorconfig` retains all vendored values (`default`, `gcc`,
   `codeclimate`, and `github-actions`) and adds `sarif` as an Atmos-owned
   renderer.
2. `validate schema` and `validate stacks` annotations/SARIF results carry a
   real line + column, not just a file path.
3. Zero behavior change to the default text-mode output of any command.
4. Reuse existing framework primitives (`pkg/ci`, `pkg/provenance`) — no parallel
   infrastructure.
5. Every validation entry point also supports Atmos-owned `--format=rich`, a
   human-readable source-context renderer. The aggregate `atmos validate`
   command renders failing validators as separate rich blocks; `validate.format:
   rich` supplies the shared persistent default.

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

**Verified live against real fixtures — two different JSON-Schema libraries are
in play, not one.** `validate schema` uses `gojsonschema` (dotted `Field()`,
no leading slash). `validate stacks`' manifest check
(`internal/exec/stack_processor_utils.go:1073-1130`) uses
`github.com/santhosh-tekuri/jsonschema` instead — confirmed by actually
triggering it: `atmos --chdir tests/fixtures/scenarios/invalid-manifest-schema
validate stacks` (a fixture added during this investigation — well-formed
`components:` map, wrong-typed `name` field) prints a raw JSON dump,
`{"valid":false,"errors":[{"keywordLocation":"/properties/name/type",
"instanceLocation":"/name","error":"expected string, but got number"}]}` — a
JSON-Pointer `instanceLocation`, not a `gojsonschema`-shaped message.

Don't write a new `yaml.Node`-walking indexer or reuse `pkg/provenance`'s
text-based parser — a better tool already exists and is already wired one
line above the point that needs it: `pkg/utils/yaml_position.go`.
```go
type Position struct{ Line, Column int } // 1-indexed
type PositionMap map[string]Position     // JSONPath-style key -> Position
func ExtractYAMLPositions(node *yaml.Node, enabled bool) PositionMap
func GetYAMLPosition(positions PositionMap, path string) Position
```
It walks a `yaml.Node` tree directly and joins paths via the same
`utils.AppendJSONPathKey`/`AppendJSONPathIndex` helpers `pkg/provenance` uses.
`stack_processor_utils.go:1031` already calls
`u.UnmarshalYAMLFromFileWithPositions[...]`, returning exactly this
`PositionMap`, immediately before the `compiledSchema.Validate(dataFromJson)`
call at line 1120 that produces the `instanceLocation`-bearing errors — the
position data this plan needs already exists in scope at the point of failure.

One catch, resolved: that function's own doc comment says "If
`atmosConfig.TrackProvenance` is false, returns an empty position map" — gated
today behind an unrelated, performance-motivated global flag. Don't flip that
global default (it's used elsewhere on the hot describe/deploy path); instead,
unconditionally enable position tracking for the duration of any `validate`
command's run — validate commands don't care about the performance cost
`TrackProvenance` was gated to avoid.

Concrete plan:
1. **`validate stacks`** (santhosh-tekuri path): after
   `compiledSchema.Validate(dataFromJson)` fails, call
   `utils.ExtractYAMLPositions(node, true)` for a `PositionMap`; write a small
   JSON-Pointer → `AppendJSONPathKey`/`AppendJSONPathIndex`-format converter
   (strip leading `/`, split on unescaped `/`, unescape `~1`→`/` and `~0`→`~`
   per RFC 6901); look up each `instanceLocation` via `utils.GetYAMLPosition`.
2. **`validate schema`** (gojsonschema path): same
   `utils.ExtractYAMLPositions`/`GetYAMLPosition` machinery — no new parser,
   no `pkg/provenance` dependency.
   ```go
   // pkg/validator/schema_validator.go
   type ValidationError struct {
       gojsonschema.ResultError
       Line, Column int // 0 = unknown, falls back to file-level annotation
   }
   func ValidateYAMLSchema(schema, sourceFile string) ([]ValidationError, error)
   func ValidateYAMLContent(schema string, yamlContent []byte) ([]ValidationError, error)
   ```

**First task, before any wiring**: verify `gojsonschema.Field()`'s exact join
format matches `AppendJSONPathKey`/`AppendJSONPathIndex`'s (dot-separated
keys, `parent[i]` index format, no separator before the bracket) — a mismatch
silently produces `Line=0` for every array-nested error.

We deliberately do **not** reach for `pkg/merge`'s cross-file provenance
tracking (`ProvenanceEntry{File, Line, Column}` — wired only into
`internal/exec/stack_processor_utils.go` today) — both `validate schema` and
`validate stacks` validate each file standalone (confirmed:
`internal/exec/validate_schema.go:printValidation` loops
`ValidateYAMLSchema(schema, file)` per file), so there's no merged value whose
source needs cross-file attribution.

**Live-tested finding, now precisely confirmed with three dedicated
fixtures — corrects an earlier, less precise version of this finding.**
Three separate, purpose-built scenarios (added during this investigation)
each isolate one failure category cleanly:

- `tests/fixtures/scenarios/invalid-manifest-schema/` (two files, `dev.yaml`
  + `staging.yaml`, each with a different JSON-Schema violation): **schema
  violations already aggregate correctly across files today** — both files'
  errors are reported together in one run. This corrects the earlier draft of
  this finding, which conflated this with the case below. One real,
  independent UX issue this surfaced: `staging.yaml`'s single mistake
  (`settings.templates` given as a string instead of an object) produces
  **7 near-duplicate bullets** for one conceptual error, because the schema's
  `oneOf`/`anyOf` construct reports every failed candidate branch
  individually (`"expected string, but got object"` right next to
  `"expected object, but got string"` for the same location). Worth
  deduplicating/collapsing oneOf branches in the `Diagnostic` model (PR 2) —
  keep only the most specific leaf per `instanceLocation`, or the deepest
  `keywordLocation`.
- `tests/fixtures/scenarios/invalid-stack-yaml-syntax/` (two files: `dev.yaml`
  has a raw YAML syntax error — an unterminated quoted string — and
  `staging.yaml` is otherwise perfectly valid): a raw parse failure **aborts
  the entire `validate stacks` run**, not just that one file — `staging.yaml`
  is never reached or mentioned at all, confirmed by testing both files
  present together. This is the real, more severe version of the earlier
  "fail-fast" finding: it's not "stops at the first bad file and reports nothing
  after," it's "one unparseable file blanks out every other file's
  diagnostics, valid or not." Fixing this (parse failures should produce one
  file-level `Diagnostic` and let the run continue to other files) is now
  confirmed as necessary, not just suspected.
- `tests/fixtures/scenarios/invalid-config-schema/` (pre-existing fixture,
  atmos.yaml with `base_path: 12345` and `stacks.base_path: ["invalid",
  "array"]`): this is a **third, structurally distinct failure category** —
  it fails during Viper/mapstructure config *decoding*, which happens for
  every Atmos command unconditionally, before `validate config`'s own
  gojsonschema check ever runs. Confirmed live: `atmos --chdir
  tests/fixtures/scenarios/invalid-config-schema validate config` never
  reaches `validate config`'s own logic at all — the CLI's own bootstrap
  fails first, the same way `atmos version` or any other command would
  against this same broken atmos.yaml. This is structurally different from
  stack manifests (parsed into a loosely-typed map, decode essentially never
  fails, so type mismatches surface cleanly as gojsonschema errors) —
  atmos.yaml's config schema is generated directly from strongly-typed Go
  structs, so most realistic type-violations manifest as decode errors, not
  schema errors. A genuine "atmos.yaml passes decode but fails schema" case
  would need something Go's type system can't express but JSON-Schema can
  (e.g. `additionalProperties: false` on a free-form map, a `pattern`
  constraint, an enum) — none were found at first pass; worth a deeper look
  during PR 3, but don't force a synthetic fixture for it if none exists
  naturally.

Also still true from the earlier draft: a separate, ad-hoc Go structural
check in `pkg/component/resolver.go:292` (`"invalid components section..."`)
fires for certain malformations *instead of* reaching the JSON-Schema
validator at all — not JSON-Schema-based, no line/column by construction. A
third `Diagnostic` source alongside the two JSON-Schema libraries, needing
its own small position lookup (via `utils.ExtractYAMLPositions` against the
`components` key).

**Revised per-file/per-run contract for PR 1** (per user direction: stage
the check, don't just show the first error): per file, produce at most one
syntax-level `Diagnostic` (parse failure) OR proceed to schema validation and
collect *all* violations for that file (deduplicated across `oneOf`
branches); across a directory, one file's parse failure must not suppress
every other file's diagnostics — continue processing remaining files and
report all of them together in the final `Report`.

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

`validate component`, `validate stacks`, and `validate schema` add, via
`pkg/flags` (matching `cmd/terraform/plan.go`):
```go
flags.WithStringFlag("format", "", "text", "Output format: text, sarif"),
flags.WithBoolFlag("ci", "", false, "Enable CI mode for automated pipelines (annotations, SARIF upload)"),
flags.WithEnvVars("format", "ATMOS_VALIDATE_FORMAT"),
flags.WithEnvVars("ci", "ATMOS_CI", "CI"),
```

`validate editorconfig` keeps its existing `--format` flag and expands its
accepted values to `default|gcc|codeclimate|github-actions|sarif`; it also adds the same `--ci` flag and
environment-variable wiring. `default` and `gcc` continue through the vendored
editorconfig-checker renderer unchanged. `sarif` is an Atmos-owned renderer:
do not pass it to the vendored library, whose output-format contract remains
`default|gcc|codeclimate|github-actions`.

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
  Render that one report as Atmos SARIF for `--format=sarif`; in `--ci` mode,
  use the same diagnostics for line-anchored annotations and the gated SARIF
  upload. Preserve the upstream `default` and `gcc` renderers byte-for-byte.

### Documentation (PR 7-equivalent, folded into per-command PRs or its own pass)

Update `website/docs/cli/commands/validate/{validate-component,validate-stacks,
validate-schema,validate-editorconfig,usage}.mdx` and the `stack validate` alias
doc page with new `--format`/`--ci` flag entries and CI-integration examples
(e.g. `atmos validate schema --format=sarif > results.sarif` piped into
`github/codeql-action/upload-sarif`). Document editorconfig's
all EditorConfig format values explicitly, including that `--ci` writes
file-and-line annotations and uploads SARIF only when `ci.results.enabled` is
set. Build via
`cd website && npm run build`.

### E2E workflow coverage (PR 3/4, alongside the Go tests)

`.github/workflows/native-ci.yml` + `tests/fixtures/scenarios/native-ci-e2e/`
already establishes the pattern for visually exercising native CI features
against a real PR: the fixture's own README is explicit that "generic
`format: sarif` coverage belongs in Go tests, not in this visual E2E workflow"
— that E2E is reserved for real, end-to-end annotation/Code-Scanning behavior,
the same way `terraform-plan`/`terraform-apply` there exercise real
checkov/trivy scanner hooks against intentionally-insecure Terraform, not a
synthetic SARIF fixture.

`validate` needs the same treatment once `--ci`/`--format=sarif` exist (not
before — nothing to exercise yet): a new `[native ci] validate` job in
`native-ci.yml`, running `atmos validate stacks --ci` (and/or `validate
schema --ci`) against a fixture with a genuine, isolated JSON-Schema
violation — mirroring `tests/fixtures/scenarios/invalid-manifest-schema/`
(added during this investigation: a minimal scenario with a well-formed
`components:` map but a wrong-typed `name` field, isolated from the other
`invalid-stacks/` fixture's mixed failure types so it actually reaches the
`santhosh-tekuri/jsonschema` path instead of failing fast on an unrelated
YAML syntax error first) — and asserting the resulting Code Scanning
annotation shows up on the PR diff at the right file/line. Add this job to
`native-ci.yml`'s `paths:` trigger alongside the existing `pkg/ci/**`/
`pkg/hooks/**` entries once it exists. Land this as part of PR 3 or PR 4
(whichever ships `--ci` first), not deferred to the docs-only PR 7.

## Files Changed (planned)

| File | Change | Status |
|------|--------|--------|
| `tests/fixtures/scenarios/invalid-manifest-schema/` | New: two isolated JSON-Schema-violation fixtures (`dev.yaml`, `staging.yaml`) — confirmed these already aggregate correctly | Done |
| `tests/fixtures/scenarios/invalid-stack-yaml-syntax/` | New: raw YAML syntax error (`dev.yaml`) alongside an otherwise-valid file (`staging.yaml`) — confirmed the syntax error currently suppresses the valid file's result too, not just its own | Done |
| `pkg/utils/yaml_position.go` | Reuse as-is (`ExtractYAMLPositions`/`PositionMap`/`GetYAMLPosition`) — no changes needed | N/A |
| `internal/exec/stack_processor_utils.go` | Line/column via `formatManifestSchemaValidationErrors`+`jsonPointerToPositionKey` (done, ad hoc — see below); still needed: dedupe `oneOf` cascades, and stop one file's parse failure from suppressing every other file's diagnostics | Partially done |
| `pkg/validator/schema_validator.go` | `ValidationError` type with Line/Column via `pkg/utils/yaml_position.go` | Planned |
| `pkg/component/resolver.go` | Third `Diagnostic` source (ad-hoc "components must be a map" check) — needs its own small position lookup | Planned |
| `internal/exec/validate_stacks.go` | Stage YAML-parse check before schema check per file; return `*validate.Report` | Planned |
| `pkg/validation/diagnostic.go` | New: `Diagnostic`, `Severity`, `Report` | Planned |
| `pkg/validation/sarif.go` | New: SARIF 2.1.0 encoder | Planned |
| `pkg/ci/mode.go` (or similar) | New: `ModeEnabled`, `HooksEnabled` | Planned |
| `cmd/terraform/utils.go` | `terraformCIModeEnabled` → thin wrapper over `ci.ModeEnabled` | Planned |
| `internal/exec/validate_utils.go` | `ValidateWithJsonSchema`/`ValidateWithOpa*` return violation detail | Planned |
| `.github/workflows/native-ci.yml` | New `[native ci] validate` job (PR 3/4) | Planned |
| `cmd/validate_schema.go`, `cmd/validate_stacks.go`, `cmd/validate_component.go`, `cmd/validate_editorconfig.go`, `cmd/stack/validate.go` | `--format`/`--ci` flags + dispatch; editorconfig supports all existing vendored formats plus `sarif` | Planned |
| `website/docs/cli/commands/validate/*.mdx` | Flag docs + CI examples | Planned |

## Verification

- `go build ./... && atmos test` after each PR.
- `atmos validate schema --format=sarif` against this repo's own `atmos.yaml` →
  valid SARIF 2.1.0.
- `atmos validate editorconfig --format=gcc` preserves the existing GCC output;
  `atmos validate editorconfig --format=sarif` emits valid SARIF 2.1.0 from the
  same file-and-line diagnostics.
- `GITHUB_ACTIONS=true GITHUB_REPOSITORY=... atmos validate stacks --ci` against
  a stack with a known bad manifest → `::error file=...,line=...::` on stderr,
  gated correctly by `ci.enabled`.
- `GITHUB_ACTIONS=true GITHUB_REPOSITORY=... atmos validate editorconfig --ci`
  against a known formatting violation → file-and-line annotation plus gated
  SARIF upload, both derived from the same diagnostic report.
- `atmos test --full` before opening each PR; `atmos lint --changed` before every
  commit.
