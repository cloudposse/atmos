# Fix: locals — name_template rendered against the un-merged file (#2343, #2374), N² re-evaluation during `list stacks` (#2344), and ignored `ignore_missing_template_values` (#2345)

**Date:** 2026-05-02

**Branch:** `aknysh/fix-locals-8`

**Reported issues — all four covered by this fix:**

- [#2343 — locals breaks stack listing when name_template vars come from parent imports](https://github.com/cloudposse/atmos/issues/2343).
  `atmos list stacks` returns a malformed stack name (`-prod`
  instead of `acme-prod`) whenever a leaf stack file defines
  `locals:` and `name_template` references `vars` (e.g.
  `namespace`) defined in a parent `_defaults.yaml`. The real
  stack disappears from listings.
- [#2344 — `!terraform.state` in locals is re-evaluated N times per `list stacks`](https://github.com/cloudposse/atmos/issues/2344).
  YAML functions inside `locals:` are re-evaluated on every
  pipeline visit. The reporter measured **1362** state-getter
  calls for **7** refs across a 20-file tree (≈ 195× per ref),
  turning `list stacks` into a 20-second hang even when every
  call fails fast.
- [#2345 — `describe-stacks-name-template` ignores global `ignore_missing_template_values`](https://github.com/cloudposse/atmos/issues/2345).
  Twelve `ProcessTmpl(... NameTemplate ..., false)` call sites
  pass a hardcoded `false` for the `ignoreMissingTemplateValues`
  argument, bypassing the global
  `templates.settings.ignore_missing_template_values: true` flag
  introduced in PR #2158. Users who set the flag still hit
  `map has no entry for key "<x>"` errors.
- [#2374 — locals seems to be broken](https://github.com/cloudposse/atmos/issues/2374).
  `{{ .locals.* }}` renders as `<no value>` in component `vars`
  whenever the project uses `name_template: "{{ .settings.* }}"`
  and `settings` are inherited from `_defaults.yaml`. As a
  side-effect, `atmos describe locals <c> -s <stack>` errors with
  `stack not found` while `atmos describe component <c> -s <stack>`
  succeeds on the same stack — same root cause as #2343, but the
  selector is `.settings.*` instead of `.vars.*`.

**Issue → Cluster map (see "Problem analysis" below for details):**

| Issue   | Cluster                              | Leaf bug                                                                                       |
|---------|--------------------------------------|------------------------------------------------------------------------------------------------|
| #2343   | A. Stack-name derivation             | Imports not merged before rendering `name_template` in the locals pre-pass                     |
| #2374   | A. Stack-name derivation             | Same as #2343 + `templateData` only carries `vars`, never `settings`                           |
| #2344   | B. Performance                       | `extractLocalsFromRawYAML` not memoized; YAML functions re-run on every pipeline visit         |
| #2345   | C. Flag routing                      | Twelve `ProcessTmpl(... NameTemplate ..., false)` sites bypass `ignore_missing_template_values` |

**Affected versions:** v1.200.0 (file-scoped locals landed) through
v1.216.0.

**Severity:** High — `locals:` becomes effectively unusable in any
project that follows the documented hierarchical `_defaults.yaml`
layout. The performance regression (#2344) makes `list stacks`
appear to hang.

---

## Status

**Fixed.** All four issues addressed on `aknysh/fix-locals-8`:

- 5 new fixture-level integration tests (`tests/cli_locals_test.go`)
- 5 new unit tests (`internal/exec/describe_locals_test.go`,
  `internal/exec/describe_stacks_component_processor_test.go`,
  `internal/exec/stack_processor_utils_test.go`)
- 2 new fixtures
  (`tests/fixtures/scenarios/locals-name-template-vars-imports/`,
  `tests/fixtures/scenarios/locals-name-template-settings-imports/`)
  exercising the hierarchical `_defaults.yaml` layout that previously
  broke. Each was verified to fail on `main` and pass after the fix.

### Progress checklist

- [x] Reproduce: added fixtures
      `tests/fixtures/scenarios/locals-name-template-vars-imports/`
      (vars flavor, #2343) and
      `tests/fixtures/scenarios/locals-name-template-settings-imports/`
      (settings flavor, #2374), each with a leaf stack file that
      defines `locals:` and a parent `_defaults.yaml` carrying the
      identifying `vars`/`settings`. Each fixture failed on `main`
      and passes after the fix.
- [x] Reproduce: added 5 fixture-level tests in
      `tests/cli_locals_test.go` (`TestLocalsNameTemplate*`) that
      hit `describe stacks`, `describe component`, and
      `describe locals` against both fixtures. Verified all five
      fail on `main` (`map has no entry for key "namespace"`,
      "Could not find component", "stack not found") and pass after
      the fix.
- [x] Reproduce #2344: added two unit tests in
      `internal/exec/stack_processor_utils_test.go`
      (`TestExtractLocalsFromRawYAML_Memoizes`,
      `TestExtractLocalsFromRawYAML_MemoizeInvalidatesOnContentChange`)
      that inject a counting `stateGetter` and assert
      `EXPECT().Times(1)`. The first failed on `main` ("expected
      call ... has already been called the max number of times")
      and passes after the cache lands.
- [x] Reproduce #2345: added
      `TestResolveStackName_NameTemplate_HonorsIgnoreMissingTemplateValues`
      in `internal/exec/describe_stacks_component_processor_test.go`.
      The `flag=true` subtest failed on `main` (`map has no entry
      for key "namespace"`); both pass after the fix.
- [x] Apply Cluster A fix:
      `deriveStackNameFromTemplate` (`describe_locals.go`) now
      passes `settings` and `env` alongside `vars` in `templateData`,
      renders with `ignoreMissingTemplateValues=true`, and rejects
      any rendered name containing `<no value>`. Added
      `deriveStackNameSections` to walk imports for a lite
      YAML-only `vars`/`settings`/`env` overlay used solely for
      stack-name derivation.
- [x] Apply Cluster A deeper fix:
      `extractAndAddLocalsToContext` (`stack_processor_utils.go`)
      now returns a `localsContextResult` flag struct identifying
      which sections (`vars`/`settings`/`env`/`locals`) the current
      file actually populated on `context`. The downstream
      stackConfigMap-overwrite block is gated on those flags, so
      the parent recursion frame's `context.vars` no longer
      clobbers an imported file's vars (the actual root cause of
      #2343 and #2374 in the live pipeline).
- [x] Apply Cluster B fix: added `extractLocalsCache` (`sync.Map`
      keyed by `absFilePath || \x00 || sha256(yamlContent)`) with
      a thin caching wrapper `extractLocalsFromRawYAML` around the
      renamed `uncachedExtractLocalsFromRawYAML`. Errors are
      memoized too.
- [x] Apply Cluster C fix: replaced the hardcoded `false` with
      `atmosConfig.Templates.Settings.IgnoreMissingTemplateValues`
      at all 12 `ProcessTmpl(... NameTemplate ..., false)` sites
      (full list in "Cluster C" section).
- [x] Confirm: each cluster's tests verified to fail on `main` and
      pass on this branch.
- [x] Regression:
      `go test ./internal/exec/... ./pkg/locals/... ./tests/... -count=1`
      green; existing locals integration tests unchanged.

---

## Problem analysis: three independent root causes

Although all four issues surface the same way ("locals don't
work"), they stem from three independent defects. Two of them
share `deriveStackNameForLocals` / `deriveStackNameFromTemplate`
as their leaf bug; the third is a leaf perf bug; the fourth is a
flag-routing bug that compounds the first.

| Cluster                     | Issues          | Defect                                                                                                              | Component                                          |
|-----------------------------|-----------------|---------------------------------------------------------------------------------------------------------------------|----------------------------------------------------|
| **A. Stack-name derivation** | #2343 + #2374  | `name_template` rendered against an un-merged single file, `templateData` only contains `vars` (not `settings`)     | `internal/exec/describe_locals.go`                 |
| **B. Performance**          | #2344           | `extractLocalsFromRawYAML` re-runs the whole resolver (and every YAML function inside it) on every pipeline visit   | `internal/exec/stack_processor_utils.go`           |
| **C. Flag routing**         | #2345           | Eleven `ProcessTmpl(... NameTemplate ..., false)` sites ignore `templates.settings.ignore_missing_template_values`  | `internal/exec/*.go`                               |

The reason all four reports look like "locals are broken" is that
Cluster A produces a **silently malformed stack name** (e.g. `-prod`
or `<no value>-key-logs`) which then:

1. Causes the stack to disappear from `atmos list stacks` (the user
   can't find it by name).
2. Causes `atmos describe locals <c> -s <real-name>` to return
   `stack not found` (the lookup index was built with the malformed
   name).
3. Leaves `{{ .locals.* }}` references in component `vars`
   unresolved, because the locals pipeline bails out for the file
   whose stack name didn't validate.
4. Hits the un-suppressed `ProcessTmpl(... NameTemplate ..., false)`
   path (Cluster C), which then surfaces `map has no entry for key
   "namespace"` instead of being silenced by the user's
   `ignore_missing_template_values: true` setting.

Fixing only one Cluster mitigates the symptom; fixing all three is
the clean solution.

---

## Cluster A: stack-name derived from un-merged file

### Issues

- [#2343](https://github.com/cloudposse/atmos/issues/2343) — `vars`-flavor.
- [#2374](https://github.com/cloudposse/atmos/issues/2374) — `settings`-flavor.

### Reproducers

Both reproducers use the documented hierarchical `_defaults.yaml`
layout. The only difference between them is whether the
`name_template` selects `.vars.*` or `.settings.*`.

#### #2343 — `vars`-flavor

```yaml
# atmos.yaml
stacks:
  base_path: stacks
  included_paths: ["orgs/**/*"]
  excluded_paths: ["**/_defaults.yaml"]
  name_template: "{{ .vars.namespace }}-{{ .vars.stage }}"
templates:
  settings:
    enabled: true
```

```yaml
# stacks/orgs/_defaults.yaml
vars:
  namespace: acme
```

```yaml
# stacks/orgs/prod.yaml
import: [./_defaults]
vars:
  stage: prod
locals:
  foo: bar              # any locals block triggers the pre-pass
components:
  terraform:
    vpc:
      metadata: { component: mock }
      vars: { name: "{{ .locals.foo }}" }
```

```text
$ atmos list stacks
expected: acme-prod
actual:   -prod          # namespace lost; stack now unfindable

$ atmos describe component vpc -s acme-prod
Error: invalid component … Could not find the component vpc in the stack acme-prod
```

Removing the `locals:` block restores the expected behavior — the
import-merging path that `describe stacks` uses *does* see
`namespace: acme`. The locals pre-pass is the only path that sees
the un-merged file.

#### #2374 — `settings`-flavor

```yaml
# atmos.yaml
stacks:
  name_template: "{{.settings.namespace}}-{{.settings.tenant}}-{{.settings.environment}}-{{.settings.stage}}"
```

```yaml
# stacks/orgs/.../_defaults.yaml
settings:
  namespace: cloudlabs
  tenant: plat
  environment: ue1
```

```yaml
# stacks/orgs/.../prod.yaml
import: [./_defaults]
settings:
  stage: prod
locals:
  prefix: "cloudlabs-plat-ue1-prod"
components:
  terraform:
    kms:
      vars:
        aliases:
          - "{{ .locals.prefix }}-key-logs"
```

```text
$ terraform plan kms ...
Error: "name" must begin with 'alias/' and only contain [...]
  with aws_kms_alias.this["<no value>-key-logs"], ...
                          ^^^^^^^^^^
$ atmos describe locals kms -s cloudlabs-plat-ue1-prod
Error: stack not found
```

`atmos describe component kms -s cloudlabs-plat-ue1-prod` works on
the same project (separate code path; that one merges imports
correctly).

### Code path

```go
// internal/exec/describe_locals.go:504-536
func deriveStackNameFromTemplate(
    atmosConfig *schema.AtmosConfiguration,
    stackFileName string,
    varsSection map[string]any,         // ← only vars from THIS file
) string {
    if atmosConfig.Stacks.NameTemplate == "" {
        return ""
    }

    // Wrap varsSection in "vars" key to match template syntax: {{ .vars.environment }}.
    templateData := map[string]any{
        "vars": varsSection,            // ← settings is missing entirely
    }

    stackName, err := ProcessTmpl(
        atmosConfig,
        "describe-locals-name-template",
        atmosConfig.Stacks.NameTemplate,
        templateData,
        false,                          // ← Cluster C bug; see below
    )
    if err != nil { ... return "" }

    // Only catches raw "{{" / "}}", not "<no value>" produced by
    // missing-key rendering with sprig's default behavior.
    if strings.Contains(stackName, "{{") || strings.Contains(stackName, "}}") {
        return ""                       // falls back to filename
    }
    return stackName                    // ← may be e.g. "-prod" or
                                        //   "<no value>-key-logs"
}
```

The same function is reached from two callers:

1. `processYAMLConfigFileWithContextInternal` (stack pipeline) →
   `extractLocalsFromRawYAML` →
   `deriveStackNameForLocals` (`internal/exec/stack_processor_utils.go:100-117`)
   → `deriveStackName` → `deriveStackNameFromTemplate`.
2. `cmd/describe_locals.go` → `DescribeLocalsExec` (`internal/exec/describe_locals.go:340-371`)
   → `deriveStackName` → `deriveStackNameFromTemplate`.

Both callers pass `varsSection` extracted from the **single
file's** `rawConfig` — no import merging. The `settings` section
is never even read.

#### Why imports aren't merged

The locals pre-pass runs *before* `processYAMLConfigFileWithContextInternal`
walks the import graph (the pre-pass exists precisely so locals
are resolved before templates in `vars`/`settings`/`env` are
processed — see the #1933 fix doc). At pre-pass time, the only
data available for the current file is what the YAML parser
returned from that one file.

The previous fix (#2080) computed the stack name purely as a
"hint" for `!terraform.state` 2-arg form. That fix was correct for
the case it was solving (the file *itself* had the identifying
vars in it), but it didn't anticipate hierarchical layouts where
identity lives in `_defaults.yaml`.

#### Why no test caught it

Every existing fixture under `tests/fixtures/scenarios/locals-*`
defines identity vars (`namespace`, `tenant`, `stage`, etc.)
**directly in the leaf file**. None of them inherit identity vars
from imports while also defining `locals:` in the leaf — exactly
the configuration that fails. The `locals-deep-import-chain`
fixture comes closest but its `name_template` doesn't reference
imported vars.

### Fix

Three changes, all in `describe_locals.go` and `stack_processor_utils.go`:

#### A1. Include `settings` (and other top-level sections) in `templateData`

```go
// internal/exec/describe_locals.go (deriveStackNameFromTemplate)
templateData := map[string]any{
    "vars":     varsSection,
    "settings": settingsSection,   // ← new
    "env":      envSection,        // ← new (parity with the rest of the pipeline)
}
```

This alone is enough to make `name_template: "{{ .settings.* }}"`
work for #2374 when `settings` is defined in the same file. It
does **not** fix the import case (#2343) on its own.

#### A2. Walk imports to merge `vars`/`settings`/`env` before deriving the name

Added `deriveStackNameSections` in `describe_locals.go` — a lite,
YAML-only DFS that resolves the `import:` list (using
`ProcessImportSection` + a focused
`resolveImportFilePathsForStackName` helper) and merges
`vars`/`settings`/`env` from this file plus all transitively-imported
files. No template processing, no YAML function resolution -- this
is *only* used to feed `name_template` during stack-name derivation.

The walk is best-effort: any unresolvable path is silently skipped
(falls back to the filename rather than failing). Cycles are broken
via a `visited` set keyed on absolute path.

#### A2.1. The deeper bug: parent context overwriting the import's stackConfigMap

While running the fixture-level tests, the surface fix above wasn't
enough -- `info.ComponentSection.vars` still came up missing
`namespace` in the live `describe stacks` pipeline. Tracing showed
the actual root cause is in
`processYAMLConfigFileWithContextInternal`
(`stack_processor_utils.go`). Lines that wrote
`stackConfigMap[VarsSectionName] = context[VarsSectionName]`
unconditionally overwrote the current file's stackConfigMap with
whatever was sitting in `context`. When the function recurses for
an import, `context` carries the **parent's** resolved
vars/settings/env (set by the leaf file's
`extractAndAddLocalsToContext`). So when `_defaults.yaml` is
processed recursively, its real `vars: { namespace: acme }` was
overwritten with the parent's `vars: { stage: prod }`. The
downstream deep-merge then propagated only `{stage: prod}` to
`globalVarsSection` in `ProcessStackConfig`.

The fix is the second part of Cluster A: `extractAndAddLocalsToContext`
now returns a `localsContextResult` struct with `SetLocals`,
`SetVars`, `SetSettings`, `SetEnv` flags identifying which sections
were populated by **this** file (vs. inherited). The overwrite
block is gated on those flags. When the recursion frame for
`_defaults.yaml` reaches the overwrite block, all four flags are
`false` (the file has no `locals:` section, so
`extractAndAddLocalsToContext` returns early), and the import's
own vars/settings/env survive untouched.

#### A3. Reject malformed rendered names

```go
// in deriveStackNameFromTemplate after ProcessTmpl
if strings.Contains(stackName, "<no value>") {
    log.Debug("Name template result contains <no value>; using filename",
        "file", stackFileName, "result", stackName)
    return ""
}

// Treat empty leading/trailing/consecutive segments around the
// configured delimiter (default "-") as malformed.
if hasEmptySegments(stackName, atmosConfig.Stacks.NameTemplate) {
    return ""
}
```

The fallback (`return ""`) routes to `deriveStackNameFromPattern`
and ultimately to the filename — which is correct for files where
the stack name genuinely can't be derived without imports.

#### Tests for Cluster A

Two new fixtures plus tests in `tests/cli_locals_test.go`:

| Fixture                                  | What it asserts                                                                                             |
|------------------------------------------|-------------------------------------------------------------------------------------------------------------|
| `locals-name-template-vars-imports`      | `_defaults.yaml` defines `vars.namespace`; leaf file has `vars.stage` + `locals:`. `list stacks` → `acme-prod`. |
| `locals-name-template-settings-imports`  | `_defaults.yaml` defines `settings.namespace/tenant/environment`; leaf file has `settings.stage` + `locals:`. `list stacks` → `cloudlabs-plat-ue1-prod`. `describe locals kms -s cloudlabs-plat-ue1-prod` succeeds. |

Plus a unit test in
`internal/exec/describe_locals_test.go` for `deriveStackNameFromTemplate`:

- Renders against a config with only `vars` populated where
  template references `.settings.X` → returns `""` (was: returned
  `"<no value>-..."`).
- Renders against a config with merged `vars` from imports →
  returns the correct name.
- Renders against a config where any segment is empty/`<no value>`
  → returns `""`.

---

## Cluster B: `!terraform.state` in locals re-evaluated N× per `list stacks`

### Issue

[#2344](https://github.com/cloudposse/atmos/issues/2344).

### Reproducer

```text
repro/
├── atmos.yaml                              # name_template: "{{ .vars.namespace }}-{{ .vars.stage }}"
├── components/terraform/mock/main.tf       # empty
└── stacks/orgs/
    ├── _defaults.yaml                      # vars.namespace: acme
    ├── a.yaml … e.yaml                     # 5 trivial filler stacks (no locals)
    └── target.yaml                         # ← the locals-bearing file
```

```yaml
# stacks/orgs/target.yaml
import: [./_defaults]
vars:
  namespace: acme   # duplicated to work around #2343
  stage: target
locals:
  vpc_id: !terraform.state vpc some-other-stack ".vpc_id"
components:
  terraform:
    app:
      metadata: { component: mock }
      vars: { vpc: "{{ .locals.vpc_id }}" }
```

```text
$ ATMOS_LOGS_LEVEL=Debug atmos list stacks 2>&1 | grep -c 'function="!terraform.state'
1362
```

For 7 state refs across 20 stacks, the user observed ~195× per
ref. With cloud auth disabled, each call fails fast (~15 ms) and
the command takes ~20 s; with auth, it remains linear in (refs ×
stack-tree visits).

### Code path

`extractLocalsFromRawYAML` (`internal/exec/stack_processor_utils.go:52`)
is called from `extractAndAddLocalsToContext` (`:263`), which is
itself called from `processYAMLConfigFileWithContextInternal` —
the recursive walker that visits every stack file (and every file
those files import) during `list stacks` /
`describe stacks` / etc.

For a target file that defines `locals:`:

```text
processYAMLConfigFileWithContextInternal(target.yaml)
  → extractLocalsFromRawYAML(target.yaml)
      → ProcessStackLocals
          → ExtractAndResolveLocals (global)
              → resolver.Resolve()
                  → for each local: yamlFunctionProcessor(value)
                      → ProcessCustomYamlTags
                          → !terraform.state → stateGetter.GetState(...)   ← N×
```

The `ProcessCustomYamlTags` path is **the entire point** of #2080 —
it's how `!terraform.state` works in locals at all. The bug is
that **nothing memoizes the result**. Every time the pipeline
revisits `target.yaml` (because another stack imports it, or the
walker is iterating, or the import graph touches it), the entire
locals block is re-resolved, and every YAML function is re-called.

### Fix

Memoize at the file level with a `sync.Map` (or per-`atmosConfig`
field) keyed by `(absFilePath, contentHashOrModTime)`:

```go
// internal/exec/stack_processor_utils.go

var localsExtractCache sync.Map  // map[string]*extractLocalsResult

func extractLocalsFromRawYAML(
    atmosConfig *schema.AtmosConfiguration,
    yamlContent string,
    filePath string,
) (*extractLocalsResult, error) {
    abs, err := filepath.Abs(filePath)
    if err != nil { abs = filePath }

    // Hash the content so a file edited between commands isn't served stale.
    sum := sha256.Sum256([]byte(yamlContent))
    cacheKey := abs + "\x00" + hex.EncodeToString(sum[:])

    if cached, ok := localsExtractCache.Load(cacheKey); ok {
        return cached.(*extractLocalsResult), nil
    }

    result, err := extractLocalsFromRawYAMLUncached(atmosConfig, yamlContent, filePath)
    if err != nil {
        return nil, err
    }
    localsExtractCache.Store(cacheKey, result)
    return result, nil
}
```

Memoizing at this level — rather than inside `pkg/locals/resolver.go` —
covers all callers (stack pipeline + `describe locals`) and
naturally caches the **resolved** locals (post-YAML-function),
which is what the user pays for.

#### Cache lifetime

The cache is process-local and lives for the duration of one
`atmos` command invocation. Each invocation starts a new process,
so there's no cross-command staleness risk. Within one command,
cache hits are correct because:

- `!terraform.state <c> <s> <q>` is purely a function of `(c, s, q)`
  → same inputs ⇒ same output.
- `!env`, `!exec`, `!store`, `!terraform.output` are likewise
  deterministic for the lifetime of the command.
- The content hash invalidates the entry if any byte of the file
  changes (defensive; not expected to happen mid-command, but cheap).

#### Why not memoize inside `pkg/locals/resolver.go`

The resolver is constructed fresh for each call — it doesn't have
a stable identity across invocations. Memoizing inside it would
require plumbing a cache through every constructor and would
still miss the "same file, two pipeline visits" case. Caching at
`extractLocalsFromRawYAML` is the narrowest correct layer.

#### Tests for Cluster B

Add a counting-stub test in
`internal/exec/stack_processor_utils_test.go` patterned after the
#2080 fix's `TestExtractLocalsFromRawYAML_TerraformStateInLocals`:

- Inject a `stateGetter` whose `GetState` increments a counter.
- Run `extractLocalsFromRawYAML` for the same file 100 times.
- Assert the counter is `1`, not `100`.

Plus a fixture-level integration test in `tests/cli_locals_test.go`:

- Use `tests/fixtures/scenarios/locals-yaml-functions` with a
  no-op `stateGetter` (the existing test infrastructure for
  `!terraform.state` mocks).
- Run `atmos list stacks`.
- Assert call count is `1` per unique `(component, stack, query)`,
  not `N × stack-tree-size`.

---

## Cluster C: `ProcessTmpl(NameTemplate, ..., false)` ignores `ignore_missing_template_values`

### Issue

[#2345](https://github.com/cloudposse/atmos/issues/2345).

### Background

PR #2158 (v1.209) added the global flag
`templates.settings.ignore_missing_template_values: true` so users
could opt into the Go-template stdlib's
`option("missingkey=zero")` behavior. The plumbing landed correctly
inside `ProcessTmpl` itself, but the audit that should have
followed — replacing every hardcoded `false` argument at call
sites — was never done.

### Affected call sites

`grep -rn 'ProcessTmpl(.*NameTemplate.*, false)' internal/exec/`:

| File                                              | Line  | Template label                          |
|---------------------------------------------------|-------|-----------------------------------------|
| `describe_stacks_component_processor.go`          | 506   | `describe-stacks-name-template`         |
| `describe_locals.go`                              | 518   | `describe-locals-name-template`         |
| `describe_affected_utils_2.go`                    | 428   | `spacelift-admin-stack-name-template`   |
| `describe_affected_utils_2.go`                    | 467   | `spacelift-stack-name-template`         |
| `aws_eks_update_kubeconfig.go`                    | 197   | `cluster_name_template`                 |
| `atlantis_generate_repo_config.go`                | 399   | `atlantis-stack-name-template`          |
| `spacelift_utils.go`                              | 111   | `name-template`                         |
| `stack_utils.go`                                  | 34    | `terraform-workspace-stacks-name-template` |
| `terraform_generate_backends.go`                  | 219   | `terraform-generate-backends-template`  |
| `terraform_generate_varfiles.go`                  | 171   | `terraform-generate-varfiles-template`  |
| `validate_stacks.go`                              | 363   | `validate-stacks-name-template`         |
| `utils.go`                                        | 364   | `name-template`                         |

### Compounding interaction with Cluster A

When the locals pre-pass mis-derives a stack name (Cluster A),
downstream component processing runs against component sections
whose `vars` may not match the user's `name_template`. If the
user has set `ignore_missing_template_values: true` to defend
against that, Cluster C means the flag is silently dropped at
`describe_stacks_component_processor.go:506` — they get a
`map has no entry for key "namespace"` error from the very flag
they set to suppress that error. This is exactly the failure mode
in #2345's repro.

### Fix

Mechanical replace at all 12 sites:

```go
// Before
name, err := ProcessTmpl(atmosConfig, "<label>",
    atmosConfig.Stacks.NameTemplate, info.ComponentSection, false)

// After
name, err := ProcessTmpl(atmosConfig, "<label>",
    atmosConfig.Stacks.NameTemplate, info.ComponentSection,
    atmosConfig.Templates.Settings.IgnoreMissingTemplateValues)
```

The `cluster_name_template` site
(`aws_eks_update_kubeconfig.go:197`) takes the same treatment for
`atmosConfig.Components.Helmfile.ClusterNameTemplate`.

#### Comment from issue thread

A commenter on #2345 asked whether silently substituting empty
strings for missing keys could produce broken stack names. The
answer is: **honoring the user's flag is the contract**. The flag
is opt-in and documented to do exactly that. The Cluster A fix
already adds defensive `<no value>` / empty-segment detection in
the one path that uses the rendered name as a primary identifier
(stack-name derivation in the locals pre-pass), so opt-in zeroing
won't silently produce malformed identifiers in the new code.

#### Regression guard

Add a `go vet`-style or unit-level scanner that fails the build if
`ProcessTmpl(... NameTemplate ..., false)` reappears. A simple
test that walks `internal/exec/*.go` with `go/ast` and asserts the
last argument to `ProcessTmpl` is `atmosConfig.Templates.Settings.IgnoreMissingTemplateValues`
(or another non-literal expression) for any call where the
template-label literal contains `name-template` is sufficient.

#### Tests for Cluster C

In `internal/exec/describe_stacks_component_processor_test.go`:

- Build an `atmosConfig` with
  `Templates.Settings.IgnoreMissingTemplateValues = true` and a
  `name_template` referencing `.vars.missing_key`.
- Call `resolveStackName` with a component section that lacks
  that key.
- Assert no error and a non-empty (zero-substituted) name.
- Verify the same call returns the missing-key error when the
  flag is `false`.

Repeat for the 11 other sites (table-driven against the public
entry points where possible).

---

## Why these three Clusters interact (Cluster A + Cluster C)

The four reports overlap because Cluster A's malformed stack name
flows into Cluster C's un-suppressed missing-key path:

```text
# 1. Cluster A: locals pre-pass derives a wrong name
deriveStackNameForLocals(target.yaml) → "-prod"  (namespace lost from imports)

# 2. The bad name goes into ProcessStackLocals(... currentStack="-prod")
#    causing !terraform.state 2-arg form to look up state in a
#    non-existent stack — but the resolver swallows the error and
#    leaves the local unresolved.

# 3. The pipeline continues. Later, describe_stacks_component_processor
#    runs ProcessTmpl(NameTemplate, info.ComponentSection, false).

# 4. Cluster C: hardcoded `false` ignores the user's
#    ignore_missing_template_values: true flag.
#    info.ComponentSection.vars may still be incomplete (because
#    the locals path mis-handled the file).

# 5. User sees:
#       map has no entry for key "namespace"
#    despite having set the flag to suppress exactly that error.
```

Cluster B (perf) is independent and fires whenever the project
uses `!terraform.state` (or any other YAML function) in `locals:`
across many stacks. It would have shipped on top of correct
results too — it just makes the symptoms worse during `list stacks`
because the broken behavior runs N× instead of 1×.

---

## Out of scope

- **Re-architecting locals to evaluate after import merge.** The
  current architecture (locals resolved before
  vars/settings/env templates) is correct per the PRD and is what
  enables `{{ .locals.* }}` references in `vars` to work at all.
  Cluster A fixes the leaf bug (derive the stack name correctly)
  without disturbing that ordering.
- **Component-level YAML functions.** This doc only addresses
  YAML functions used inside `locals:`. The same memoization
  pattern (Cluster B) could later be applied to component-level
  YAML functions; that's a separate fix.
- **Auditing every `ProcessTmpl(... false)` site.** Cluster C
  bounds itself to the 12 `*NameTemplate*` sites. Other
  `ProcessTmpl(... false)` calls in the codebase (e.g., one-off
  user-template rendering inside `internal/exec/utils.go`) may or
  may not also need the flag — those are evaluated separately.

---

## Files changed

### Production code

- `internal/exec/describe_locals.go` —
  `deriveStackName`/`deriveStackNameFromTemplate` now merge imports +
  pass `settings`/`env` to templateData; new
  `deriveStackNameSections`, `resolveImportFilePathsForStackName`,
  `mergeMapShallow` helpers; `<no value>` sentinel rejected.
- `internal/exec/stack_processor_utils.go` — new
  `localsContextResult` struct returned from
  `extractAndAddLocalsToContext`; `processYAMLConfigFileWithContextInternal`
  gates the stackConfigMap-overwrite block on
  `localsSetByThisFile.Set*`; new `extractLocalsCache` (`sync.Map`) and
  `extractLocalsFromRawYAML` cache wrapper around the renamed
  `uncachedExtractLocalsFromRawYAML`.
- `internal/exec/describe_stacks_component_processor.go` —
  `resolveStackName` honors `IgnoreMissingTemplateValues`.
- 9 other files in `internal/exec/` patched for the same flag at
  their `ProcessTmpl(... NameTemplate ..., false)` call sites.

### Tests

- `internal/exec/describe_locals_test.go` — 3 new tests
  (`TestDeriveStackName_MergesImportedVars`, `_MergesImportedSettings`,
  `_NoValueRejected`).
- `internal/exec/describe_stacks_component_processor_test.go` — 1
  new table-driven test
  (`TestResolveStackName_NameTemplate_HonorsIgnoreMissingTemplateValues`).
- `internal/exec/stack_processor_utils_test.go` — 3 new tests
  (`TestExtractLocalsFromRawYAML_Memoizes`,
  `_MemoizeInvalidatesOnContentChange`,
  `TestExtractAndAddLocalsToContext_ReturnsSetByFlags`); existing
  callers updated for the new return value.
- `tests/cli_locals_test.go` — 5 new fixture-level tests
  (`TestLocalsNameTemplate*`).

### Fixtures

- `tests/fixtures/scenarios/locals-name-template-vars-imports/`
- `tests/fixtures/scenarios/locals-name-template-settings-imports/`

## Related

- `docs/fixes/file-scoped-locals-not-working.md` —
  initial integration of locals into the stack pipeline (#1933).
  Established `extractLocalsFromRawYAML` and the pre-pass shape
  that Cluster A and Cluster B operate on.
- `docs/fixes/2026-03-15-locals-terraform-state-missing-stack-context.md` —
  introduced `deriveStackNameForLocals` to support `!terraform.state`
  2-arg form (#2080). Cluster A is a follow-up: that fix assumed
  identifying vars are local; it didn't anticipate the
  `_defaults.yaml` / settings hierarchy.
- `docs/prd/file-scoped-locals.md` — original spec; design intent
  preserved by all three Clusters.
- PR #2158 — added
  `templates.settings.ignore_missing_template_values`. Cluster C
  finishes the audit that should have accompanied that PR.
- `internal/exec/describe_locals.go:504-536` — Cluster A leaf.
- `internal/exec/stack_processor_utils.go:52-93` — Cluster A
  caller + Cluster B leaf.
- `internal/exec/describe_stacks_component_processor.go:506` —
  Cluster C primary site.
