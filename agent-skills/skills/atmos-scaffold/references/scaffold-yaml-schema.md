# scaffold.yaml Schema Reference

The full `AtmosScaffoldConfig` manifest schema (`pkg/project/config.ScaffoldSpec`,
JSON Schema generated at `pkg/datafetcher/schema/scaffold/scaffold-config/1.0.json`).

## Envelope

```yaml
apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: my-template        # required
  description: ...
  author: ...
  version: ...
spec:
  source: ...               # provenance; written to project records, ignored in templates
  baseRef: ...               # 3-way-merge base ref; written to project records
  delimiters: ["[[", "]]"]   # optional: override the default {{ }} Go template delimiters
  fields: [...]              # the questionnaire
  values: {...}              # preset/default answer values keyed by field name
  files: [...]               # optional conditional-generation overlay
  hooks: {...}               # optional pre/post-generate hooks
```

## `spec.fields[]` — questionnaire

```yaml
- name: component_name        # required; used as the template variable (.Config.component_name)
  type: input                 # input|text|string|select|multiselect|confirm|bool|boolean|computed
  label: Component name       # short prompt label (falls back to name if omitted)
  description: ...            # longer help text shown with the prompt
  required: true
  default: my-default
  options: [a, b, c]          # select/multiselect only; see below for {label,value}/dynamic forms
  placeholder: e.g. vpc       # input fields only
  validation:
    pattern: '^[a-z][a-z0-9-]*$'
    message: "Must be lowercase alphanumeric with hyphens"
  when: "answers.some_earlier_field == true"   # optional; gates whether this field is shown
```

Field name uniqueness is enforced — a duplicate `name` fails to load
(`ErrDuplicateScaffoldFieldName`) rather than silently dropping an answer.

### `type: computed` — derived fields

A `computed` field is never prompted for and can't be set with `--set`
(`ErrScaffoldComputedFieldNotSettable`) — it's always derived from other
answers via a `value:` Go-template expression, evaluated with the same
`answers.*` binding `options:`'s dynamic form uses:

```yaml
- name: regions
  type: multiselect
  options: [us-east-1, us-west-2, eu-west-1]
- name: primary_region_select
  type: select
  options: answers.regions
  when: "size(answers.regions) > 1"
- name: primary_region
  type: computed
  value: "{{ ternary answers.primary_region_select (index answers.regions 0) (gt (len answers.regions) 1) }}"
```

Rules, enforced at scaffold-load time (`ErrScaffoldComputedFieldInvalid`):

- `value:` is required on a `computed` field, and only valid on a `computed` field.
- `required:` and `default:` are both rejected on a `computed` field — it's always
  self-supplied, and `value:` already determines its value.
- A `computed` field may reference any regular field's answer, or an
  **earlier-declared** `computed` field's own result — computed fields are evaluated
  once, in `spec.fields[]` declaration order, after every regular field's answer is
  final (prompted, `--set`, or defaulted). A `computed` field that references a
  *later*-declared `computed` field simply sees no value at that point (same posture
  as an `options:` dot-path forward reference).
- Because computed fields are only evaluated after the interactive form completes, a
  regular field's `when:` cannot depend on a computed field's result — only the
  reverse (a computed field depending on a regular field) works.
- The resolved value lands in `.Config.<name>` exactly like any other field, so it's
  usable everywhere `.Config` is (file content, `target:`, `matrix:` axes, `options:`
  expressions, and other computed fields).

### `options:` — static, label/value, or dynamic

`options:` (select/multiselect only) accepts four shapes:

- A plain string list — label and value are the same:
  ```yaml
  options: [dev, staging, prod]
  ```
- A list of `{label, value}` objects — `value` is required (a non-empty string); `label` is
  optional and defaults to `value` when omitted. Only `value` ever reaches
  `answers`/templates/`when:` — `label` is presentation-only:
  ```yaml
  options:
    - label: Development
      value: dev
    - label: Production
      value: prod
  ```
- A dot-path string, `answers.<name>` — resolves against an earlier field's answer, a
  `spec.values` preset, or a `--set`-supplied value never declared as a field at all. Must resolve
  to a list-shaped value (a `multiselect` answer, or a structured `[]string`/`[]any` of strings).
  Mirrors `spec.files[].matrix` axis dot-path resolution exactly — same `answers.` prefix
  convention, same underlying resolver:
  ```yaml
  options: answers.envs
  ```
- A Go-template expression (recognized by containing the scaffold's configured left delimiter,
  `{{` by default) — computes the list from nested/structured answer data:
  ```yaml
  options: '{{ splitList "," answers.csv_field }}'
  ```

Both dynamic forms (dot-path and template expression) resolve correctly once the referenced
earlier field has been answered — interactively (fields prompt one at a time, so a later field is
only ever shown after the ones before it) or headlessly against `--set`/`--defaults`-supplied
values. Unlike
an earlier, now-removed load-time check, a dot-path's root name is **not** validated against field
declaration order at load time — load time can't distinguish a genuine forward/self-reference
mistake from a legitimate `spec.values` preset or `--set`-supplied value that was never declared
as a field at all. A forward reference simply resolves to no options at that point (disabling the
membership check for that field, not erroring); a self-reference is tautologically valid (the
field's own final answer is what gets checked against itself). See
`validateFieldOptionsSource`/`resolveFieldOptionsFromAnswers` in
`pkg/project/config/validation.go`.

**Label recovery**: when a *direct* single-segment dot-path (`answers.<name>`, not a deeper path
like `answers.nested.envs`, and not a template expression) sources from a field whose own
`options:` used `{label, value}` pairs, those labels are recovered for the filtered subset of
values present in the referenced answer — a value present in the answer but absent from the
source field's own options list falls back to `label == value` rather than erroring or being
dropped. Label recovery does not propagate through a chained dynamic reference (a dot-path field
sourcing from another dot-path field) or through the template-expression form — both always yield
`label == value`.

`when:` is evaluated by building one `huh.Group` per field (huh's `WithHideFunc`)
against a snapshot of every other field's current answer at render time — so a
`when:` can only meaningfully reference fields **declared earlier** in the `fields:`
list. In non-interactive mode (`--defaults`/no TTY), the same `when:` check gates
whether a hidden required field is treated as "missing" (`MissingRequiredValues`), so
`--set` doesn't need a value for a conditionally-hidden field.

## `spec.files[]` — conditional generation overlay

```yaml
- path: stacks/deploy/dev.yaml   # required; matched against the file's discovered path
  when: "'dev' in answers.environments"
```

Files not listed in `spec.files:` always generate (subject to the pre-existing
path-templating sentinel-skip behavior). This overlay does not declare *which* files
exist — the template's file tree does that; it only gates whether an already-discovered
file gets written.

`path:` may be a glob pattern (doublestar syntax) instead of a literal path, matched
against every discovered file:

```yaml
- path: "docs/legacy/**"          # *, ?, [...], ** (any depth incl. zero), {a,b}
  when: "answers.include_legacy_docs == true"
```

- Always forward slashes; a backslash in the pattern is normalized to `/` regardless
  of authoring/runtime OS (`pkg/utils.NormalizeGlobPattern`).
- A malformed pattern (unclosed `[`/`{`) fails scaffold load and
  `atmos scaffold validate` (`ErrScaffoldFilePathPatternInvalid`) — not just a silent
  permanent non-match at generation time.
- When multiple entries' `path:` match the same file, the **last** declared entry
  wins (`.gitignore`/`CODEOWNERS` precedence: write broad patterns first, specific
  overrides after — a wrong-order override is a silent no-op, not a load error).

### `spec.files[].matrix` — dynamic file generation

```yaml
- path: templates/deploy.yaml
  target: "deploy/{{ .matrix.environment }}/{{ .matrix.region }}.yaml"   # required when matrix is set
  matrix:
    environment: answers.environments                    # dot-path into an already list-shaped answer
    region: [us-east-1, us-west-2]                        # literal list
  when: "matrix.region in answers.environments[matrix.environment].regions"
```

Expands this one file entry into one generated file per resolved combination of every
axis's values (their full Cartesian product, sorted per axis for deterministic output)
— the same shape the workflow `matrix:` step uses. `target:` is required (a single
`path:` can't serve as more than one output) and rendered once per combination.

Each axis's value is one of:
- a literal list of strings
- a string starting with `answers.`, a dot-path into an already list-shaped answer
- a Go-template expression (any string containing `{{`) computing the list from
  nested/structured or free-text answer data (via `collectKeys`, `splitList`, or any
  other Sprig/Gomplate function — see `atmos-templates`)

`when:` gets a `matrix` CEL variable alongside `answers`, evaluated once per resolved
combination to prune ones that don't apply. The resolved combination is also available
as `.matrix.<axis>` in `target:` and the file's own rendered content.

When `path:` is a glob matching more than one file, `matrix:` is resolved once per
`path:` (cached, not recomputed per matched file — so a non-deterministic axis
expression, e.g. Sprig's `randAlphaNum`, still resolves the same value across every
matched file for one combination) and every matched file gets its own output per
combination. Two additional template variables, available in `target:` and content
alongside `.matrix.<axis>` regardless of whether `path:` is a glob:

- `.file.Path` — the currently matched file's own discovered path.
- `.file.RelPath` — `.file.Path` with the matching entry's glob literal prefix
  stripped (equal to `.file.Path` when `path:` has no glob metacharacter).

`target:` must reference `.file.Path` or `.file.RelPath` whenever its `path:` matches
more than one file, or every matched file renders to the same output path for a given
combination — checked deterministically before any file in the run is written
(`ErrScaffoldMatrixTargetMissingFileContext`), not only once a collision is reached
mid-run. `.file.*` is Go-template-only; it is not exposed to CEL `when:`.

## `spec.hooks` — step-backed hooks

```yaml
hooks:
  <hook-name>:
    events: [before.scaffold.generate, after.scaffold.generate]   # default: both
    kind: step            # or: steps
    when: "..."            # default (empty): success-only, mirroring stack-level hooks
    type: shell             # kind: step only — a registered step type
    with:                   # kind: step: one step's params; kind: steps: an ordered list
      command: "git add ."
```

This reuses `pkg/hooks.Hook` field-for-field (same struct as stack-level lifecycle
hooks) — see `atmos-hooks` for the full vocabulary. Only `kind: step`/`kind: steps` are
implemented for scaffold hooks; `command`/`store`/`git` kinds are stack/component-only
today (their execution engines assume `ExecContext.Info`, which scaffold generation
doesn't have).

`before.scaffold.generate` hooks run once, right after the form is filled in and before
any file is written — a failure aborts before any write happens (no rollback needed).
`after.scaffold.generate` hooks run once after the file loop completes and the project
record (`.atmos/scaffold.yaml`) is saved; they still get a chance to run on a failed
generation if their `when:` explicitly opts in (`when: always`/`when: failure`) — the
implicit-success default skips them on failure, matching stack-level hooks.

### Why `when:` can't use the `{all:/any:/not:}` map form here

`pkg/condition.Condition`'s Go type has no exported fields (by design — it's a small
AST wrapper), so when it's reflected into JSON Schema, invopop emits a schema with
`additionalProperties: false` alongside the `oneOf` branches. If `object` were included
as an allowed branch, a value like `when: {all: [ci, success]}` would satisfy the
`oneOf`'s object branch but then immediately fail the sibling `additionalProperties:
false` (since `all` isn't a declared JSON Schema property) — a real, confirmed
contradiction, not a hypothetical one. Scaffold's `when:` schema therefore only allows
`string` (predicate keyword or CEL expression) and `array` (implicit `all`) — use CEL's
`&&`/`||`/`!` operators for compound logic instead of the map form.

## `spec.values` — preset/default values

```yaml
values:
  cloud_provider: aws   # overrides a field's own `default:`, still overridden by --set
```

Precedence (lowest to highest): field `default:` → `spec.values` → `--set`/interactive
answer.
