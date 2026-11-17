# PRD: Hooks, Lifecycle Events, and Policy Support for `atmos docs generate`

**Status**: Proposed

**Related PRDs**: [Custom Hooks](./custom-hooks.md) | [Hook step types](./hooks-step-types.md) | [Shared Policy Runtime Context](./policy-runtime-context.md)

## Problem and scope

Teams generate documentation with `atmos docs generate` (a README, release notes, component
reference pages) and then need that output somewhere else: a wiki, a documentation portal, an
artifact bucket, a chat announcement. Today that last step lives outside Atmos, in a shell
wrapper or a CI job that must remember to run after the generator and must know where the
output landed.

Lifecycle hooks already solve this for component operations. Documentation generation is a
repository-level operation, not a component operation, so it has no events, no place to declare
hooks, and no way for a policy to require or forbid a follow-up action.

This PRD defines how documentation generation joins the existing hook and policy system. It
adds no publishing feature for any specific destination. Publishing to a wiki is a generic
`http` step against that wiki's REST API, using the same step library available in workflows
and component hooks.

## Goals

1. Run any existing hook kind (`step`, `steps`, `command`) before and after `atmos docs generate`.
2. Declare hooks next to the generator they belong to, so each document type owns its follow-up.
3. Give hook steps the facts they need (which document, where it was written) without parsing
    configuration.
4. Let governing policies select hooks for documentation generation the same way they do for
    component operations.
5. Keep behavior familiar: the same fields, `when:` predicates, `on_failure` handling, and
    `--skip-hooks` semantics as component hooks.

## Non-goals

- A built-in integration for any particular documentation destination.
- Hooks for repository-level operations other than documentation generation. Other operations
  may adopt the same mechanism later; this PRD does not commit to them.
- A new hook kind. Publishing uses `kind: step` with the existing `http` step type, or
  `kind: command`.
- Hooks that change what gets generated. Hooks run around generation; they do not alter the
  template, inputs, or output path.

## User-facing behavior

### Events

Two lifecycle events are registered:

| Event | When it fires |
|---|---|
| `before.docs.generate` | After the generator is selected and validated, before any input is read or any file is written. |
| `after.docs.generate` | After generation finishes, with the operation's outcome (success or failure). |

Both follow the same event-name validation and [edition](./editions.md) gating as every other
registered hook event. An unregistered event name in a hook declaration is an error.

### Declaring hooks

Hooks are declared under the generator they belong to, in `atmos.yaml`:

```yaml
docs:
  generate:
    readme:
      base-dir: .
      input:
        - "./README.yaml"
      template: "./README.md.gotmpl"
      output: "./README.md"
      hooks:
        publish-wiki:
          kind: step
          type: http
          events: [after.docs.generate]
          when: success
          on_failure: fail
          with:
            url: https://wiki.example.com/rest/api/content/12345
            method: PUT
            headers:
              Authorization: "Bearer {{ .env.WIKI_TOKEN }}"
              Content-Type: application/json
            body: '{"title": "README", "body": "see attached"}'
```

Each entry under `hooks` has the same shape as a component hook: `kind` (`step`, `steps`, or
`command`), `events`, `when`, `on_failure`, and the kind's own fields. Hooks belong to one
generator; a hook under `readme` never runs for `release-notes`.

A hook with no `events` list fires on both documentation events, matching component hook
behavior.

### Ordering and failure

1. `before.docs.generate` hooks run first. If one fails with `on_failure: fail` (the default) or a policy denies it, generation is aborted and no output
    file is written or modified.
2. Generation runs.
3. `after.docs.generate` hooks run with the outcome of generation. Each hook's `when:` decides
    whether it runs: `success` (the default), `failure`, or `always`.
4. If generation succeeded and an after-hook fails, the command fails according to that hook's
    `on_failure` setting (`fail` by default; `warn` and `ignore` keep the command successful).
5. If generation failed, the command reports the generation error. After-hooks selected by
    `when: failure` or `when: always` still run.

Within one event, hooks run in a deterministic order (sorted by hook name). Hooks run with the
process working directory (where `atmos docs generate` starts, or the `--chdir` target), not
`base-dir`;
`ATMOS_DOCS_BASE_DIR` and `ATMOS_DOCS_OUTPUT_PATH` locate the generator's files.

### Context available to steps

Hook steps receive these environment variables:

| Variable | Value |
|---|---|
| `ATMOS_DOCS_TYPE` | The generator name, such as `readme`. |
| `ATMOS_DOCS_OUTPUT_PATH` | The resolved path of the output file. |
| `ATMOS_DOCS_BASE_DIR` | The resolved base directory of the generator. |

The same values are available to step templates as `{{ .docs.type }}`,
`{{ .docs.output_path }}`, and `{{ .docs.base_dir }}`. After-hooks also see the operation
outcome through the existing `{{ .status }}` and `{{ .error }}` values and `ATMOS_HOOK_*`
variables. Documentation hooks have no infrastructure component and no stack: the stack is empty and
`ATMOS_COMPONENT` holds the generator name (for example `readme`).

### Skipping hooks

`--skip-hooks` and `ATMOS_SKIP_HOOKS` work as they do for component commands: skip all hooks
for one invocation, or name specific hooks. Hooks required by a policy cannot be skipped.

### Policy support

A governing policy that uses `on: operation` with `action: hooks` applies to documentation
generation. The operation input is:

| Field | Value |
|---|---|
| `event` | `before.docs.generate` or `after.docs.generate` |
| `component_type` | `docs` |
| `command` | `generate` |
| `component` | The generator name, such as `readme` |
| `stack` | Empty; documentation generation has no stack |
| `status` | `success` or `failure`; before-events begin with `success` |

A policy can therefore require a hook for a given generator, restrict a hook to one
generator, or deny generation outright with a policy rule on the before-event. Policy hook
selection, `required`, and `require_success` behave exactly as for component operations.

## Success criteria

- A `kind: step` hook with `type: http` under `docs.generate.<type>.hooks` runs after a
  successful `atmos docs generate <type>` and can read the output path from
  `ATMOS_DOCS_OUTPUT_PATH`.
- A failing `before.docs.generate` hook leaves the output file untouched.
- A `when: failure` after-hook runs when generation fails and does not run when it succeeds.
- `--skip-hooks` suppresses non-required documentation hooks and does not suppress policy-required ones.
- A policy rule on `before.docs.generate` can block generation for a named generator.
- A hook declared with an unregistered event name is rejected when configuration loads.
- The `atmos.yaml` JSON schema accepts `hooks` under each generator.

## Documentation

- `atmos docs generate` command page: a Hooks section with the `http` step example, the
  environment variables, and the policy note.
- `docs.generate` configuration page: the `hooks` option.
- Hooks reference: the new events in the supported events table.
- Policy documentation: `docs` as a component type in the operation input.
- A changelog post and a roadmap entry.
