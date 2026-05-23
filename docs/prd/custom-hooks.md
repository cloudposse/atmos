# Custom Hooks

**Status**: 🟡 In Progress (kind system, scanner kinds, `--skip-hooks`, dependency auto-install, and workdir compatibility have shipped on the in-flight PR; Pro upload backend still pending — see Implementation Plan below)

**Last Updated**: 2026-05-22

**Related PRDs**: [Hooks Component Scoping](./hooks-component-scoping.md) | [Tool Dependencies Integration](./tool-dependencies-integration.md) | [Native CI Integration](./native-ci-integration.md) | [CI Summary Templates](./ci-summary-templates.md)

## Overview

Extend the existing hook system so external tools (infracost, checkov, trivy, …) can be wrapped as a hook **kind** and invoked against a component via the existing `before/after.terraform.*` lifecycle. Tool output flows to Atmos Pro automatically when Pro is connected, and to local storage otherwise.

This is a **minor tweak** to today's hook architecture, not a new subsystem:

- `Hook.Command` (today's dispatch discriminator) is renamed to `Hook.Kind`. The legacy `command:` YAML key is accepted as an alias so existing `command: store` hooks keep working unchanged.
- The `command:` field is freed up to mean **what gets executed** — the binary the hook runs.
- A handful of new kinds (`command` for generic use, plus named kinds `infracost`, `checkov`, `trivy`, `kics`) are registered alongside the existing `store` kind.
- If Pro is connected, hook output uploads automatically. No per-hook upload configuration.

## Problem Statement

Today's `pkg/hooks/` only supports one active kind: `store` (read Terraform outputs into a parameter store). Users who want to run security scanners, cost estimators, or compliance tools against their components currently have to:

1. **Wrap them in GitHub Actions** — Couples the integration to GitHub, duplicates CLI logic, requires bash glue scripts between steps. Doesn't run locally.
2. **Wrap them in custom commands** — Works, but bypasses the hook lifecycle, doesn't tie into `before/after.terraform.*` events, and can't surface results in a unified Pro UI.
3. **Hardcode them per-tool in core** — A `case "scanner-x"` branch in `Hooks.RunAll` doesn't scale; every new tool would ship in core with no extensibility path.

Meanwhile, Atmos Pro has structured upload endpoints today (`UploadInstances`, `UploadInstanceStatus`, `UploadAffectedStacks`) but no generic artifact path — so even if users captured tool output, there's no clean way to publish it.

## Goals

1. **Generic engine, ergonomic presets** — One generic hook kind (`command`) runs any tool. Named kinds (`infracost`, `trivy`, etc.) ship sane defaults so common cases need zero configuration.
2. **Tool-agnostic Pro contract** — Pro receives `kind` + `tool_version` + bytes. Adding a new tool to Atmos requires a corresponding renderer in Pro, but no API/DTO changes.
3. **Implicit uploads** — If Pro is connected, hook output flows there. No `upload:`, `to:`, `output_format:`, or `producer:` fields in user YAML.
4. **Natural tool output** — Tool stdout/stderr streams through Atmos's I/O layer (`pkg/io/` + `pkg/ui/`) so users see infracost / trivy output in real time, the same way they see Terraform output today.
5. **Format symmetry** — `format: markdown` renders the same way in the terminal, on the Pro run page, and in PR comments.
6. **Toolchain-pinned** — Hook binaries resolve through `pkg/toolchain/` so the same pinned version runs locally and in CI.
7. **Skippable at runtime** — Operators can disable hooks for a single invocation (`--skip-hooks` / `ATMOS_SKIP_HOOKS`) without editing stack config. Useful for debugging, emergency operations, or local iteration where scans / cost estimates are noise.
8. **Back-compat** — Existing `command: store` hooks continue to work without modification.

## Non-Goals

- **A new component "kind" for scanners / cost tools.** Scanners don't have state, inputs, or a lifecycle; modeling them as peers of terraform/helmfile forces them into a workspace-shaped hole. They're hooks, not components.
- **tfsec support or examples.** Aqua Security folded tfsec's IaC scanning into Trivy and states that engineering attention is directed to Trivy. The standalone tfsec binary remains available for legacy users, but Atmos won't introduce a built-in `tfsec` kind, a bundled `hooks-tfsec` example, or docs that position tfsec as a recommended integration. Users who insist on tfsec can still wire it manually with `kind: command`; Trivy is the maintained Aqua-backed path and ships as a built-in kind.
- **User-defined kinds in YAML** (`hooks.kinds:` registry). Atmos already has stack imports for sharing config across components. A parallel registry duplicates the existing reuse mechanism.
- **Per-hook destination configuration** (`upload:`, `to:`, `output:`). Destination is a project-level concern (Pro on/off); leaking it into every hook adds noise without enabling anything users actually want.
- **Per-tool Pro endpoints** (`/findings`, `/cost-estimate`). Over-fits the API to today's tools. One generic artifact endpoint + summary envelope generalizes.
- **Cross-run finding aggregation in Pro v1.** Server-side dedup / fingerprinting of SARIF is high-value but doubles scope. Phase 2.
- **Standalone scan command** (`atmos scan vpc -s prod`). Could be added later as a thin wrapper that triggers a synthetic event and reuses the kind machinery. Out of scope here.

## Architecture

### Hook kind system

`kind:` is the dispatch discriminator. `command:` is the binary executed. Same word at two levels because the generic kind is centered on the `command:` field — K8s `kind: Service` / `spec.serviceName` precedent.

#### Built-in kinds

| Kind        | Engine          | Output                       | Pro renderer        |
|-------------|-----------------|------------------------------|---------------------|
| `store`     | StoreEngine     | Terraform outputs to store   | n/a (no upload)     |
| `command`   | CommandEngine   | Whatever the tool emits      | Generic (see below) |
| `infracost` | CommandEngine   | JSON breakdown               | Cost-diff card      |
| `checkov`   | CommandEngine   | SARIF findings               | SARIF viewer        |
| `trivy`     | CommandEngine   | SARIF findings               | SARIF viewer        |
| `kics`      | CommandEngine   | SARIF findings               | SARIF viewer        |

Each named kind ships defaults for `command`, `args`, `on_failure`, and a `ResultHandler` that parses the tool's structured output into a summary envelope. Users only set hook fields they want to override.

#### Two output channels

The tool's stdout / stderr stream straight through Atmos's I/O layer (`pkg/io/` + `pkg/ui/`). The user sees infracost progress, trivy warnings, and human-readable output in real time — same as Terraform output today. **Nothing gets swallowed.**

Structured machine-readable output is a **side channel** via `ATMOS_OUTPUT_FILE`: the tool writes its JSON / SARIF / HTML to that temp path via its own `--out-file` / `--output` flag (declared in the kind's `DefaultArgs`). The engine reads that file and hands it to:

1. The kind's `ResultHandler` for summary parsing.
2. The Pro artifact backend for upload (if Pro is enabled).

Stdout is for humans; the side-channel file is for machines.

#### Interpolation

Two distinct interpolation surfaces:

**Go templates in YAML strings** use the standard Atmos template context (same as everywhere in stack files — hooks already render with `ProcessTemplates: true` in `pkg/hooks/hooks.go`):

- `{{ .atmos_component }}`, `{{ .atmos_stack }}`, `{{ .atmos_stack_file }}`
- `{{ .vars.<name> }}`, `{{ .settings.<name> }}`, `{{ .workspace }}`
- `{{ .providers.<name> }}` plus all gomplate/sprig funcs

**Runtime values for the subprocess** are passed as environment variables to the executed binary — no new template variables introduced:

- `ATMOS_COMPONENT_PATH` — on-disk path to the Terraform module
- `ATMOS_PLANFILE` — planfile path (on after-plan events)
- `ATMOS_OUTPUT_FILE` — temp file the tool writes structured output to (for tools that take a single output file path)
- `ATMOS_OUTPUT_DIR` — temp directory containing `ATMOS_OUTPUT_FILE` (for tools like KICS that write to a directory rather than a file)
- `ATMOS_STACK`, `ATMOS_COMPONENT` — names

Tools consume these directly (e.g. `infracost --path "$ATMOS_COMPONENT_PATH"`).

### Pro integration — implicit upload, kind-driven

If Pro is configured for the project (same connection that drives instance-status and affected-stacks uploads today), hook output flows to Pro automatically.

**Pro learns everything from `kind`.** It maintains its own catalog of kinds and knows what each one emits and how to render it. The Atmos client sends `kind` + `tool_version` (resolved from the toolchain lockfile) + the bytes. There is no `media_type`, no `producer`, no `output_format` field in the data model or wire format.

Pro primitives, both keyed on `kind`:

- **Run summary** — typed envelope `{ kind, status, title, counts?, body? }` that every kind's `ResultHandler` fills the same way. Drives the run-page card, PR check status, dashboard list view.
- **Artifact** — opaque blob tagged with `kind` + `tool_version` + optional `format` (for generic kinds) + metadata.

#### Pro renderer classes

**Named-kind renderers** — bespoke UI for each kind Atmos ships. Adding a new named kind to Atmos requires a paired renderer in Pro, but no API/DTO changes.

- `infracost` → cost-diff card with per-resource table
- `checkov` / `trivy` / `kics` → SARIF findings viewer with severity filters

**Generic content renderer** — one format for v1, used with `kind: command` (and any future generic component that wants to send an artifact):

- `format: markdown` → rendered markdown
- omitted → stored as a downloadable artifact, no inline rendering

Markdown is the only generic format in v1 because it's the universal rich-content format: it renders cleanly in the terminal (via the existing `ui.Markdown()` in `pkg/ui/`), on Pro's run page, in PR comments, in step summaries — same content, every surface. Other formats (html, json, yaml, text) can be added later if a concrete need emerges; until then they're opaque artifacts.

**Format symmetry**: `format:` isn't a Pro-specific knob. Whatever the hook declares, every consumer renders the same way. A `format: markdown` hook's output appears as rendered markdown in the user's terminal during `atmos terraform plan` AND on the Pro run page — same bytes, same rendering.

#### Pro endpoints

```
POST /workflow-runs/{id}/summaries
POST /workflow-runs/{id}/artifacts
GET  /workflow-runs/{id}/artifacts/{name}
```

A new `pkg/ci/artifact/pro/` backend slots into the existing `pkg/ci/artifact/` registry. Tool integrations upload through the same `artifact.Store` interface S3 / GitHub Artifacts already use — Pro is one more backend.

HTTP `Content-Type` on the upload is implementation plumbing — set by the Pro backend from kind metadata or by content sniffing — not part of the user-facing data model.

### Kind registry

A kind is defaults + an optional result handler, registered in Go via `init()`:

```go
// pkg/hooks/kinds/infracost/kind.go
func init() {
    hooks.RegisterKind(&hooks.Kind{
        Name:    "infracost",
        Command: "infracost",
        DefaultArgs: []string{
            "breakdown",
            "--path", "$ATMOS_COMPONENT_PATH",
            "--format", "json",
            "--out-file", "$ATMOS_OUTPUT_FILE",
        },
        OnFailure:     "warn",
        ResultHandler: parseInfracostJSON,
    })
}
```

`Name` is all Pro needs to identify the kind; tool version comes from the toolchain lockfile.

**Resolution**: kind defaults → hook field overrides. Sharing across components goes through normal stack imports — same mechanism every other section of Atmos uses for reuse.

### Disabling hooks at runtime

Atmos does not have a runtime hook-skip flag today (verified across `pkg/hooks/`, `cmd/terraform/utils.go`, and the flag layer). This PRD adds one because the hook surface materially expands with tool kinds, and operators need an escape hatch that doesn't require editing stack config.

**Flag**: `--skip-hooks` accepts either a boolean (skip all) or a comma-separated list of hook names (skip specific ones).

**Env**: `ATMOS_SKIP_HOOKS` mirrors the flag.

**Precedence**: CLI flag > env var > config (standard Atmos precedence; see `pkg/flags/`).

```bash
atmos terraform plan vpc -s prod --skip-hooks               # skip all
atmos terraform plan vpc -s prod --skip-hooks=cost,security # skip specific
ATMOS_SKIP_HOOKS=true atmos terraform apply vpc -s prod     # via env
```

Skipping is logged at INFO level so it's visible in run output and CI logs ("Skipped hooks: cost, security").

Scope is per-invocation only — `--skip-hooks` does not propagate to nested commands or workflows; each invocation makes its own decision. Implementation lives in `cmd/terraform/utils.go` next to `runHooks` / `runHooksWithOutput`, and applies uniformly to all kinds (store, command, infracost, trivy, …) — it's a layer above the kind dispatch, not a per-kind concern.

### Toolchain integration

Tool binaries are pinned via the in-flight `dependencies.tools` syntax from [Tool Dependencies Integration](./tool-dependencies-integration.md). The hook engine invokes through `pkg/toolchain.Exec` so the exact pin used locally also runs in CI.

```yaml
components:
  terraform:
    vpc:
      dependencies:
        tools:
          infracost: "0.10.x"
          trivy:     "0.50.x"
      hooks:
        cost:     { events: [after.terraform.plan], kind: infracost }
        security: { events: [after.terraform.plan], kind: trivy }
```

## Schema

### Hook YAML

```yaml
hooks:
  # ── Existing store kind, unchanged semantics ──────────────────────────
  vpc-id:
    events: [after.terraform.apply]
    kind: store              # 'command: store' still accepted as alias
    name: vpc/id
    outputs:
      id: .vpc_id

  # ── Named tool kind — all defaults ────────────────────────────────────
  cost:
    events: [after.terraform.plan]
    kind: infracost

  # ── Named tool kind with one override ─────────────────────────────────
  security:
    events: [after.terraform.plan]
    kind: trivy
    on_failure: fail         # trivy default is 'warn'

  # ── Generic engine, fully user-defined ────────────────────────────────
  custom-scan:
    events: [after.terraform.apply]
    kind: command
    command: my-internal-scanner          # binary, resolved via toolchain
    args:
      - "--component"
      - "{{ .atmos_component }}"          # template-rendered before exec
      - "--source"
      - "$ATMOS_COMPONENT_PATH"           # env-substituted at exec
      - "--out"
      - "$ATMOS_OUTPUT_FILE"
    env:
      SCANNER_PROFILE: "{{ .settings.security.profile }}"
    format: markdown                      # optional; v1 supports only 'markdown'
    on_failure: warn
```

### Go data model

```go
// pkg/hooks/hook.go

type Hook struct {
    // Dispatch.
    Kind   string   `yaml:"kind"`              // primary discriminator
    Events []string `yaml:"events,omitempty"`  // event filter; empty = all

    // Generic command engine (kind: command and named tool kinds).
    Command string            `yaml:"command,omitempty"` // binary (toolchain-resolved)
    Args    []string          `yaml:"args,omitempty"`    // templated + env-substituted
    Env     map[string]string `yaml:"env,omitempty"`     // extra env (templated)
    Format  string            `yaml:"format,omitempty"`  // v1: "markdown" or empty

    // Failure semantics. Empty = inherit kind default.
    OnFailure string `yaml:"on_failure,omitempty"` // "warn" | "fail" | "ignore"

    // Store kind specific (existing, unchanged).
    Name    string            `yaml:"name,omitempty"`
    Outputs map[string]string `yaml:"outputs,omitempty"`
}

// Kind is a registered hook type. Built-ins self-register from
// pkg/hooks/kinds/*/kind.go via init().
type Kind struct {
    Name          string
    Command       string
    DefaultArgs   []string
    DefaultEnv    map[string]string
    OnFailure     string
    Engine        Engine
    ResultHandler ResultHandler
}

// Output is what one hook invocation produces.
type Output struct {
    Artifact *Artifact // raw tool output, ready to upload
    Summary  *Summary  // parsed/normalized envelope (optional)
}

type Artifact struct {
    Name     string            // filename inside the upload bundle
    Body     io.Reader         // streamed from ATMOS_OUTPUT_FILE
    Format   string            // "markdown" for kind: command; empty for named kinds
    Metadata map[string]string
}

type Summary struct {
    Kind   string         // built-in kind name; Pro selects renderer
    Status SummaryStatus  // success | warning | failure
    Title  string         // "2 HIGH, 5 MED" or "+$47.20/mo"
    Counts map[string]int // {"high": 2, "medium": 5} — optional
    Body   string         // markdown detail — optional
}
```

### Pro wire format

```go
// pkg/pro/dtos/summary.go

type SummaryUploadRequest struct {
    WorkflowRunID string         `json:"workflow_run_id"`
    Stack         string         `json:"stack"`
    Component     string         `json:"component"`
    Kind          string         `json:"kind"`
    Status        string         `json:"status"`
    Title         string         `json:"title"`
    Counts        map[string]int `json:"counts,omitempty"`
    Body          string         `json:"body,omitempty"`
}

// pkg/pro/dtos/artifact.go

type ArtifactUploadRequest struct {
    WorkflowRunID string            `json:"workflow_run_id"`
    Stack         string            `json:"stack"`
    Component     string            `json:"component"`
    Kind          string            `json:"kind"`
    ToolVersion   string            `json:"tool_version,omitempty"`
    Format        string            `json:"format,omitempty"` // for kind=command
    Name          string            `json:"name"`
    SHA256        string            `json:"sha256"`
    SizeBytes     int64             `json:"size_bytes"`
    Metadata      map[string]string `json:"metadata,omitempty"`
    // Body streamed separately via multipart / chunked upload.
}
```

### Resolution order

For each hook invocation:

1. **Kind lookup** — `hook.Kind` resolves to a registered `Kind`. Legacy `command: store` is translated to `kind: store` during YAML unmarshal.
2. **Defaults applied** — `Kind.Command`, `Kind.DefaultArgs`, `Kind.DefaultEnv`, `Kind.OnFailure` fill in any field the hook didn't set.
3. **Templates rendered** — Go template expansion against the standard Atmos template context (already done by `ExecuteDescribeComponent` for the hooks section).
4. **Engine runs** — `Kind.Engine.Run()` executes the binary with rendered args, exporting `ATMOS_*` env vars. Tool stdout/stderr stream through `pkg/io/` / `pkg/ui/`.
5. **Result handled** — `Kind.ResultHandler` reads `ATMOS_OUTPUT_FILE` and produces a `Summary`. The engine packages the file as an `Artifact`.
6. **Output routed** — If Pro is connected, summary + artifact upload via `pkg/ci/artifact/pro/`. Otherwise, artifact lands in the local backend; summary is rendered to the terminal via `ui.Markdown()` when `format: markdown`.
7. **Failure mode applied** — `on_failure: fail` propagates non-zero exit; `warn` logs and continues; `ignore` swallows.

## Backwards Compatibility

- Existing `command: store` hooks parse identically. The YAML unmarshaller folds `command:` into `Kind` when `Kind` is empty.
- Existing `store` semantics (parameter store writes via `outputs:` map) are unchanged.
- No existing hook field is removed or repurposed in a breaking way.
- `pkg/hooks/event.go` events (`before/after.terraform.{init,plan,apply,deploy}`) are unchanged.
- Deprecated `ci.*` commands in `Hooks.RunAll` continue to no-op as today.

## Open Questions

1. **Failure semantics defaults** per built-in kind. **Resolved** — `checkov`, `trivy`, `kics`, `infracost` all default to `warn` (never fail the plan/apply by default). User overrides per hook via `on_failure: fail` / `ignore`.
2. **Workflow-run identity on the Pro side** — does Pro mint a stable run ID at run start (clean UUID join key for all subsequent uploads), or does each upload re-discover the run by (repo, sha, component, stack) like `UploadInstanceStatus` does today? Former is cleaner long-term and a soft prerequisite for cross-run aggregation.
3. **Cross-run finding aggregation** (Phase 2) — server-side dedup/fingerprinting of SARIF turns Pro into the system of record for posture over time. High-value but doubles scope. Confirm phasing.
4. **Standalone scans** — should `atmos scan vpc -s prod` exist for ad-hoc / scheduled compliance, or only as a hook? Out of scope for v1; revisit after the hook surface is in.
5. **Planfile threading** — the engine exposes `$ATMOS_PLANFILE` as part of its env-var contract, but the value is currently always empty because `cmd/terraform/plan.go` doesn't stash the planfile path before invoking hooks. Tools that benefit from a planfile (infracost is the canonical case — it would otherwise re-run `terraform plan` internally; terraform-cost-estimation tools and policy engines like OPA/Sentinel similarly want the JSON plan) get cleaner integration once this is wired. Implementation: hook into the existing planfile manager in `pkg/ci/plugins/terraform/planfile/`, populate `ExecContext.Planfile` before calling `Hooks.RunAll`. Phase 2.

6. **Atmos curated registry for tool overrides** — `pkg/toolchain/registry/atmos/` already supports inline tool definitions in `atmos.yaml` with a priority higher than the default Aqua registry, so per-project overrides work today. KICS in particular needs this because the upstream Aqua entry uses `type: go_build` (which the Atmos installer doesn't support yet) and KICS's distribution shape needs the query library extracted alongside the binary. Two follow-up tracks: (a) ship a curated Atmos registry baked into the CLI for tools the broader registry doesn't model well (KICS, anything else with awkward distribution), and (b) extend the `pkg/toolchain/installer/` tool-type set to handle tarball-with-data-files installs so KICS, Atmos itself, OpenTofu, etc. can all use the same machinery. Phase 2.

## Implementation Plan

### Phase 1 — Schema + Engine (back-compat-safe)

1. Rename `Hook.Command` → `Hook.Kind` in `pkg/hooks/hook.go`. Add legacy YAML alias in the unmarshaller.
2. Add `Hook.Args`, `Hook.Env`, `Hook.OnFailure`, `Hook.Format` fields.
3. Introduce `pkg/hooks/kind.go`: `Kind` struct + `Engine` interface + `RegisterKind` + resolution pipeline.
4. Refactor existing `store` dispatch in `pkg/hooks/hooks.go` to register as a `Kind` via the new mechanism.
5. Update JSON Schema in `pkg/datafetcher/schema/` for the new hook fields.
6. Add unit tests covering: kind lookup, legacy alias, default resolution, template/env interpolation.

### Phase 2 — Generic `command` kind + `--skip-hooks`

1. Add `pkg/hooks/command_kind.go` implementing the `Engine` interface: toolchain-resolved binary execution with `ATMOS_*` env injection, stdout/stderr streaming via `pkg/io/`/`pkg/ui/`, `OutputFile` capture, `on_failure` handling.
2. Markdown rendering of `format: markdown` output to the terminal via `ui.Markdown()`.
3. `--skip-hooks` flag (`pkg/flags/` registration) + `ATMOS_SKIP_HOOKS` env binding via `pkg/flags/NewStandardParser`. Skip logic applied in `cmd/terraform/utils.go::runHooks` above the kind dispatch.
4. Unit + integration tests in `tests/fixtures/scenarios/hooks-command-kind/`, including skip-flag coverage.

### Phase 3 — Named kinds

1. `pkg/hooks/kinds/infracost/kind.go` — defaults + JSON breakdown parser → `Summary`.
2. `pkg/hooks/kinds/checkov/kind.go` — defaults + SARIF parser → `Summary`.
3. `pkg/hooks/kinds/trivy/kind.go` — defaults + SARIF parser (shared with checkov).
4. `pkg/hooks/kinds/kics/kind.go` — defaults using `$ATMOS_OUTPUT_DIR` + SARIF parser (reads `results.sarif` from the output dir).
5. Blank import in `cmd/root.go` (or equivalent) to self-register kinds.
6. Per-kind integration tests + golden snapshots for summary rendering.

### Phase 4 — Pro backend

1. `pkg/pro/dtos/artifact.go`, `pkg/pro/dtos/summary.go` — wire DTOs (no media_type field).
2. `pkg/pro/api_client_artifacts.go` — `UploadArtifact` (chunked, reusing patterns from `chunked_upload.go`) + `UploadSummary`.
3. `pkg/ci/artifact/pro/backend.go` — `Backend` implementation slotting into the existing registry.
4. Hook engine routes to Pro backend when Pro is connected; falls back to local backend otherwise.
5. Mocked Pro upload tests; end-to-end fixture exercising the full path with Pro disabled (local backend) and a stubbed Pro server.

### Phase 5 — Documentation

1. Docusaurus pages: hook kind reference, per-kind pages (infracost, checkov, trivy, kics, command), Pro upload behavior.
2. Update `pkg/datafetcher/schema/` JSON Schema with `format:` enum.
3. Examples under `examples/quick-start-advanced/` showing hook kinds in a realistic stack.
4. Blog post (PR labeled `minor`) announcing the feature.
5. Roadmap update in `website/src/data/roadmap.js`.

## Verification

End-to-end fixture under `tests/fixtures/scenarios/custom-hooks/`:

- Component with `dependencies.tools.infracost: 0.10.x` + `hooks.cost: { kind: infracost }`.
- `atmos terraform plan vpc -s test` triggers `after.terraform.plan`.
- `command` engine resolves `infracost` via toolchain, runs it with `ATMOS_*` env, streams stdout to the user terminal.
- JSON written to temp `ATMOS_OUTPUT_FILE`.
- Pro disabled → artifact lands in local backend; summary rendered to terminal via `ui.Markdown()`.
- Pro enabled (stubbed server) → artifact + summary uploaded.

Assertions:

- Local-backend artifact bundle contains the breakdown JSON.
- Summary envelope has correct `kind`, `status`, `title`, `counts`.
- Exit code respects `on_failure: warn`.
- Repeat with `kind: store` to confirm legacy semantics intact under the field rename (regression).
- Repeat with `kind: command` + `format: markdown` to confirm terminal markdown rendering and Pro upload.

## Related Work

- [Hooks Component Scoping](./hooks-component-scoping.md) — inheritance and scoping rules apply unchanged.
- [Tool Dependencies Integration](./tool-dependencies-integration.md) — `dependencies.tools` provides version-pinned binaries this PRD relies on.
- [Native CI Integration](./native-ci-integration.md) — same philosophy: Atmos works identically locally and in CI; no wrapper scripts.
- [CI Summary Templates](./ci-summary-templates.md) — summary envelope format borrows from CI plugin templates.
- [Native CI Artifact Storage](./native-ci-artifact-storage.md) — `pkg/ci/artifact/` is the upload abstraction the Pro backend extends.
