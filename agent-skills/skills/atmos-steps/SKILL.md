---
name: atmos-steps
description: "Shared Atmos step DSL for workflows, custom commands, hooks, and cast recordings: step types, env, output, working_directory, retry, and native alternatives to shell glue"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Steps

Atmos steps are the shared execution DSL used by workflows, custom commands,
hooks (`kind: step`), and cast recordings. When a task involves `steps:`, step
`type`, `working_directory`, `env`, `output`, retries, or hook `with:` payloads,
use this skill together with the surface-specific skill (`atmos-workflows`,
`atmos-custom-commands`, or `atmos-hooks`).

## Core Model

A step is a typed action with native fields. Do not treat steps as a place to
write shell scripts by default. Prefer the Atmos field that expresses the
operation directly:

- Use `working_directory` instead of `cd`.
- Use `env` maps instead of inline `FOO=bar command` or `export`.
- Use `output: none` instead of redirecting to `/dev/null`.
- Use `type: script` instead of heredoc shell snippets like `python3 - <<'PY'`.
- Use `type: workdir` with `source` and `reset` instead of `mkdir`, `rm -rf`,
  and `cp`.
- Use `type: atmos` for Atmos commands instead of shelling out to `atmos ...`
  when the surface supports typed steps.
- Use Atmos Terraform/OpenTofu `rc` configuration instead of hand-managing
  temporary `.terraformrc` or `.tofurc` files.

Shell is still valid for real shell work, but it should be a conscious fallback
when no native step type or field expresses the intent.

## Where Steps Run

Step fields are shared, but defaults differ by surface:

- Workflows default command steps to `type: atmos`.
- Custom command string steps historically behave like shell commands; use
  structured step objects when you need typed behavior.
- Hooks use an envelope plus `with:`. For `kind: step`, the hook's `type`
  selects the step type, while `with:` contains the step-specific fields.
- Cast steps can run nested steps and record their terminal output.

Always load the surface-specific skill for invocation rules, path anchoring, and
template context.

## Common Fields

Most typed steps can use these fields when the step type supports them:

```yaml
steps:
  - name: validate
    type: script
    interpreter: python3
    working_directory: !repo-root .
    env:
      ATMOS_LOGS_LEVEL: warn
    output: none
    retry:
      max_attempts: 3
      delay: 2s
    script: |
      print("ok")
```

Important shared fields:

- `name`: Stable step id for logs, dependencies, outputs, and resume behavior.
- `type`: Step handler (`atmos`, `shell`, `script`, `parallel`, `http`, etc.).
- `command`: Command text for command-running steps.
- `script` and `interpreter`: Inline script body and runtime for `type: script`.
- `working_directory`: Directory for the subprocess or script.
- `env`: Map of environment variables layered onto the step.
- `output`: Output mode: `raw`, `log`, `viewport`, or `none`.
- `retry`: Retry policy around the whole step.
- `identity`: Atmos identity used when the step runs.
- `when`: Declarative condition for whether the step runs.
- `needs`: Dependency names for concurrent control steps.
- `timeout`: Duration limit for supported steps.
- `tty` and `interactive`: Terminal handoff for commands that need it.

## Step Types

Use [atmos.tools/workflows/steps/type](https://atmos.tools/workflows/steps/type) and its
type-specific subpages (e.g. `atmos.tools/workflows/steps/type/shell`) as the canonical
reference. Current step families include:

- Command and integration: `atmos`, `shell`, `script`, `exec`, `container`,
  `http`/`webhook`, `require`.
- Orchestration: `parallel`, `matrix`, `wait`, `wait-all`, `cancel`, `sleep`,
  `exit`.
- Interactive: `input`, `confirm`, `choose`, `filter`, `file`, `write`.
- UI and output: `toast`, `markdown`, `spin`, `table`, `pager`, `format`,
  `join`, `style`, `log`, `alert`, `say`, `title`, `clear`, `linebreak`,
  `stage`.
- Workspace and recording: `workdir`, `cast`.

## Environment

Prefer map syntax:

```yaml
env:
  PATH: '{{ env "PWD" }}/../../.context/bin:{{ env "PATH" }}'
  ATMOS_LOGS_LEVEL: warn
```

For custom commands, command-level `env` supports the same map style. Use
template expressions such as `{{ env "PATH" }}` or `.Env` where supported. Use
`valueCommand` only when the value genuinely must come from a command's stdout;
do not move shell string building into `valueCommand`.

## Working Directory

Use `working_directory` at the narrowest useful scope:

```yaml
steps:
  - name: docs
    type: shell
    working_directory: !repo-root .
    command: npm run docs:build
```

Do not use `--chdir` or `cd` when a workflow, custom command, or step
`working_directory` can express the same thing. Relative paths must be checked
against the surface's base path rules.

## Output

Use output modes instead of pipe redirection:

```yaml
steps:
  - name: prepare
    type: atmos
    command: terraform generate varfile vpc -s dev
    output: none
```

`output: none` is for quiet setup. `raw` preserves command output, `log` routes
through Atmos logging, and `viewport` is for richer terminal display.

## Script Steps

Use `type: script` for inline scripts:

```yaml
steps:
  - name: validate-cast
    type: script
    interpreter: python3
    script: |
      from pathlib import Path

      text = Path("path/to/your.cast").read_text()
      if "All proofs passed" not in text:
          raise SystemExit("cast validation failed")
```

Do not put `command` on a `script` step. The schema requires `interpreter` and
`script`.

## Workdir Steps

Use `type: workdir` for repeatable scratch directories:

```yaml
steps:
  - name: stage-fixtures
    type: workdir
    path: .context/casts/demo
    source: demo/casts/fixtures
    reset: true
```

This replaces shell sequences that create, delete, and copy directories.

## Hooks

For hooks, the hook envelope controls lifecycle behavior and `with:` is the
step payload:

```yaml
hooks:
  notify:
    kind: step
    type: http
    events: [after.terraform.apply]
    on_failure: warn
    retry:
      max_attempts: 3
    with:
      url: https://example.com/hook
      method: POST
```

Use `atmos-hooks` for hook events, outcome conditions, `on_failure`, and
preflight behavior.

## Verification

When changing step YAML, verify the user-facing command from the directory where
users actually run Atmos. Prefer `working_directory` fields in YAML and command
tool `workdir` settings in tests over shell `cd` or `--chdir`.
