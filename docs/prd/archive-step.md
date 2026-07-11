# PRD: Native Archive Step (`type: archive`)

**Status:** Phase 1 implemented
**Version:** 1.1
**Last Updated:** 2026-07-11
**Author:** Erik Osterman

---

## Problem Statement

Packaging build artifacts — most commonly zipping a directory of source files into a
deployable archive (e.g. an AWS Lambda function's `handler.zip`) — had no native Atmos
primitive. The only way to do it was to shell out to a platform-specific binary
(`zip`, `tar`) via a generic `kind: command` hook or `type: shell` step.

This gap surfaced concretely while migrating a Terragrunt example (a Lambda + DynamoDB
+ IAM role stack) to Atmos. The source Terragrunt unit uses a `before_hook "package"`
that calls an external `scripts/package.sh` wrapping the `zip` binary, before every
`plan`/`apply`/`destroy`. The direct Atmos translation of that `before_hook` required
an equivalent shell-based `kind: command` hook — which works, but:

- **Breaks cross-platform portability.** This repository's own conventions
  (`CLAUDE.md`) explicitly forbid relying on platform-specific binaries in tests
  (`zip`/`tar`/`unzip` are not guaranteed to exist on a Windows CI runner) — the same
  concern applies to any hook/step pattern Atmos documents and users copy into their
  own projects.
- **Requires hand-written shell scripts** for a very common task, with no declarative
  schema, no typed validation, and errors that surface only as opaque shell failures.
- **Has no `terraform`-graph-native alternative that reliably works either.** A
  `data "archive_file"` (`hashicorp/archive` provider) data source was tried first
  during the same migration; it failed because there's no dependency edge forcing it
  to run before a `locals` block elsewhere in the same module reads the file it
  produces — Terraform doesn't guarantee data-source-before-local-eval ordering
  without an explicit reference. A `before.terraform.*` hook remains the only reliable
  place to run packaging logic before Terraform starts, but that hook's *body*
  shouldn't have to be a hand-rolled shell script.

## Goals

- A native, declarative `archive` **step type** (`type: archive`), usable in
  workflows, custom commands, and — via the existing generic hook bridge — as a
  component lifecycle hook. See [Design: one implementation, not two](#design-one-implementation-not-two).
- Implemented on Go's standard library (`archive/zip`, `archive/tar`,
  `compress/gzip`) with zero required external binary dependency, so it behaves
  identically on macOS, Linux, and Windows.
- Support formats: `zip`, `tar`, `tgz` (`tar.gz`), `tar.bz2`, `tar.xz`.
- Support `include`/`exclude` glob filtering.
- Support a `subpath` concept that reads symmetrically in both directions: on pack,
  where the source content is nested inside the resulting archive; on unpack, which
  path inside the archive gets extracted (with that prefix stripped from the output).
- Define a complete 4-action vocabulary now — `create`, `extract`, `update`,
  `replace` — even though V1 only implements a subset, so the schema never needs a
  breaking change as later actions are filled in.

## Design: one implementation, not two

The original draft of this PRD proposed two separate primitives: a hook `kind:
archive` (bound to `before.terraform.*`/`after.terraform.*` events like any other
hook kind) and a workflow/custom-command `type: archive` step, implemented
independently.

Codebase research changed that scope. Atmos already has a generic bridge —
`kind: step` (`pkg/hooks/step_engine.go`, formalized in
[Run custom step types as component lifecycle hooks](./hooks-step-types.md)) — that
runs **any** registered step type as a hook via `kind: step` + `type: <name>` +
`with:`. Existing step types (`emulator`, `http`, `container`) have **no dedicated
hook kind** — that PRD's explicit goal is "the entire step library becomes available
on Terraform lifecycle events without growing the hook-kind list."

`archive` follows that established precedent: it ships as a step type only. Hooks get
it for free as:

```yaml
hooks:
  package:
    kind: step
    type: archive
    events: [before.terraform.plan, before.terraform.apply, before.terraform.destroy]
    with:
      source: src/
      destination: handler.zip
```

This halves the implementation versus the original draft — no
`pkg/hooks/kinds/archive/` package, no new hook-kind schema enum entry, no
kind-level tests — while producing an identical user-facing capability, consistent
with how every other step type reaches hooks.

## Non-Goals (V1)

- **`action: create` and `action: extract` are reserved in the schema but not
  implemented in V1.** Selecting either returns a clear, typed "not yet supported"
  validation error — never a silent no-op or a fallback to different behavior.
- **`action: update` is never supported on formats where an incremental edit isn't
  actually incremental.** The classification rule is not "is this format
  compressed?" — `zip` entries are individually compressed and still supports
  update just fine. The rule is: **can one entry be added or replaced without
  touching the rest of the archive stream?**
  - `zip`: yes — each entry is compressed independently and the central directory
    can be rewritten cheaply. **Update supported.**
  - `tar` (uncompressed): yes — no compression at all, entries can be appended/
    rewritten directly. **Update supported.**
  - `tgz`/`tar.gz`, `tar.bz2`, `tar.xz`: no — these wrap the *entire* tar byte
    stream in one continuous compression pass. Touching a single entry requires
    decompressing and recompressing the whole archive, which is not meaningfully
    cheaper than `replace` and would mislead users into thinking they're getting a
    real incremental operation. **Update is rejected outright with a clear error
    naming the unsupported format**, not silently downgraded to a full rebuild
    under an incremental-sounding name. (If a future format supports surgical
    edits despite being "compressed" in some sense — e.g. certain streaming
    formats with independent per-entry frames — it should be classified as
    update-capable using this same test, not by a blanket compressed/uncompressed
    rule.)
- No multi-source-per-archive support (e.g. combining several independently-built
  directory trees into one archive, each under its own `subpath` — a pattern AWS
  Lambda Layers sometimes need). V1 is single `source` per archive operation.
- No archive-inspection commands (list contents, verify integrity). A natural future
  follow-on, not required for the packaging use case that motivated this PRD.
- **`tar.bz2`/`tar.xz` write support is not implemented in Phase 1** — see
  [Open Questions](#open-questions).

## Schema

Flat fields on the step (`type: archive`); the same fields work identically inside a
`kind: step` hook's `with:` block:

```yaml
type: archive
action: replace           # create | extract | update | replace — see Non-Goals for V1 scope
format: zip                # zip | tar | tgz | tar.bz2 | tar.xz
                            # inferred from destination (pack) / source (extract)
                            # extension when omitted; explicit value always wins
source: src/                 # pack: directory/file(s) to archive
                            # extract: archive file to read
destination: handler.zip   # pack: archive file to write
                            # extract: directory to extract into
subpath: ""                  # pack: nest source content under this path inside the archive
                            # extract: only extract this path from inside the archive,
                            # with the prefix stripped
include:
  - "**/*.js"
exclude:
  - "**/*.test.js"
  - "**/node_modules/**"
```

`action` and `source` are shared fields on the step schema (`pkg/schema/workflow.go`,
`pkg/schema/task.go`), reused from the `container` step type (`action`) and the
`workdir` step type (`source`) respectively — the same pattern the container step
already established, where a field's meaning is scoped by the step's `type:`, not
globally unique across all ~25 step types.

### Action semantics

| Action | V1 status | Behavior |
|---|---|---|
| `create` | Reserved, not implemented | Build a new archive at `destination`; error if `destination` already exists. |
| `replace` | **Implemented — default action for V1** | Always rebuild `destination` fresh from `source`; the archive is fully overwritten regardless of prior contents. Works on every supported format. |
| `update` | **Implemented for `zip` and uncompressed `tar` only** | Add new / refresh changed entries from `source` into the existing `destination` archive; entries not touched by `source` are left as-is. Rejected with a clear error for `tgz`/`tar.bz2`/`tar.xz` — see Non-Goals. |
| `extract` | Reserved, not implemented | Unpack `subpath` of `source` into `destination`. |

For V1, the practical entry point is `action: replace` — every real use case this PRD
was written for (packaging a Lambda zip fresh before every plan/apply/destroy, the
Terragrunt `before_hook` equivalent) only needs "always rebuild this archive," which
`replace` covers on every format without any of the update-capability caveats above.

### Format detection

If `format` is omitted, infer it from the relevant path's extension — `destination`
for pack actions, `source` for extract actions: `.zip` → zip, `.tar` → tar,
`.tar.gz`/`.tgz` → tgz, `.tar.bz2`/`.tbz2` → tar.bz2, `.tar.xz`/`.txz` → tar.xz.
Explicit `format:` always overrides inference and is required when the extension is
ambiguous or absent.

### Cross-platform implementation

- `zip`: Go stdlib `archive/zip` (read + write). No external dependency.
- `tar` / `tgz`: Go stdlib `archive/tar` + `compress/gzip`. No external dependency.
- `tar.bz2` / `tar.xz`: the Go standard library has **read-only** bzip2
  (`compress/bzip2`) and **no** xz support at all. Writing either requires a small,
  pure-Go dependency (`github.com/dsnet/compress` for bzip2 write,
  `github.com/ulikunitz/xz` for xz write) — both are already **indirect**
  dependencies in `go.mod` (pulled in transitively by `github.com/jfrog/archiver/v3`,
  used elsewhere for toolchain/vendor downloads), so adopting them for archive write
  support is a promotion to direct requirement, not new supply-chain surface. Not
  implemented in Phase 1; see Open Questions.

## Implementation

- **`pkg/archive/`** — new package (mirrors `pkg/generator/`'s shape: dispatch at
  the top, format-specific logic split into small files):
  - `archive.go` — `Run(action Action, opts *PackOptions) error` dispatch by action.
  - `format.go` — extension-based format inference + the update-capability rule.
  - `walk.go` — source-tree walking, include/exclude filtering (via
    `pkg/utils.PathMatch`, the same doublestar matcher the vendor manifest's
    `included_paths`/`excluded_paths` already uses), and archive-path joining
    (always forward-slash, independent of OS).
  - `zip.go` / `tar.go` — format-specific pack/update, using stdlib only.
- **`pkg/runner/step/archive.go`** — the `ArchiveHandler` step type (`Register`ed
  as `"archive"`), following the `http` step type's structural precedent (own
  config, own error taxonomy, no shelling out).
- **`pkg/schema/workflow.go`, `pkg/schema/task.go`** — new `Format`, `Destination`,
  `Subpath`, `Include`, `Exclude` fields (flat, matching the `http` step's
  precedent), reusing the existing `Action` and `Source` fields; wired through
  `Task.ToWorkflowStep()` / `TaskFromWorkflowStep()`.
- **`errors/errors.go`** — new sentinel errors for both the `pkg/archive` package
  and the step's own validation, built via the `errUtils.Build(...)` error-builder
  pattern.
- **No hook-side code** — `kind: step` + `type: archive` already works through
  `pkg/hooks/step_engine.go` unmodified; a hook-bridge test
  (`pkg/hooks/step_engine_test.go`) proves it.

### Tests

Table-driven across format × action in `pkg/archive/archive_test.go`, explicitly
covering the negative cases (`update` + any compressed-tar format → typed error;
`create`/`extract` selected in V1 → typed "not yet supported" error). No test shells
out to `zip`/`tar`/`unzip` — archives are built and read back purely through Go's own
`archive/zip`/`archive/tar` packages, proving the cross-platform claim structurally
rather than asserting it in prose. `pkg/runner/step/archive_test.go` covers
`Validate`/`Execute` following the `http` step's test pattern.

### Docs

- `website/docs/workflows/workflows/workflow/steps/type/archive.mdx` — step type
  reference, matching the `emulator`/`http` doc pattern.
- Row added to the step-type summary tables (`type.mdx`,
  `_step-types.mdx`).
- `website/docs/stacks/hooks.mdx` — an `archive` example alongside the existing
  `emulator` example under `kind: step`.

## Phased Rollout

1. **Phase 1 (implemented) — `replace`, pack only, `zip`/`tar`/`tgz`.** Ships the
   primitive that replaces the Terragrunt-`before_hook`-style shell-out pattern for
   the "always rebuild this archive fresh" case (the motivating Lambda-zip use
   case). Scoped to `zip`/`tar`/`tgz` only — fully stdlib, zero new dependencies —
   deferring `tar.bz2`/`tar.xz` write support to a follow-up once the dependency
   promotion is a deliberate decision, rather than pulling it into the first pass.
   `tgz` alone already covers the overwhelming majority of real-world
   compressed-archive packaging needs.
2. **Phase 1 (implemented) — `update` for `zip` and uncompressed `tar`.**
   Incremental add/refresh into an existing archive, with compressed-tar formats
   explicitly and permanently rejected (not "not yet" — see Non-Goals). Shipped in
   the same PR as `replace`, since both actions share the same package.
3. **Phase 2 (future, not scoped here) — `create` and `extract`.** `extract` is
   the natural inverse of `replace`/`create` and reuses most of the
   format/subpath/include-exclude machinery already built.
4. **Phase 3 (future, not scoped here) — multi-source archives, archive-inspection
   commands, `tar.bz2`/`tar.xz` write support.**

## Open Questions

1. **Is the `tar.bz2`/`tar.xz` write-support dependency promotion worth taking
   now, or should it wait?** Recommendation: wait. `zip`/`tar`/`tgz` alone cover
   the motivating use case and the large majority of real-world archive needs;
   add bzip2/xz write support only once there's a concrete user need, as a
   deliberate, isolated decision rather than bundled into Phase 1.

## Related

- [Run custom step types as component lifecycle hooks](./hooks-step-types.md) —
  the `kind: step` bridge this feature relies on instead of a dedicated hook kind.
- [Custom Hooks](./custom-hooks.md) — the hook `kind` registration contract; not
  used by this feature, but the reference point for why it wasn't.
- [Generate Terraform Files](/stacks/generate) — a sibling declarative-file-producing
  mechanism (text/templated files vs. this PRD's binary archives); worth
  cross-referencing so users know which one fits their need.
- [DAG-Based Concurrent Execution](./dag-concurrent-execution.md) — component hooks
  (including `kind: step` / `type: archive`) are subject to the separately-tracked
  bug where hooks don't fire under `--all`/`--affected`/`--query` dispatch; that
  must be fixed independently of this feature, but is directly relevant to anyone
  relying on the archive step inside a `before.terraform.*` hook in bulk-execution
  contexts.
