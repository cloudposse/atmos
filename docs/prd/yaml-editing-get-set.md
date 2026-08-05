# PRD: Format-Preserving YAML Editing (Get / Set / Delete) for Atmos

## Status

- **Phase 1 (core engine in `pkg/yaml`)**: implemented and tested.
- **Phase 2 — Config** (`atmos config get|set|delete|list|format`): implemented and
  tested (E2E verified). `atmos config` has no separate `config` sub-namespace — this
  flat surface *is* the canonical form for the config domain.
- **Phase 2 — Stack**: `atmos stack config get|set|delete|format|list`
  (`cmd/stack/config.go`) is the canonical command set, all built on shared free
  functions (`runStackGet`/`runStackSet`/`runStackDelete`/`runStackFormat` in
  `cmd/stack/operations.go`). `atmos stack get|set|delete|format`
  (`cmd/stack/operations.go`) are literal code-level aliases — the flat `RunE`
  closures call the exact same functions. `list` has no flat form: a flat
  `atmos stack list` would collide with the existing `atmos list stacks`, so
  `atmos stack config list` is the only way to invoke it. Provenance-based file
  resolution + `--file` override, E2E verified.
- **Phase 2 — Vendor**: `atmos vendor config get|set|delete|format|list`
  (`cmd/vendor/config.go`) is the canonical command set, addressing `vendor.yaml` by
  arbitrary dot-notation path (e.g. `spec.sources[0].version`) via the shared
  `pkg/yaml` engine. `atmos vendor get|set` (`cmd/vendor/edit.go`) are aliases that
  resolve a **component name** to its `spec.sources[N].version` path (via
  `pkg/vendoring.ComponentVersionPath`, matched by name so manifest ordering does
  not matter) and delegate to the same `get`/`set` engine as `vendor config` — not a
  separate implementation. `delete`, `format`, and `list` have no component-name
  shortcut and are only available via `atmos vendor config`.
- **Follow-up within this effort**: port the remaining `feat/vendor-diff-and-update`
  feature set (`atmos vendor diff` / `atmos vendor update`, version-constraint
  resolution, git-diff, source providers) and close that branch. That branch is
  ~6 months stale relative to `main` and needs a careful rebase; the YAML-write
  path will route through this engine.

## Problem

Atmos users and automation routinely need to read and modify Atmos YAML —
`atmos.yaml` (Atmos Config), stack manifests (Stacks), and `vendor.yaml`
(Vendor) — programmatically: bump a vendored component version, flip a setting,
update a region, etc. Today this means reaching for `sed`, `yq`, or hand-editing.

These approaches are fragile:

- `sed`/regex edits are positional and unaware of YAML structure.
- A naive structured edit (unmarshal into a Go map → marshal back) **destroys
  comments, anchors/aliases, and formatting** — unacceptable for human-authored
  config that relies on comments and DRY anchors.

We want first-class Atmos commands to do this safely, preserving fidelity.

## Goals

1. A **reusable core package** (`pkg/yaml`) that can Get / Set / Delete / Format
   values in YAML **while preserving comments, anchors/aliases, formatting,
   Atmos YAML function tags (`!terraform.output`, `!env`, `!store`, …), and
   embedded Go/Gomplate templates (`{{ … }}`)**.
2. **yq-style addressing, dot-notation by default** (`vars.region`,
   `sources[0].version`), with a raw-yq escape hatch for power users.
3. **Domain-shaped commands** built on the core — one each for **Config**,
   **Stack**, and **Vendor** — optimized for that domain's schema and
   file-resolution rules. Every operation is conceptually "edit Atmos Config" or
   "edit a Stack" or "edit Vendor", not a generic raw-YAML tool.
4. **Strict anchor/alias safety**: an edit that would alter or expand an anchor
   or alias **fails loudly** rather than silently rewriting shared data.
5. Subsume the vendor diff/update feature from `feat/vendor-diff-and-update` and
   route its YAML writes through this engine.

## Non-Goals

- A general-purpose `yq` replacement CLI. The surface is domain-shaped.
- Editing remote/non-file sources.
- Reformatting that intentionally rewrites anchors (the strict guard forbids it).

## Design

### Why yq (yqlib) on raw bytes

Atmos already vendors `github.com/mikefarah/yq/v4`. The existing wrapper
`pkg/utils.EvaluateYqExpression` round-trips data **through Go types**
(`ConvertToYAML(data)`), which is why it cannot preserve formatting — by the time
yq sees the data, comments/anchors are already gone.

The key realization: feeding **raw file bytes** straight into yqlib's
`StringEvaluator` with an assignment expression edits the document **in place**,
the same way `yq -i` does, preserving comments, anchors, and aliases. This was
validated with a spike before building the engine. So the engine already exists
in-tree; we only add a raw-bytes entry point plus a safety guard.

### Core API (`pkg/yaml`)

All operations funnel through a single `evaluate(content, expr)` choke point that
runs yqlib with fixed preferences (2-space indent, no colors, doc separators
preserved, scalars unwrapped on read) and silences yq's internal logger.

| Function | Purpose |
| --- | --- |
| `Get(content, path) (string, error)` | Read a value (missing/null → `ErrYAMLPathNotFound`). |
| `GetTyped[T](content, path) (T, error)` | Read and decode into `T`. |
| `Set(content, path, value) ([]byte, error)` | Assign a string scalar. |
| `SetRaw(content, path, rhs) ([]byte, error)` | Assign a typed/raw yq RHS (`true`, `42`, …). |
| `Delete(content, path) ([]byte, error)` | Remove a node (`del(...)`). |
| `Eval(content, expr) ([]byte, error)` | Raw yq expression escape hatch. |
| `Format(content) ([]byte, error)` | Identity reformat / normalize. |
| `GetFile` / `SetFile` / `SetFileRaw` / `DeleteFile` / `FormatFile` | File wrappers with **atomic write** (temp + rename, mode-preserving). |

### Addressing: dot-notation → yq path

`DotPathToYqPath` translates user-facing dot paths into yq path expressions:

```
vars.region                         -> .vars.region
sources[0].version                  -> .sources[0].version
components.terraform.vpc.vars.cidr  -> .components.terraform.vpc.vars.cidr
metadata."weird.key"                -> .metadata.["weird.key"]
```

Simple identifier keys are emitted bare; keys with dots/symbols use yq's quoted
`.["..."]` form. A path already starting with `.` is treated as a raw yq
expression and passed through unchanged.

### Strict anchor/alias guard

yqlib preserves anchor *topology*, but assigning **through an alias** silently
mutates the shared anchor — e.g. setting `.components.vpc.tags.Team` when
`vpc.tags` is `*commontags` also changes every other location that aliases
`&commontags`. A before/after comparison of the anchor *set* alone misses this,
because the set is unchanged.

`guardAnchors(before, after)` therefore compares, per anchor name:

1. **Existence** — anchor added, removed, renamed, or expanded → reject.
2. **Alias count** — number of aliases referencing it changed (an alias was
   flattened to a literal, or a new one appeared) → reject.
3. **Shared content** — an anchor that is referenced by ≥1 alias had its value
   change → reject (this is the silent-shared-mutation case).

Edits that touch no anchor/alias relationship always pass. Violations return
`ErrYAMLAnchorAltered` with a hint to edit the anchor definition explicitly or
restructure.

### Domain commands (Phase 2)

The three domains never re-implement YAML editing; they only (a) resolve **which
file**, (b) shape the **dot-path** for their schema, (c) optionally validate. For
each domain, `get|set|delete|format|list` is the **canonical** command set; where a
flat/shorthand command exists, it is a convenience alias of the canonical form, not a
separate implementation.

- **Config** — `atmos config get|set|delete|list|format <dot-path> [value]`.
  Auto-resolves the active `atmos.yaml` (with `--config` override). Paths are
  config-relative. This flat surface is already canonical — there is no separate
  `atmos config config` sub-namespace.
- **Stack** — canonical: `atmos stack config get|set|delete|format|list -s <stack>
  -c <component> '<dot-path> [= value]'` (`cmd/stack/config.go`). Uses the existing
  **provenance** package (`pkg/merge` + `internal/exec` merge-context store) to find
  the *winning* source manifest for a merged value
  (`ProvenanceStorage.GetLatest(path).File`), then edits that file. If the path is
  defined nowhere, an explicit `--file` is required (never guess where to create new
  keys). The edited file is always reported. `atmos stack get|set|delete|format`
  (`cmd/stack/operations.go`) are code-level aliases — their `RunE` closures call the
  identical `runStackGet`/`runStackSet`/`runStackDelete`/`runStackFormat` functions
  used by the `stack config` subcommands. `list` has no flat form (would collide
  with `atmos list stacks`), so it is only reachable via `atmos stack config list`.
- **Vendor** — canonical: `atmos vendor config get|set|delete|format|list`
  (`cmd/vendor/config.go`), addressing `vendor.yaml` by arbitrary dot-notation path,
  with all YAML writes routed through `pkg/yaml.SetFile`. `atmos vendor get|set`
  (`cmd/vendor/edit.go`) are aliases: they resolve a **component name** to its
  `spec.sources[N].version` path (`pkg/vendoring.ComponentVersionPath`, matched by
  name on every invocation so manifest ordering never matters) and then call the
  same `runVendorConfigGet`/`runVendorConfigSet` functions the `vendor config`
  subcommands use — not the separate hardcoded `sources[].version` writer the
  superseded branch used. `delete`, `format`, and `list` have no component-name
  shortcut; use `atmos vendor config` for those. Also ported: the remaining
  `atmos vendor update` / `atmos vendor diff` feature set.

### `format`

`Format`/`FormatFile` power `<domain> [config] format` in all three domains: `atmos
config format`, `atmos stack config format` (alias: `atmos stack format`), and
`atmos vendor config format`. Each formats the relevant manifest(s) in place,
preserving comments, anchors, YAML functions, and templates.

## Errors

Package-local sentinels in `pkg/yaml/errors.go`: `ErrInvalidYAMLExpression`,
`ErrYAMLPathNotFound`, `ErrYAMLUpdateFailed`, `ErrYAMLAnchorAltered`,
`ErrParseYAML`, `ErrReadFile`, `ErrWriteFile`. Domain layers wrap these with
the error builder + hints.

## Preservation guarantees (verified)

Validated empirically and locked in by tests, an edit to one value preserves,
verbatim, everything it did not target:

- **Comments** — head, line/inline, and foot comments (inline-comment spacing is
  normalized to a single space).
- **Anchors & aliases** — anchor definitions, alias references, and YAML merge
  keys (`<<: *anchor`); the strict guard rejects edits that would change shared
  anchor data.
- **Atmos YAML functions** — custom tags such as `!terraform.output`, `!env`,
  `!store`, `!terraform.state` round-trip unchanged.
- **Go/Gomplate templates** — `{{ … }}` expressions in single-quoted,
  double-quoted, literal-block, and unquoted scalars round-trip without delimiter
  mangling, both when adjacent to an edit and when set as a new value.

## Testing

- `pkg/yaml/edit_test.go` — comment preservation across set/delete; nested and
  indexed paths; typed `SetRaw`; `Get`/`GetTyped`; not-found; atomic file write
  with mode preservation; dot-path translation; the three anchor-guard rejection
  cases (edit-through-alias, edit-anchor-def, untouched-anchors-preserved). The
  edit-through-alias rejection was written as a failing test first.
- `pkg/yaml/stability_test.go` — a "kitchen-sink" fixture (block/folded scalars,
  flow collections, quoting styles, unicode, merge keys, sequences of maps) plus
  focused fixtures asserting comments + anchor topology survive set/delete/format
  and that `Format` is idempotent.
- `pkg/yaml/functions_templates_test.go` — asserts Atmos function tags and Go
  templates are preserved verbatim across set/delete/format and round-trip when
  set as values.

## Known limitations

- yqlib normalizes inline-comment spacing to a single space and standardizes
  indentation to the configured value; untouched lines are otherwise preserved.
  This is "preserve as much as possible", not byte-identical round-tripping.
- **Blank lines are not preserved.** The `gopkg.in/yaml.v3` node model (which
  yqlib is built on — and which the superseded branch also used) does not record
  blank lines between mapping entries, so they are collapsed on write. Comments,
  anchors, function tags, and templates are preserved; purely cosmetic blank
  separators are not. This is inherent to the yaml.v3 ecosystem.
- `Get` treats an explicit `null` the same as a missing key
  (`ErrYAMLPathNotFound`).
- Anchor-heavy documents where the user genuinely wants to edit shared data must
  edit the anchor definition explicitly; the strict guard rejects implicit
  shared mutation by design.
