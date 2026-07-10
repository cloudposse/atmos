---
name: atmos-imports
description: "Atmos imports: local and remote stack imports, go-getter schemes, templated imports, import context, remote imports vs component source/vendoring, and private GitHub imports through github/sts"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos Imports

Use this skill for stack manifest `import:` behavior, especially remote imports. Imports pull
stack configuration, not component source code.

## Related Skills

| Need | Load |
|---|---|
| General stack structure and merge precedence | [atmos-stacks](../atmos-stacks/SKILL.md) |
| Component source provisioning and vendoring | [atmos-vendoring](../atmos-vendoring/SKILL.md) |
| Private GitHub auth with Atmos Pro STS | [atmos-auth](../atmos-auth/SKILL.md) |
| Migration from old import/name patterns | [atmos-modernization](../atmos-modernization/SKILL.md) |

## Import Basics

```yaml
import:
  - catalog/vpc/defaults
  - mixins/region/us-east-2
  - orgs/acme/plat/prod/_defaults
```

Atmos resolves imports before producing the final stack. Use imports for `_defaults.yaml`, mixins,
catalog defaults, reusable stack fragments, and remote stack configuration.

If both `file.yaml` and `file.yaml.tmpl` exist, the template version is preferred. Template imports
can receive context and render before merge.

## Remote Imports

Remote imports use the same stack `import:` key:

```yaml
import:
  - https://raw.githubusercontent.com/acme/config/main/stacks/catalog/vpc.yaml
  - github://acme/config/main/stacks/catalog/eks.yaml
  - git::https://github.com/acme/config.git//stacks/catalog/db.yaml?ref=v1.2.3
  - s3::https://s3.amazonaws.com/acme-configs/stacks/catalog/vpc.yaml
  - gcs::gs://acme-configs/stacks/catalog/eks.yaml
  - oci://ghcr.io/acme/atmos-config:v1.2.3
```

Use pinned refs or immutable artifact tags for production imports. Prefer HTTPS/GitHub forms for
read-only public config and Git/OCI forms when versioning, auth, or packaged artifacts matter.

## Import vs Source vs Vendoring

- `import:` pulls stack YAML configuration.
- Component `source:` provisions component source code.
- `vendor.yaml` plus `atmos vendor pull` materializes external files into the repository.

Remote imports do not create `components/terraform/<name>` directories. If a remote imported stack
references a component whose source is not local, also configure component `source:` or vendor the
component with `atmos vendor pull`.

Use `atmos vendor pull` to cache important remote imports/components locally for offline access,
repeatability, and faster CI.

## Private GitHub Imports

For private GitHub remote imports in CI, prefer Atmos Pro `github/sts`:

```yaml
auth:
  providers:
    atmos-pro:
      kind: atmos/pro
      spec:
        workspace_id: !env ATMOS_PRO_WORKSPACE_ID
  identities:
    atmos-pro:
      kind: atmos/pro
      via:
        provider: atmos-pro
  integrations:
    github-sts:
      kind: github/sts
      via:
        provider: atmos-pro
      spec:
        auto_provision: true
```

In CI, `github/sts` can lazily mint GitHub tokens before the first private remote read, including
`atmos vendor pull`, component `source:`, remote `import:`, and private Terraform module fetches.

## Debugging

- Use `atmos describe stacks --stack <stack>` to verify imported and merged output.
- Use `atmos validate stacks` before relying on generated stack names or inherited component config.
- If a remote import works but Terraform cannot find a component, check component source/vendoring.
- If a private import fails in CI, check `id-token: write`, `atmos/pro`, `github/sts`, and repository
  trust policy in Atmos Pro.
