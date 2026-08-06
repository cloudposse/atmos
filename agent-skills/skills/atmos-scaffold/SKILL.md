---
name: atmos-scaffold
description: "Scaffold templates: authoring scaffold.yaml, form fields (types, validation, conditional when:), conditional file generation, step-backed hooks (pre/post-generate), update-safe 3-way merge, and atmos scaffold generate/list/validate"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
references:
  - references/scaffold-yaml-schema.md
  - references/merge-strategy.md
---

# Atmos Scaffold

Use this skill for generating boilerplate (components, configs, directory structures)
from templates via `atmos scaffold generate`, for authoring new templates
(`scaffold.yaml`), and for updating previously-generated output from a changed
template via `--update`.

For bootstrapping a brand-new Atmos *project* from the built-in template catalog, load
`atmos-init` instead — it shares this exact engine but has its own command surface and
built-in template list.

## Quick Shape

```yaml
apiVersion: atmos/v1
kind: AtmosScaffoldConfig
metadata:
  name: terraform-component
  description: Standard Terraform component structure
spec:
  fields:
    - name: component_name
      label: Name of the component
      type: input
      required: true
```

```shell
atmos scaffold generate terraform-component ./components/terraform/vpc
atmos scaffold list
atmos scaffold validate ./components/terraform/vpc/scaffold.yaml
```

`atmos scaffold` ships experimental — behavior may change between releases.

## Creating a Template

A template is a directory containing `scaffold.yaml` (the questionnaire and
optional conditional-generation/hooks config) plus the files to generate.
**Files are auto-discovered by walking the template directory** — there is no
`files:` manifest listing every file (`spec.files:` exists only for the optional
conditional-generation overlay, see below).

Mark a file as a Go template (rendered with the collected answers) either by:
- Naming it with a `.tmpl` extension, or
- Adding an `atmos:template` magic comment in the first 10 lines, in the
  comment style matching the file type: `# atmos:template` (shell/YAML/Python),
  `// atmos:template` (Go/JS/C++), `/* atmos:template */` (C-style block),
  `<!-- atmos:template -->` (HTML/XML/Markdown)

Template sources: embedded (built into the Atmos binary), custom (declared under
`scaffold.templates` in `atmos.yaml`), or catalog/remote (git/https/s3 — advertised
as stubs, fetched on selection).

## Form Fields

`spec.fields` is an ordered questionnaire; fields prompt in the order declared.

| Type | Prompt widget |
|---|---|
| `input` / `text` / `string` | Free-form text (huh Input) |
| `select` | Single choice from `options:` |
| `multiselect` | Multiple choices from `options:` (filterable) |
| `confirm` / `bool` / `boolean` | Yes/no |

Common field keys: `name` (required, used as the template variable — access via
`{{ .Config.<name> }}`), `label`, `description`, `required`, `default`,
`options` (select/multiselect), `placeholder` (input), `validation.pattern`/`message`
(regex, input fields only).

### Conditional prompts (`when:`)

A field can declare `when:` to be shown only if a condition on **earlier-declared
fields'** answers holds true:

```yaml
spec:
  fields:
    - name: enable_monitoring
      type: confirm
      default: false
    - name: alert_email
      type: input
      when: "answers.enable_monitoring == true"   # only asked if confirmed above
```

`when:` accepts a predicate keyword (`always`, `never`, `ci`, `local`), a CEL string, or
a list (implicit `all`). Reference collected answers via the `answers` map — e.g.
`"'dev' in answers.environments"` for a multiselect, `"answers.x == true"` for a
confirm (a bare `answers.x` is **not** valid CEL here — it's typed `dyn`, not `bool`;
compare explicitly). Use CEL's `&&`/`||`/`!` for compound conditions — the
`{all:/any:/not:}` map form is not accepted for scaffold `when:` (see
[references/scaffold-yaml-schema.md](references/scaffold-yaml-schema.md) for why).
A `when:` can only see fields declared *before* it in the list.

Full field/validation reference: [references/scaffold-yaml-schema.md](references/scaffold-yaml-schema.md).

## Conditional File Generation

`spec.files:` is an optional overlay gating specific auto-discovered files, keyed by
their path in the template tree. Files not listed always generate.

```yaml
spec:
  files:
    - path: stacks/deploy/dev.yaml
      when: "'dev' in answers.environments"
    - path: stacks/deploy/staging.yaml
      when: "'staging' in answers.environments"
```

This is *static* gating over a fixed, enumerable set of files the template author
already created — not a loop generating an unbounded number of files per answer
(no `for_each:`; that's a future enhancement, see the PRD).

This is distinct from the older path-templating trick: if a file's *path itself* is a
Go template that renders to `""`, `"false"`, `"null"`, or `"<no value>"`, the engine
skips it too (`ShouldSkipFile`). Prefer declarative `when:` for new templates — it's
evaluated before any rendering and doesn't require crafting a path template.

## Hooks

`spec.hooks:` runs step-backed actions before/after generation, keyed by hook name,
reusing the **exact vocabulary stack-level lifecycle hooks use** — load `atmos-hooks`
for the full `events`/`kind`/`when`/`type`/`with` reference and `atmos-steps` for the
step types available in `with:`. Events are `before.scaffold.generate` and
`after.scaffold.generate`; a hook with no `events:` matches both.

```yaml
spec:
  hooks:
    git-add:
      events:
        - after.scaffold.generate
      kind: step
      type: shell
      when: "size(answers.environments) > 0"
      with:
        command: "git add ."
```

Only `kind: step`/`kind: steps` are supported today. Stack-level `command`, scanner,
`store`, `git`, and CI kinds require stack/component context that scaffold generation
does not have. `kind: step` takes one registered step type in `type:` and its payload
in `with:`; `kind: steps` takes an ordered `with:` list. Answers reach `when:` through
the `answers` CEL variable and reach step bodies through `{{ .Answers.<field> }}`
Go-template syntax.

**Security**: use `--skip-hooks` (skip all) or `--skip-hooks=name1,name2` (skip
specific hooks) to bypass hooks for a diagnostic or untrusted-template run — the same
flag semantics `terraform` already has. `ATMOS_SCAFFOLD_SKIP_HOOKS` is the matching
env var.

## Updating Existing Projects (3-Way Merge)

```shell
atmos scaffold generate my-template ./target --update
atmos scaffold generate my-template ./target --update --base-ref=v1.2.0
atmos scaffold generate my-template ./target --update --merge-strategy=theirs
atmos scaffold generate my-template ./target --update --dry-run
```

`--update` performs a real 3-way merge (base = the git ref the target was generated
from, defaulting to `HEAD`) instead of failing on a non-empty target directory.
`--merge-strategy` controls conflict resolution: `manual` (surface conflicts, default),
`ours` (keep your version), `theirs` (use the template's version). Full mechanics
(base storage, conflict markers, the "offer to update instead of failing" interactive
prompt): [references/merge-strategy.md](references/merge-strategy.md).

## Commands and Flags

`atmos scaffold generate [template] [target]`: `--force`, `--update`, `--base-ref`,
`--dry-run`, `--interactive`/`-i` (default true), `--defaults` (use defaults/`--set`
without prompting), `--set key=value` (repeatable), `--scaffold-source-override`,
`--ref` (git ref for a template source), `--git`/`--no-git` (default **false** — see
`atmos-init` for the opposite default), `--merge-strategy`, `--skip-hooks`.

`atmos scaffold list`: templates from `scaffold.templates` in `atmos.yaml` (plus
embedded/catalog). `atmos scaffold validate [path]`: validates `scaffold.yaml` against
the JSON Schema.

## Routing

| Need | Skill |
|---|---|
| Stack hook kinds, lifecycle events, envelope (`events`/`when`/`retry`/`on_failure`) | `atmos-hooks` |
| Every registered step type and aliases usable in a hook's `with:` | `atmos-steps` |
| Go-template/Gomplate/Sprig functions available in file content | `atmos-templates` |
| Project bootstrap from the built-in template catalog | `atmos-init` |
| Generated JSON Schema for IDE validation | `atmos-schemas` |
| `when:`/CEL syntax reference | `atmos-workflows` |

## Guardrails

- Prefer declarative `spec.fields[].when:`/`spec.files[].when:` over hand-rolled path
  templates or post-generation `sed`/shell cleanup.
- Keep destructive `post_generate` hooks (deleting files, force-pushing, etc.) opt-in
  and visible in `scaffold.yaml`, mirroring `atmos-hooks`' guidance for stack hooks.
- A `when:` can only reference fields/files declared earlier — referencing a
  not-yet-declared field silently sees its zero value, not an error; order fields
  deliberately.
- Don't confuse the path-sentinel skip trick with declarative `when:` — use `when:`
  for new templates; the sentinel trick remains for backward compatibility.
