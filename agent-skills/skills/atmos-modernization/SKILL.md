---
name: atmos-modernization
description: "Atmos Modernization: migrate deprecated or legacy Atmos patterns to current names, Native CI, Atmos Pro drift detection, dependencies.components, name_template, and declared secrets"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Modernization

Use this skill when updating older Atmos projects to current conventions. "Atmos Modernization" is
the umbrella term for replacing legacy patterns with supported, current patterns.

## Modernization Checklist

| Legacy pattern | Replace with |
|---|---|
| `name_pattern` | `name_template` or explicit stack `name` |
| `settings.depends_on` | `dependencies.components` |
| `cloudposse/github-action-atmos*` wrapper actions | Native CI with direct `atmos` commands |
| `cloudposse/github-action-setup-atmos` as default | GitHub Actions container `ghcr.io/cloudposse/atmos:<version>` |
| `hashicorp/setup-terraform` / `opentofu/setup-opentofu` in Atmos jobs | Atmos `dependencies.tools` and toolchain |
| Manual `atmos toolchain install <tool>` preinstall steps for Atmos-owned tools | Declarative `dependencies.tools` at the owning component, workflow, hook, or custom command |
| Large inline workflow/custom-command shell scripts, repeated `echo`, shell loops, ad hoc sleeps | Native step types such as `atmos`, `toast`, `table`, `parallel`, `matrix`, `wait`, `container`, `emulator`, and `http` |
| Hand-rolled scheduled drift GitHub Actions | Atmos Pro drift detection |
| `cloudposse/github-action-atmos-terraform-drift-*` | `settings.pro.drift_detection` plus `atmos terraform plan --upload-status` |
| Secret values through raw store calls | Declared `secrets.vars` plus `!secret` |
| Legacy hook event spelling | Modern dotted lifecycle events such as `after.terraform.plan` |
| Static GitHub tokens in URLs | Atmos Auth `github/sts` through Atmos Pro |
| `cloudposse/terraform-aws-components` sources | Repos in the `cloudposse-terraform-components` organization |
| Parser-specific doubled double quotes (`""`) in `!terraform.state` or `!terraform.output` expressions | Direct YQ expressions; quote only as required by YAML |

## Process

1. Inspect current project behavior with `atmos describe stacks`, `atmos list components`, and
   `atmos validate stacks`.
2. Replace one class of legacy pattern at a time.
3. Preserve resolved stack output unless the modernization intentionally changes behavior.
4. Validate with `atmos describe component <component> -s <stack>` before changing CI.
5. Update CI last, after stack config and auth patterns are current.

## YAML Function Quoting

Older stack manifests may wrap an entire YQ expression in double quotes and double its inner
double quotes. That parser-specific escaping is obsolete: `!terraform.state` and
`!terraform.output` parse the component, optional stack, and remaining expression directly.

Replace legacy string-default quoting:

```yaml
# Before
username: !terraform.output config ".username // ""default-user"""

# After
username: !terraform.output config .username // "default-user"
```

Replace legacy string-concatenation quoting:

```yaml
# Before
postgres_url: !terraform.state aurora-postgres ".master_hostname | ""jdbc:postgresql://"" + . + "":5432/events"""

# After
postgres_url: !terraform.state 'aurora-postgres .master_hostname | "jdbc:postgresql://" + . + ":5432/events"'
```

The outer single quotes in the concatenation example are YAML quoting, not function-parser
escaping. Use YAML quoting only when the scalar requires it; for complete YAML and YQ quoting
guidance, see the [atmos-yaml-functions](../atmos-yaml-functions/SKILL.md) skill.

## Native CI Direction

New CI should run Atmos directly, preferably in the Atmos container image:

```yaml
jobs:
  plan:
    runs-on: ubuntu-latest
    container:
      image: ghcr.io/cloudposse/atmos:${{ vars.ATMOS_VERSION }}
    permissions:
      contents: read
      id-token: write
    steps:
      - uses: actions/checkout@v6
      - run: atmos terraform plan vpc -s prod
```

Use `atmos describe affected --format=matrix` for PR matrices and `atmos list instances
--format=matrix` for full estate operations.

## Drift Direction

Atmos Pro is the product path for drift detection. Enable drift per stack/component:

```yaml
settings:
  pro:
    enabled: true
    drift_detection:
      enabled: true
```

Then upload plan status:

```bash
atmos terraform plan vpc -s prod --upload-status
```

## Dependencies Direction

Use `dependencies.components` for component, file, and folder dependencies:

```yaml
dependencies:
  components:
    - component: vpc
    - component: dns-zone
      stack: plat-ue2-prod
    - kind: file
      path: configs/service.yaml
    - kind: folder
      path: src/lambda
```

Treat `settings.depends_on` as migration-only syntax.

## Component Source Repository Direction

Check `source`, `vendor.yaml`, `component.yaml`, Terraform module sources, and documentation
examples for `cloudposse/terraform-aws-components`. That monorepo is deprecated; components moved
to individual repositories in the `cloudposse-terraform-components` organization.

For example, migrate VPC references from the old monorepo form:

```yaml
source: "github.com/cloudposse/terraform-aws-components.git//modules/vpc?ref=1.450.0"
```

to the component repository form:

```yaml
source: "github.com/cloudposse-terraform-components/aws-vpc.git?ref=1.450.0"
```

Keep versions pinned when changing sources, and validate the target repository/tag exists before
updating production stacks.
