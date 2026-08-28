# Project Validation Reference

Use this reference when validating a repository or designing a validation CI job. It complements
the component-policy and generic JSON Schema references.

## Validator Matrix

| Validator | Commands | Scope |
|---|---|---|
| Aggregate project validation | `atmos validate` | Runs each applicable project validator and reports all results |
| Atmos configuration | `atmos validate config`, `atmos config validate` | Root `atmos.yaml` and its configured schema |
| Stack manifests | `atmos validate stacks`, `atmos stack validate` | Stack YAML syntax, imports, duplicate components, and manifest schema |
| Generic JSON Schema | `atmos validate schema <key>` | Files selected by `schemas.<key>.matches` against `schemas.<key>.schema` |
| EditorConfig | `atmos validate editorconfig` | Files governed by `.editorconfig` |
| GitHub Actions | `atmos ci validate`, `atmos validate ci` | Workflow files through actionlint |
| Component policies | `atmos validate component <component> -s <stack>` | Resolved component configuration through JSON Schema and OPA/Rego |

Use `atmos validate schema <key>` for any JSON Schema-compatible project input, not only Atmos
configuration. For example, a `github-actions` schema entry can validate workflow YAML, while
`atmos ci validate` additionally runs actionlint's workflow-specific semantic checks.

## Aggregate Validation

Run the project gate from the repository root:

```shell
atmos validate --format rich
```

The aggregate command records a result for every applicable validator. Prefer it over a shell loop
so CI gives one coherent outcome and one native-CI job summary.

## Affected Inputs

Use `--affected` to validate the repository-relative inputs changed since the merge base:

```shell
atmos validate --affected --base origin/main --format rich
atmos config validate --affected
atmos stack validate --affected
atmos validate schema github-actions --affected
atmos validate ci --affected
```

In a GitHub pull-request event, Atmos can use the event's base revision. Local and non-PR CI runs
can specify `--base`. Uncommitted and untracked files are included in the affected calculation.

Some changes intentionally broaden selection: a validation configuration or schema change can
require all matching files, and a stack or workflow configuration change can require validating
the relevant full set. Treat an affected result as a safe validation scope, not a promise that the
number of files will always be small.

## Excluding Test-only Inputs

`--exclude` is repeatable and accepts slash-separated repository glob patterns. It works in both
all-input and affected-input modes for aggregate, config, stacks, generic-schema, and GitHub
Actions validation:

```shell
atmos validate --exclude 'tests/fixtures/**' --format rich
atmos validate --affected --exclude 'tests/fixtures/**' --format rich
atmos validate schema github-actions --exclude 'tests/fixtures/**'
atmos validate ci --affected --exclude 'tests/fixtures/**'
```

Use it for fixtures that deliberately violate a schema or actionlint rule. Keep a dedicated
negative test for those fixtures so excluding them from the normal production gate does not remove
their coverage.

The direct EditorConfig command retains its pre-existing regex `--exclude` option. Its value is
not a repository glob. When the aggregate command is used, its generic glob exclusions are applied
before EditorConfig is invoked.

## Native CI Output

Configure Atmos-owned CI reporting in `atmos.yaml`:

```yaml
ci:
  enabled: true
  annotations:
    enabled: true
```

With `ci.enabled: true`, aggregate validation writes a GitHub Actions job summary listing each
validator as passed, skipped, or failed. The green job and that summary communicate successful
validation; success does not produce noisy source annotations.

Validators that support source locations emit native GitHub Actions annotations only for findings
when annotations are enabled. For example, `atmos ci validate --format rich` produces actionlint
diagnostics and annotations for invalid workflow YAML. `--format sarif` produces SARIF output
without native annotation side effects so another uploader can own reporting.

Use a separate, path-scoped E2E workflow to exercise expected invalid fixtures and assert their
annotations. Do not make the normal production validation job fail on intentionally broken test
inputs.
