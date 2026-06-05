# Component `source`: Simple-Form String Rejected + `metadata.source` Silently Ignored

**Date:** 2026-06-04
**Introduced by:** the JIT source provisioner (source-provisioner PRD, `pkg/provisioner/source`) — the
stack processor's `source` extraction never kept pace with the two input forms the provisioner accepts.
**Severity:** Medium — a documented, supported config (`source: <go-getter URI>`) fails before JIT
provisioning ever runs; a closely-related misconfiguration (`metadata.source`) fails with no clue why.
**Reproducers:**
- `internal/exec/stack_processor_source_normalize_test.go` — `TestNormalizeComponentSourceSection`
- `pkg/component/workdir_path_test.go` — `TestSourceMisplacedUnderMetadata`

---

## Why this is a fix doc (and not a blog post / changelog entry)

Both changes make the **already-documented** component `source` behavior (source-provisioner PRD) work
as written — the simple-form string is documented and accepted by the provisioner, and the warning only
turns an existing silent failure into an actionable message. No net-new user-facing capability — no new
command, flag, or config surface. Per the repo's label decision tree that is a `patch`/`no-release`
docs concern, not a `minor`. The rationale and the design notes are captured here.

These fixes are **separate** from the github/sts envelope + import token-shadowing chain documented in
`2026-06-04-github-sts-envelope-and-import-token-shadowing.md`; they happened to ride along in the same
PR (#2568).

---

## Symptom

### 1. Simple-form `source` string rejected

The component `source` section has two documented forms. The full map form:

```yaml
components:
  terraform:
    vpc:
      source:
        uri: github.com/cloudposse/terraform-aws-components//modules/vpc
        version: 1.2.3
```

…and the **"simple form"** — a bare go-getter URI string:

```yaml
components:
  terraform:
    vpc:
      source: github.com/cloudposse/terraform-aws-components//modules/vpc
```

The JIT provisioner (`pkg/provisioner/source`, `parseSource`/`HasSource`) has always accepted both. But
the **stack processor** only accepted the map form and rejected the string outright:

```text
invalid component source section: 'components.terraform.vpc.source' in the file '<stack>'
```

So the simple form failed during stack processing, before JIT provisioning ever ran. Every
source-provisioner fixture uses the map form, so the gap went unnoticed.

### 2. `source` misplaced under `metadata` fails silently

Declaring the source under `metadata` is a natural-looking footgun:

```yaml
components:
  terraform:
    vpc:
      metadata:
        source: github.com/org/repo//module   # WRONG — silently ignored
```

`source` is a **top-level** component section (a sibling of `vars`/`settings`/`metadata`).
`metadata.source` is **not** a schema field, so it is dropped during decode, JIT provisioning never
runs, and the component fails to resolve with a confusing, cause-free error:

```text
Terraform component does not exist
```

This is unrelated to `metadata.component` (a base component to inherit from), which *is* valid.

---

## Root Cause

### 1. Extraction only accepted the map form

the `source` extraction in `extractComponentSections`
(`internal/exec/stack_processor_process_stacks_helpers_extraction.go`) type-asserted the `source` value
to `map[string]any` and returned `ErrInvalidComponentSource` for anything else — including the string
the provisioner downstream is perfectly happy to parse. The normalized map is what flows into the final
`source` section, so the processor was strictly more restrictive than the consumer.

### 2. `metadata.source` is dropped with no diagnostic

Nothing inspected `metadata` for a misplaced `source`. `HasSource(info.ComponentSection)` correctly
reports "no source" (because there is no *top-level* source), the code falls back to the local component
dir, that dir does not exist, and the user gets a generic "does not exist" — with no hint that a
misplaced `source` one level down was the actual cause.

---

## Fix (PR #2568)

1. **Simple-form acceptance** — new `normalizeComponentSourceSection(raw any) (map[string]any, bool)`
   coerces the `source` value into the canonical map form used throughout stack processing:
   - `string` → `{uri: <string>}` (empty string → empty map, i.e. "no source"),
   - `map[string]any` → passthrough,
   - anything else → `ok=false`, so the caller still raises `ErrInvalidComponentSource`.

   It is applied at **both** extraction sites (component `source` and base-component `source`
   inheritance) in `stack_processor_process_stacks_helpers_extraction.go`. This keeps the stack
   processor consistent with the JIT provisioner, which has always accepted both forms.

2. **Misplacement warning** — `ProvisionAndResolveComponentPath`
   (`pkg/component/workdir_path.go`) now calls `warnIfSourceMisplacedUnderMetadata` at the exact point
   it falls back to the local component dir (no top-level `source` found). `sourceMisplacedUnderMetadata`
   detects a non-empty `metadata.source` and emits an actionable warning telling the author to move
   `source` up one level. Behavior is unchanged — only a warning is added — and the single shared
   chokepoint covers terraform, helmfile, and packer.

3. **PRD callout** — `docs/prd/source-provisioner.md` gains an explicit "`source` is a top-level
   component section — not under `metadata`" note, and the one inconsistent example in the git-worktree
   section (which had nested `source` under `metadata`, the exact form that leaked into a downstream
   config) was corrected.

---

## Tests

- `internal/exec/stack_processor_source_normalize_test.go` — `TestNormalizeComponentSourceSection`:
  string → `{uri}`, empty string → empty map, map passthrough, and the invalid-type → `ok=false` path.
- `pkg/component/workdir_path_test.go` — `TestSourceMisplacedUnderMetadata`: string and map
  `metadata.source` detected; empty-string and absent cases treated as "not misplaced" (matching
  `HasSource`).

---

## Verification

```bash
go build ./...
go test ./internal/exec/... -run TestNormalizeComponentSourceSection
go test ./pkg/component/... -run TestSourceMisplacedUnderMetadata
```

End-to-end: a component using `source: github.com/org/repo//module` (simple form) now processes and JIT-
provisions instead of failing with "invalid component source section"; a component with
`metadata.source` still fails to resolve (unchanged) but now logs a warning pointing at the misplacement.

---

## Related

- `docs/prd/source-provisioner.md` — source provisioner PRD (top-level-`source` callout, the two forms)
- `internal/exec/stack_processor_process_stacks_helpers_extraction.go` — `normalizeComponentSourceSection`
- `pkg/component/workdir_path.go` — `ProvisionAndResolveComponentPath`,
  `sourceMisplacedUnderMetadata`, `warnIfSourceMisplacedUnderMetadata`
- `pkg/provisioner/source` — `parseSource`/`HasSource` (the consumer that always accepted both forms)
- PR #2568 (this fix). Commits: `e851d8438` (simple-form string), `4532a760f` (metadata warning),
  `0e562ad43` (PRD clarification)
