# Atmos Pro Skill Templates

Templates rendered by the `atmos-pro` skill when generating onboarding artifacts.

## Placeholder syntax

Templates use Go `text/template` with **custom `<<` / `>>` delimiters** to avoid collisions
with the `{{ }}` syntax that appears literally in:

- Atmos Pro workflow-dispatch inputs (e.g., `{{ .atmos_component }}`)
- GitHub Actions expressions (e.g., `${{ vars.ATMOS_VERSION }}`)
- Atmos vendor version interpolation (e.g., `{{ .Version }}`)

All native `{{ }}` in template output passes through untouched — no escaping needed.

### Scalar substitution

```
<<.org>>              → "cloudposse"
<<.repo>>             → "atmos"
<<.namespace>>        → "dev"
<<.target_org>>       → "dev"
<<.root_account_id>>  → "111111111111"
<<.probe_stack>>      → "dev-core-gbl-iam"
```

### Iteration (`range`)

```
<<range .accounts>>
  - tenant: <<.tenant>>
    stage: <<.stage>>
    account_id: <<.account_id>>
    is_root: <<.is_root>>
<<end>>
```

`$` inside a range refers to the top-level context, e.g., `<<$.namespace>>`.

### Conditional (`if`)

```
<<if .geodesic_detected>>
  # Geodesic-specific section.
<<end>>

<<if .is_root>>
  # Safety rail.
<<else>>
  # Normal path.
<<end>>
```

### Built-in functions

- `<<len .accounts>>` — number of accounts

## Renderer

Templates are rendered by `pkg/ai/skills/atmospro.Render`. See that package for the full data
context type (`RenderData`).

## Why custom delimiters?

Before adopting `<<` / `>>`, templates used standard `{{ }}` delimiters and escaped pass-through
`{{ }}` via backtick literals:

```
"{{ `{{ .atmos_component }}` }}"
```

This produced visual noise and was error-prone. Custom delimiters separate "template directives"
from "literal output containing braces" cleanly. The cost is that the templates are no longer
drop-in copies of the final artifacts — an AI agent or Go renderer must do one substitution pass
before writing.

## Template ↔ output mapping

| Template                                     | Output path                                             |
|----------------------------------------------|---------------------------------------------------------|
| `mixins/atmos-pro.yaml.tmpl`                 | `stacks/mixins/atmos-pro.yaml`                          |
| `catalog/iam-role-defaults.yaml.tmpl`        | `stacks/catalog/aws/iam-role/defaults.yaml`             |
| `catalog/iam-role-gha-tf.yaml.tmpl`          | `stacks/catalog/aws/iam-role/gha-tf.yaml`               |
| `component/iam-role-component.yaml.tmpl`     | `components/terraform/aws/iam-role/component.yaml`      |
| `profiles/github-plan.yaml.tmpl`             | `profiles/github-plan/atmos.yaml`                       |
| `profiles/github-apply.yaml.tmpl`            | `profiles/github-apply/atmos.yaml`                      |
| `workflows/atmos-pro.yaml.tmpl`              | `.github/workflows/atmos-pro.yaml`                      |
| `workflows/atmos-pro-list-instances.yaml.tmpl` | `.github/workflows/atmos-pro-list-instances.yaml`     |
| `workflows/atmos-terraform-plan.yaml.tmpl`   | `.github/workflows/atmos-terraform-plan.yaml`           |
| `workflows/atmos-terraform-apply.yaml.tmpl`  | `.github/workflows/atmos-terraform-apply.yaml`          |
| `workflows/oidc-test.yaml.tmpl`              | `.github/workflows/oidc-test.yaml`                      |
| `tfstate-backend-edit.yaml.tmpl`             | Merged into `stacks/catalog/tfstate-backend/defaults.yaml` |
| `docs/atmos-pro.md.tmpl`                     | `docs/atmos-pro.md`                                     |
| `docs/atmos-pro-pr-body.md.tmpl`             | `.github/PULL_REQUEST_BODY.md` (used by `gh pr create`) |

Templates with no `<<>>` directives (e.g., the four workflow files and the component vendor
manifest) are copied verbatim — the renderer is a no-op for those.
