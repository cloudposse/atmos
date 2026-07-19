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
  type: input                 # input|text|string|select|multiselect|confirm|bool|boolean
  label: Component name       # short prompt label (falls back to name if omitted)
  description: ...            # longer help text shown with the prompt
  required: true
  default: my-default
  options: [a, b, c]          # select/multiselect only
  placeholder: e.g. vpc       # input fields only
  validation:
    pattern: '^[a-z][a-z0-9-]*$'
    message: "Must be lowercase alphanumeric with hyphens"
  when: "answers.some_earlier_field == true"   # optional; gates whether this field is shown
```

Field name uniqueness is enforced — a duplicate `name` fails to load
(`ErrDuplicateScaffoldFieldName`) rather than silently dropping an answer.

`when:` is evaluated by building one `huh.Group` per field (huh's `WithHideFunc`)
against a live snapshot of every other field's current answer at render time — so a
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
