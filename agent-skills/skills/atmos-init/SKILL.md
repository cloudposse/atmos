---
name: atmos-init
description: "Bootstrapping new Atmos projects with atmos init: built-in template catalog, differences from atmos scaffold generate, project record, and update-safe 3-way merge"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Init

Use this skill for bootstrapping a brand-new Atmos project from the built-in
template catalog via `atmos init`.

`atmos init` is a thin, project-bootstrap-scoped wrapper around the exact same
engine `atmos scaffold generate` uses (`pkg/generator/ui.InitUI`). For authoring a
*new* template — custom fields, conditional `when:` on fields/files, hooks — load
`atmos-scaffold` instead; everything documented there (field types, conditional
prompts/files, `spec.hooks:`, 3-way merge mechanics) applies unchanged to project
templates consumed via `init`. This skill only covers what's specific to `init`.

## Quick Shape

```shell
atmos init basic ./my-project
atmos init aws/landing-zone ./my-project
atmos init                          # interactive template + target selection
```

Built-in templates: `basic` (minimal cloud-agnostic layout), `simple`, `atmos`
(atmos.yaml only), `aws/app` (SDLC repo: dev/staging/prod, native CI, emulator-enabled),
`aws/landing-zone`, `gcp/landing-zone`, `azure/landing-zone`. Run `atmos scaffold list`
to see all templates, including remote/catalog ones not built in.

`atmos init` ships experimental — behavior may change between releases.

## Shared Template Contract

Every `init` template is an `AtmosScaffoldConfig`, so it uses the same contract
as `atmos scaffold generate`: `spec.fields`, `spec.files`, and `spec.hooks`.
Fields and files can use `when:` predicates or CEL expressions over earlier
`answers`; generation hooks run at `before.scaffold.generate` or
`after.scaffold.generate`. Scaffold hooks deliberately support only `kind: step`
and ordered `kind: steps`, even though stack lifecycle hooks support more kinds.

Load `atmos-scaffold` before authoring or changing any of those template
features. It contains the field, CEL, file-condition, hook, and template-context
rules that `init` consumes unchanged.

## Differences from `atmos scaffold generate`

| | `atmos init` | `atmos scaffold generate` |
|---|---|---|
| `--git` default | **true** (auto-creates initial commit) | **false** |
| `--dry-run` | not available | available |
| `--defaults` | not available (non-interactive detection only) | available |
| `list`/`validate` subcommands | none — use `atmos scaffold list`/`validate` | `scaffold list`/`scaffold validate` |
| Template sources | embedded + catalog (project templates) | embedded + custom (`atmos.yaml`) + catalog/remote |

Both share every other flag: `--force`, `--update`, `--base-ref`, `--interactive`/`-i`,
`--set key=value`, `--source-override` (env `ATMOS_INIT_SOURCE_OVERRIDE`, also accepts
`ATMOS_SCAFFOLD_SOURCE_OVERRIDE`), `--ref`, `--no-git`, `--merge-strategy`,
`--skip-hooks` (env `ATMOS_INIT_SKIP_HOOKS`) — same semantics as documented in
`atmos-scaffold`.

## Project Record

A successful `atmos init` writes `.atmos/scaffold.yaml` into the generated project —
the same `AtmosScaffoldConfig` manifest kind, now carrying the user's answers
(`spec.values`) and provenance (`spec.source`, `spec.baseRef`). This record makes later
`atmos init --update` re-runs against the same template possible without re-answering
every question, and is what `--update`'s 3-way merge uses as the base-ref default
source of truth for "what generated this."

## Updating an Existing Project

```shell
cd my-project
atmos init --update
atmos init --update --merge-strategy=theirs
```

Same 3-way merge mechanics as `atmos scaffold generate --update` — see
`atmos-scaffold`'s [references/merge-strategy.md](../atmos-scaffold/references/merge-strategy.md)
for the full mechanics (base storage, conflict strategies, the "offer to update instead
of failing" interactive prompt, shared verbatim between both commands).

## Routing

| Need | Skill |
|---|---|
| Template authoring: `spec.fields`, CEL `when:`, conditional files, step-backed hooks, full schema | `atmos-scaffold` |
| 3-way merge mechanics | `atmos-scaffold` → `references/merge-strategy.md` |
| Shared hook vocabulary | `atmos-hooks` |
| Project layout produced by a template (`atmos.yaml`, stacks, components) | `atmos-project-layout` |

## Guardrails

- `atmos init` is for *consuming* the built-in project-template catalog to bootstrap a
  new repo. Authoring a new template — even one you intend to use via `init` — is
  `atmos-scaffold` territory.
- `--git` defaults to **true** here (opposite of `scaffold generate`) — an accidental
  `atmos init` in an existing git repo will create an initial commit unless `--no-git`
  is passed.
