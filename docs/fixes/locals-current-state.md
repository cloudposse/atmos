# Locals in Atmos — Current State (Full Picture)

**Status:** Implemented and merged into `main` (branch `aknysh/fix-locals-9` carries no unique commits vs `main`).
**Last reviewed:** 2026-06-13

This document is a consolidated summary of how the `locals` feature works in Atmos today: its
scope rules, support for YAML functions and Go templates, inheritance behavior, the
`describe locals` command, the processing pipeline, and the history of how the feature reached
its current state. It is assembled from the Go implementation, website docs, PRDs, prior fix
documents, blog posts, and the test suite.

> **Open work:** A class of `locals` bugs around `name_template`-based stack-name derivation is
> **not yet fixed in this tree**. They are addressed by the still-open **PR #2387**
> (branch `aknysh/fix-locals-8`). See **§12 (PR #2387)** and **§13 (What's missing)**.

---

## 1. What `locals` Are

`locals` are **file-scoped temporary variables** defined inside Atmos stack manifests, analogous
to Terraform/Terragrunt `locals`. They exist to reduce repetition (DRY), build derived values by
composition, and keep computed values close to where they are used — without leaking into
Terraform/Helmfile inputs or component descriptions.

Three things make `locals` distinct from `vars`/`settings`:

| Feature                               |          `locals`          |   `vars`    |     `settings`     |
|---------------------------------------|:--------------------------:|:-----------:|:------------------:|
| Inherited across file imports         |            ❌ No            |    ✅ Yes    |       ✅ Yes        |
| Passed to Terraform/Helmfile          |            ❌ No            |    ✅ Yes    |        ❌ No        |
| Visible in `atmos describe component` |            ❌ No            |    ✅ Yes    |       ✅ Yes        |
| Available in templates (same file)    |           ✅ Yes            |    ✅ Yes    |       ✅ Yes        |
| Purpose                               | File-scoped temp variables | Tool inputs | Component metadata |

Primary docs:

- `website/docs/stacks/locals.mdx` — reference
- `website/docs/design-patterns/configuration-composition/locals.mdx` — patterns
- `website/docs/cli/commands/describe/describe-locals.mdx` — command
- `docs/prd/file-scoped-locals.md` — PRD
- `docs/fixes/file-scoped-locals-not-working.md`,
  `docs/fixes/2026-03-15-locals-terraform-state-missing-stack-context.md` — prior fixes

---

## 2. Scope Rules

### 2.1 File-scoped (the central design decision)

Locals are **strictly file-scoped**: they do **not** flow across file `import` boundaries.
A file's locals are isolated to that file; an importing file cannot reference a mixin/imported
file's locals, and vice versa. This is enforced in code by cloning the template context per file
and dropping any inherited `locals` key before processing each imported file:

- `extractAndAddLocalsToContext()` — `internal/exec/stack_processor_utils.go` (~line 357–501);
  the per-file clone skips `cfg.LocalsSectionName` so inherited locals are not carried in.

Rationale (from PRD + fix docs): predictability, no hidden cross-file dependencies, and safer
refactoring. For cross-file sharing, use `vars` or `settings` instead.

### 2.2 Within-file cascade (the levels that DO compose)

Within a single file, locals cascade across three levels, each inheriting from the one above:

1. **Global** — root-level `locals:` — available to everything in the file.
2. **Component-type** — `terraform.locals:`, `helmfile.locals:`, `packer.locals:` — inherit the
   global locals; may override them for that section.
3. **Component-level** — `components.<type>.<name>.locals:` — inherit global + section locals;
   **and** inherit from base components via `metadata.inherits` (see §5).

Resolution order for conflicts: **Global → Section → Base Component → Component** (later wins).

Key implementation:

- `ProcessStackLocals()` — `internal/exec/stack_processor_locals.go` (~line 134–207): builds a
  `LocalsContext` with `Global`, `Terraform`, `Helmfile`, `Packer` maps plus
  `HasTerraformLocals` / `HasHelmfileLocals` / `HasPackerLocals` flags so an empty section does
  not clobber the global/terraform values.
- `ResolveComponentLocals()` — same file (~line 380–391): resolves component-level locals,
  inheriting stack-level locals.

### 2.3 What locals can reference

From templates inside locals (same file only):

- `{{ .locals.<name> }}` — other locals (with automatic dependency ordering, §4)
- `{{ .settings.<key> }}`, `{{ .vars.<key> }}`, `{{ .env.<KEY> }}` — sibling sections in the same file

Locals **cannot** reach `settings`/`vars`/`env` from imported files — same file-scoped rule.

---

## 3. YAML Functions in Locals

Locals support the full Atmos YAML function set: `!env`, `!exec`, `!store` / `!store.get`,
`!terraform.state`, `!terraform.output`, `!template`, etc.

- **Ordering:** YAML functions are resolved **before** Go templates. So you can fetch a value with
  `!env`/`!terraform.state`/`!store` and then consume it in a `{{ .locals.x }}` expression in
  another local.
- **Mechanism:** `createYamlFunctionProcessor()` — `internal/exec/stack_processor_locals.go`
  (~line 112–128) wraps the raw `!`-prefixed string and delegates to `ProcessCustomYamlTags()`.
  The resolver (`pkg/locals/resolver.go`) invokes this processor when a scalar starts with `!`,
  before any template pass. Errors are wrapped as `ErrLocalsYamlFunctionFailed`.
- **Stack context for 2-arg forms:** The 2-argument form (e.g. `!terraform.state vpc .vpc_id`,
  which implicitly uses the current stack) requires the stack name. Because locals are resolved
  early — before the stack context is fully formed — the stack name is **derived from the file
  path** via `deriveStackNameForLocals()` / `computeStackFileName()`
  (`internal/exec/stack_processor_utils.go`, ~line 205–228) using the same logic as
  `describe locals`. This was the fix in
  `docs/fixes/2026-03-15-locals-terraform-state-missing-stack-context.md` (without it, the 2-arg
  form failed with "stack is required" in catalog files).
- **Caching:** YAML function results are memoized within a file so expensive operations
  (state/output/store calls) are not repeated.

Example:

```yaml
locals:
  api_endpoint: !env API_ENDPOINT
  api_url: "https://{{ .locals.api_endpoint }}/api/v1"
  vpc_id: !terraform.state vpc .vpc_id
  db_password: !store secrets/database .password
  connection_string: "postgresql://app:{{ .locals.db_password }}@db.example.com/mydb"
```

---

## 4. Go Templates in Locals

- **Full templating:** Go templates with Sprig, Gomplate, and Atmos template functions;
  conditionals, ranges, pipes; recursive resolution into nested maps and slices.
- **Dependency resolution:** Locals may reference other locals in any order. The resolver builds
  a dependency graph from `{{ .locals.X }}` references (`buildDependencyGraph()`), does a
  topological sort (Kahn's algorithm, `topologicalSort()`), and resolves dependencies-first.
- **Cycle detection:** Circular references are detected (DFS) and reported with the full chain,
  e.g. `a → b → c → a`. (Fixture: `tests/fixtures/scenarios/locals-circular/`.)
- **Missing keys:** Template execution uses `missingkey=error`; error messages list the available
  locals/context to aid debugging. Where `ignore_missing_template_values` is honored, unresolved
  references become `<no value>` instead of hard errors.
- **Performance:** A quick `strings.Contains(strVal, "{{")` short-circuit skips template
  compilation for plain scalars.

Resolver lives in `pkg/locals/resolver.go` (struct `Resolver` with `locals`, `resolved`,
`dependencies`, `filePath`, `templateContext`, `yamlFunctionProcessor`).

### Combined ordering (the important bit)

For each file:

1. Raw YAML parsed; `locals:` sections extracted **before** template processing
   (`extractLocalsFromRawYAML()`, `internal/exec/stack_processor_utils.go` ~line 143–203).
2. Locals resolved: dependency graph → topological sort → **YAML functions first, then Go
   templates**, in dependency order.
3. Resolved locals added to the template context.
4. `settings`, then `vars`, then `env` are processed with locals (and previously-processed
   sections) available in context.

So the practical guarantee is: **YAML functions → locals (templated) → settings → vars → env.**

---

## 5. Inheritance, Imports, and Overrides

- **Imports:** Locals do **not** cross imports (file-scoped, §2.1). Locals are processed after
  imports are resolved, which keeps the import system simple and avoids hidden dependencies.
- **`metadata.inherits` (component-level):** Unlike file-scoped locals, **component-level** locals
  **do** inherit from base components — `BaseComponentLocals` is merged with `ComponentLocals`
  (base → component, component wins) in `mergeComponentConfigurations()`
  (`internal/exec/stack_processor_merge.go` ~line 317–332). This mirrors how `vars` inherit.
- **Overrides:** The `overrides:` section does **not** support a `locals:` key. Locals are merged
  only at the base-component level, deliberately keeping the merge order simple.

Example (component-level inheritance):

```yaml
components:
  terraform:
    vpc/base:
      metadata: { type: abstract }
      locals: { vpc_type: "standard", cidr_prefix: "10.0" }
    vpc/prod:
      metadata: { inherits: [ vpc/base ] }
      locals: { vpc_type: "production" }          # overrides base
      vars: { cidr: "{{ .locals.cidr_prefix }}.0.0/16" }  # cidr_prefix inherited from base
```

---

## 6. The `describe locals` Command

```bash
atmos describe locals [component] -s <stack> [--format yaml|json] [--file <path>] [--query <yq>]
```

- **Implementation:** `cmd/describe_locals.go` (+ `cmd/describe_locals_test.go`).
- **`-s/--stack` required.** Accepts either a manifest filename/path (e.g. `deploy/dev`) or a
  logical stack name (e.g. `prod-us-east-1`). Stack-name derivation reuses `describe stacks`
  logic: explicit name → name template → name pattern → filename.
- **Reads raw YAML from disk** (not fully-processed stacks) and calls `ProcessStackLocals()` to
  resolve. Per-file parse errors are reported; other files are skipped gracefully.
- **Without a component:** returns the locals defined in that manifest (file-scoped, not import
  inherited), in valid stack-manifest schema:
  ```yaml
  locals:
    environment: dev
    namespace: acme
  terraform:
    locals:
      backend_bucket: acme-dev-tfstate
  ```
- **With a component:** merges global + the component-type section locals + component-level locals
  (including inherited base-component locals) and emits Atmos schema form:
  ```yaml
  components:
    terraform:
      vpc:
        locals:
          environment: dev
          namespace: acme
  ```
- This command is the primary debugging tool for inspecting resolved locals — locals are otherwise
  invisible in `describe component`.

---

## 7. Processing Pipeline (Code Map)

| # | Stage            | Function                          | File                                                                       |
|---|------------------|-----------------------------------|----------------------------------------------------------------------------|
| 1 | Parse YAML       | `parseStackFileYAML()`            | `cmd/describe_locals.go` / `internal/exec`                                 |
| 2 | Extract locals   | `extractLocalsFromRawYAML()`      | `internal/exec/stack_processor_utils.go` (~143)                            |
| 3 | Resolve locals   | `ProcessStackLocals()` / resolver | `internal/exec/stack_processor_locals.go` (~134); `pkg/locals/resolver.go` |
| 4 | Add to context   | `extractAndAddLocalsToContext()`  | `internal/exec/stack_processor_utils.go` (~357)                            |
| 5 | Extract sections | `extractComponentSections()`      | `internal/exec/stack_processor_process_stacks_helpers_extraction.go`       |
| 6 | Merge locals     | `mergeComponentConfigurations()`  | `internal/exec/stack_processor_merge.go` (~317)                            |
| 7 | Output           | `buildComponentSchemaOutput()`    | `cmd/describe_locals.go`                                                   |

**Key types/constants:**

- `LocalsSectionName = "locals"` — `pkg/config/const.go`.
- `LocalsContext{ Global, Terraform, Helmfile, Packer, HasTerraformLocals, HasHelmfileLocals, HasPackerLocals }` —
  `internal/exec/stack_processor_locals.go`.
- `Resolver{ locals, resolved, dependencies, filePath, templateContext, yamlFunctionProcessor }` —
  `pkg/locals/resolver.go`.

**Caching/perf:**

- `localsExtractionCache` (`sync.Map`, keyed by `filePath + "@" + FNV-1a(content)`) memoizes
  parse+resolve, deep-copying on retrieval; cleared via `ClearLocalsExtractionCache()`. Eliminated
  ~7s of repeated `UnmarshalYAML` on a customer workload.
- Asymmetric clone in `cloneExtractLocalsResult` reduces allocations on large stacks; a data race
  in `extractAndAddLocalsToContext` was fixed.

**Schema note:** Locals are represented as `map[string]any` (no dedicated JSON Schema file under
`pkg/datafetcher/schema/`); validated by type assertion during extraction.

---

## 8. History / Evolution

**Phase 1 — Initial feature (Dec 2025).** Commits `ee65bef53`/`38407357a` (#1883). File-scoped
locals; global + section + component levels; dependency resolution + cycle detection; resolved
before template processing; not passed to Terraform/Helmfile; not in output. Blog:
`website/blog/2025-12-16-file-scoped-locals.mdx`.

**Phase 2 — Template-resolution fix (Jan 6–7, 2026).** Issue #1933: `{{ .locals.* }}` were not
actually being resolved (code existed but wasn't wired into the pipeline). Fix added
`extractLocalsFromRawYAML()` before template processing, added `.locals` to template context, and
introduced section-override tracking flags. The **`atmos describe locals`** command was added here.
Blog: `website/blog/2026-01-06-file-scoped-locals-fix.mdx`. Fix doc:
`docs/fixes/file-scoped-locals-not-working.md`.

**Phase 3 — Context access + YAML functions (Jan 19–20, 2026).** PR #1994 / commit `2f95f4a40`.
Locals can read same-file `settings`/`vars`/`env` (#1991); full Atmos YAML functions supported in
locals (`!env`, `!exec`, `!store`, `!terraform.state`, `!terraform.output`). Blogs:
`website/blog/2026-01-19-locals-context-access-and-design-patterns.mdx`,
`website/blog/2026-01-20-locals-yaml-functions.mdx`.

**Phase 3b — Stack context for `!terraform.state` (Mar 15, 2026).** Issue #2207 / #2293-adjacent.
2-arg `!terraform.state` in locals failed with "stack is required" because the stack was empty
during early processing. Fixed by deriving the stack name from the file path before locals
processing. Fix doc: `docs/fixes/2026-03-15-locals-terraform-state-missing-stack-context.md`.

**Phase 4 — Perf & robustness.** `name_template` now merges imported vars/settings before
templating; YAML-function memoization; honor `ignore_missing_template_values`; locals-extraction
caching; asymmetric clone; race fix; coverage lifted >80%.

**Phase 5 — Docs/patterns.** Reference + design-pattern docs, `examples/locals/`, doc condensation
(#2000).

Branch lineage `aknysh/fix-locals-1` … `aknysh/fix-locals-9` (+ `aknysh/improve-locals-5`,
`osterman/locals-prd`) reflects the iterative refinement; all of it is now in `main`.

---

## 9. Test Coverage

Driver: `tests/cli_locals_test.go` and `cmd/describe_locals_test.go`. Fixtures under
`tests/fixtures/scenarios/`:

| Fixture                      | Purpose                                                 |
|------------------------------|---------------------------------------------------------|
| `locals`                     | Basic global + section scopes                           |
| `locals-advanced`            | Nested settings/vars access, Sprig functions            |
| `locals-circular`            | Circular dependency detection                           |
| `locals-component-level`     | Component-level locals + `metadata.inherits`            |
| `locals-conditional`         | Go template conditionals with YAML functions            |
| `locals-deep-import-chain`   | File-scoped isolation across a multi-level import chain |
| `locals-env-test`            | Env var access                                          |
| `locals-file-scoped`         | File-scoped isolation                                   |
| `locals-logical-names`       | Logical stack-name resolution                           |
| `locals-not-inherited`       | Mixin locals NOT inherited                              |
| `locals-settings-access`     | Same-file settings access                               |
| `locals-settings-cross-file` | Cross-file settings access fails (by design)            |
| `locals-yaml-functions`      | YAML function support                                   |

Representative tests: `TestLocalsResolutionDev/Prod`, `TestDescribeLocals`,
`TestLocalsSettingsAccessSameFile`, `TestLocalsSettingsAccessNotCrossFile`,
`TestLocalsWithYamlFunctionsEnv`, `TestLocalsGoTemplateConditionalWith{Env,EnvEmpty}`,
`TestLocalsNestedSettingsAccess`, `TestComponentLevelLocals`, `TestLocalsDeepImportChain`,
`TestLocalsCircularDependency`.

---

## 10. Known Limitations & Caveats

- **No cross-file locals** (by design). Use `vars`/`settings` to share across files.
- **Not in `describe component` output** and **not passed to Terraform/Helmfile** (by design;
  use `describe locals` to inspect).
- **Template processing is enabled when a file defines locals.** Files containing non-Atmos
  `{{ }}` syntax (e.g. Helm values) should set `skip_templates_processing: true` on the import.
- **`locals` template-context key is reserved.** A pre-existing `locals` entry passed via an
  import `context:` parameter will be overridden by the resolved locals.
- **No locals in `atmos.yaml`** — stack manifests only.
- **No lazy evaluation** — all locals are resolved upfront, once per file.

---

## 11. Quick Reference Example

```yaml
# stacks/deploy/prod.yaml  (single file — locals here do not leak to imports)

settings:
  region: { primary: us-east-1 }

vars:
  stage: prod

locals:
  namespace: acme
  environment: "{{ .vars.stage }}"                       # reads sibling vars
  region: "{{ .settings.region.primary }}"               # reads sibling settings
  prefix: "{{ .locals.namespace }}-{{ .locals.environment }}"  # local-on-local
  vpc_id: !terraform.state vpc .vpc_id                   # YAML fn, current stack derived from path
  default_tags:
    Namespace: "{{ .locals.namespace }}"
    ManagedBy: atmos

terraform:
  locals:
    backend_bucket: "{{ .locals.prefix }}-tfstate"       # inherits global locals

components:
  terraform:
    vpc:
      vars:
        name: "{{ .locals.prefix }}-vpc"
        tags: "{{ .locals.default_tags }}"
```

Inspect with:

```bash
atmos describe locals -s prod              # file-level locals
atmos describe locals vpc -s prod          # merged locals for the vpc component
```

---

## 12. PR #2387 — `name_template` × `locals` Interaction (OPEN, not yet merged)

**PR:** [cloudposse/atmos#2387](https://github.com/cloudposse/atmos/pull/2387) — branch
`aknysh/fix-locals-8`. **State as of this review: OPEN, not merged.** Its locals changes are
therefore **not present** in the current `aknysh/fix-locals-9` / `main` tree (verified by symbol
search — see §13).

The PR is the next locals chapter after the work in §1–§9. Earlier phases made locals *resolve*
and made YAML functions/templates work inside them; PR #2387 fixes a cluster of regressions where
**adding any `locals:` block to a stack file silently breaks stack-name derivation** when the
stack name comes from `name_template`. Title:
*"fix(locals): name_template merges imported vars/settings; memoize YAML functions; honor
ignore_missing_template_values."* It closes **four** issues, which surface as one symptom
("locals are broken") but stem from **three independent root causes** in `internal/exec/`.

### Issues closed

- **#2343** — `atmos list stacks` returns a malformed name (`-prod` instead of `acme-prod`) when
  `name_template` references a var (e.g. `namespace`) defined in a **parent `_defaults.yaml`** and
  the leaf file declares `locals:`. The real stack disappears from listings.
- **#2374** — same shape as #2343 but `name_template` references `.settings.*`.
  `{{ .locals.* }}` renders as `<no value>` in component vars; `describe locals -s <name>` returns
  `stack not found` while `describe component -s <name>` succeeds for the same stack.
- **#2344** — `!terraform.state` (and other YAML functions) inside `locals:` is re-evaluated **N×**
  per `atmos list stacks`. Reporter measured **1362** state-getter calls for **7** refs across a
  20-file tree (~195× per ref) on v1.216.0 — turning `list stacks` into a ~20s hang.
- **#2345** — twelve `ProcessTmpl(... NameTemplate ..., false)` call sites pass a hardcoded
  `false`, bypassing the global `templates.settings.ignore_missing_template_values: true` flag
  (added in PR #2158). Users who set the flag still hit `map has no entry for key "<x>"` errors.

### Three root causes / fixes (per the PR)

| Cluster                      | Issues       | Defect                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      | Fix in PR #2387                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
|------------------------------|--------------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **A. Stack-name derivation** | #2343, #2374 | `name_template` is rendered against the **un-merged single leaf file**; the template data wraps only `vars` (never `settings`/`env`); imports are never walked, so a `namespace`/setting defined in a parent `_defaults.yaml` is invisible. Deeper bug: in `processYAMLConfigFileWithContextInternal` the stackConfigMap-overwrite block (~lines 864–872) was **unconditional**, so on recursion into an import the parent frame's `context.vars` (set by the leaf's locals path) silently clobbered the import's own vars. | `deriveStackNameFromTemplate` now passes **vars + settings + env** and renders with `ignoreMissingTemplateValues=true`; new **`deriveStackNameSections`** walks imports to build a YAML-only overlay; results containing `<no value>` are rejected so a missing key falls back cleanly to the filename. A new **`localsContextResult`** flag struct returned from `extractAndAddLocalsToContext` records which sections *this* file populated, and the overwrite is gated on those flags. |
| **B. Performance**           | #2344        | `extractLocalsFromRawYAML` re-runs the resolver and every YAML function on each pipeline visit; no memoization.                                                                                                                                                                                                                                                                                                                                                                                                             | New **`extractLocalsCache`** (`sync.Map` keyed by `absFilePath + sha256(yamlContent)`); `extractLocalsFromRawYAML` becomes a thin caching wrapper around renamed **`uncachedExtractLocalsFromRawYAML`**. Errors memoized too.                                                                                                                                                                                                                                                             |
| **C. Flag routing**          | #2345        | 12 `ProcessTmpl(... NameTemplate ..., false)` sites bypass the global flag.                                                                                                                                                                                                                                                                                                                                                                                                                                                 | Replace hardcoded `false` with `atmosConfig.Templates.Settings.IgnoreMissingTemplateValues` at every site.                                                                                                                                                                                                                                                                                                                                                                                |

A and C compound: A produces a malformed stack name → downstream component sections get incomplete
vars → C ignores the very flag the user set to suppress the resulting missing-key error.

### What the PR adds (artifacts)

- Code: `internal/exec/stack_processor_utils.go`, `describe_locals.go`, `stack_utils.go`,
  `utils.go`, `describe_stacks_component_processor.go`, plus the 12 `ProcessTmpl` call sites
  (`atlantis_generate_repo_config.go`, `aws_eks_update_kubeconfig.go`, `describe_affected_utils_2.go`,
  `spacelift_utils.go`, `terraform_generate_backends.go`, `terraform_generate_varfiles.go`,
  `validate_stacks.go`, …).
- Fixtures: `tests/fixtures/scenarios/locals-name-template-vars-imports/` and
  `…/locals-name-template-settings-imports/` (hierarchical `_defaults.yaml` layouts).
- Tests: 5 fixture-level `TestLocalsNameTemplate*` in `tests/cli_locals_test.go` (hit
  `describe stacks`, `describe component`, `describe locals`); 7 unit tests in `internal/exec/`
  for import-merge derivation, `<no value>` rejection, the `localsContextResult` flags, the
  memo cache + content-hash invalidation, and the `ignore_missing_template_values` routing.
- Analysis doc: `docs/fixes/2026-05-02-locals-name-template-perf-and-ignore-missing.md`.

(The PR also fixes an unrelated CI-blocking `pkg/ci/providers/github/base_test.go` regression — out
of scope for this locals review.)

---

## 13. What's Missing for `locals` in the Current Codebase

Gap analysis of the current `aknysh/fix-locals-9` / `main` tree against PR #2387. Verified by
symbol/fixture search on 2026-06-13.

| Area (issue)                                                               | In current tree?                              | Evidence                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                      |
|----------------------------------------------------------------------------|-----------------------------------------------|-------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| **Stack-name derivation walks imports** (#2343, #2374)                     | ❌ **Missing**                                 | `deriveStackNameSections` not defined anywhere in `internal/exec/`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                           |
| **`deriveStackNameFromTemplate` passes settings/env** (#2374)              | ❌ **Missing**                                 | Current impl wraps **only vars**: `templateData := map[string]any{"vars": varsSection}` and passes `false` to `ProcessTmpl` (`internal/exec/describe_locals.go:~500`). No `settings`/`env`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                   |
| **`localsContextResult` overwrite-gating** (#2343/#2374 deeper bug)        | ❌ **Missing**                                 | No `localsContextResult` type; `extractAndAddLocalsToContext` returns `(context, error)` only. The unconditional stackConfigMap-overwrite path is unguarded.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| **`ignore_missing_template_values` honored at NameTemplate sites** (#2345) | ❌ **Missing**                                 | All **12** `ProcessTmpl(... NameTemplate ..., false)` call sites still pass hardcoded `false` (atlantis, eks, describe_affected ×2, describe_locals, describe_stacks_component_processor, spacelift, stack_utils, terraform_generate_backends/varfiles, utils, validate_stacks).                                                                                                                                                                                                                                                                                                                                                                                              |
| **`TestLocalsNameTemplate*` tests + 2 fixtures**                           | ❌ **Missing**                                 | `grep TestLocalsNameTemplate tests/cli_locals_test.go` → 0; fixtures `locals-name-template-vars-imports` / `…-settings-imports` absent.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |
| **Analysis doc `2026-05-02-...md`**                                        | ❌ **Missing**                                 | Not in `docs/fixes/`.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                         |
| **Per-file memoization of locals/YAML-function resolution** (#2344)        | ✅ **Present — but via a different mechanism** | The tree already has **`localsExtractionCache`** (a `sync.Map` keyed by `filePath + "@" + FNV-1a(yamlContent)`, with `cloneExtractLocalsResult` deep-copy on read and `ClearLocalsExtractionCache()`), introduced by the **describe-affected perf** work (PR #2496, commit `295d2a56d`), *not* by PR #2387. It uses **FNV-1a** rather than PR #2387's **sha256**, and the function is *not* split into `uncachedExtractLocalsFromRawYAML`. Functionally it memoizes parse + locals resolution per (file, content), which addresses #2344's N× re-evaluation symptom — but it is a parallel implementation, so PR #2387's `extractLocalsCache` is still technically "missing." |

### Net effect on the current tree (observable gaps)

The following are **still broken / unaddressed** in `aknysh/fix-locals-9` today:

1. **`name_template` + `locals` + parent `_defaults.yaml` (vars)** — #2343. If `name_template`
   references a var defined in an imported parent file and the leaf declares `locals:`, the derived
   stack name is malformed (e.g. `-prod`) and the stack can drop out of `list stacks`. Current
   `deriveStackNameFromTemplate` only sees the leaf file's own raw `vars` and does not walk imports.
2. **`name_template` referencing `.settings.*`** — #2374. Current derivation never wraps
   `settings`/`env` into the template data, so settings-based name templates can't resolve and
   `describe locals -s <derived-name>` can return `stack not found`.
3. **`ignore_missing_template_values` ignored for stack-name templates** — #2345. Setting the
   global flag does not suppress `map has no entry for key …` from the 12 hardcoded-`false`
   `ProcessTmpl` NameTemplate sites.
4. **The recursion-frame `vars` clobber** in `processYAMLConfigFileWithContextInternal` (the
   `localsContextResult` gating) is not in place, so the leaf-locals path can still overwrite an
   import's vars during recursion.

Largely (but not provably fully) mitigated already:

5. **#2344 perf hang** — the independent `localsExtractionCache` (PR #2496) memoizes locals
   extraction per (file, content) and should prevent the N× YAML-function re-evaluation that caused
   the ~20s `list stacks` hang. This is plausibly resolved in the current tree, though via a
   different cache than PR #2387 proposed; it has not been re-measured against #2344's reproduction.

### Recommended next step

To close the remaining locals gaps in this tree, land PR #2387's **A** (import-walking +
settings/env in `deriveStackNameFromTemplate` + `deriveStackNameSections` + `localsContextResult`
gating) and **C** (the 12 `ignore_missing_template_values` routing fixes), plus its
`TestLocalsNameTemplate*` tests and two fixtures. Reconcile **B** with the already-merged
`localsExtractionCache`: prefer the existing cache (or unify on one hash) rather than introducing a
second parallel `extractLocalsCache`, and re-validate against #2344's reproduction.
