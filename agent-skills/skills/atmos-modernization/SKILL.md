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
| Hand-rolled scheduled drift GitHub Actions | Atmos Pro drift detection |
| `cloudposse/github-action-atmos-terraform-drift-*` | `settings.pro.drift_detection` plus `terraform plan --upload-status` |
| Secret values through raw store calls | Declared `secrets.vars` plus `!secret` |
| Legacy hook event spelling | Modern dotted lifecycle events such as `after.terraform.plan` |
| Static GitHub tokens in URLs | Atmos Auth `github/sts` through Atmos Pro |

## Process

1. Inspect current project behavior with `atmos describe stacks`, `atmos list components`, and
   `atmos validate stacks`.
2. Replace one class of legacy pattern at a time.
3. Preserve resolved stack output unless the modernization intentionally changes behavior.
4. Validate with `atmos describe component <component> -s <stack>` before changing CI.
5. Update CI last, after stack config and auth patterns are current.

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
