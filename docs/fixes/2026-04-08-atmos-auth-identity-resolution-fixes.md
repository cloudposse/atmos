# Fix: Atmos Auth identity resolution — three related bugs

**Date:** 2026-04-08
**Branch:** `aknysh/atmos-auth-fixes-2`
**Issues:**
- [#2293](https://github.com/cloudposse/atmos/issues/2293) — `auth.identities.<name>.default: true` in imported stack files not recognized during identity resolution
- [Discussion #122](https://github.com/orgs/cloudposse/discussions/122) — Auth inheritance not scoping to stack (a default identity declared in one stack manifest leaks to every other stack across all OUs)
- Slack report — `components.terraform.<name>.auth.identity` override at the component level is silently ignored; the default identity is used instead

## Status

**Investigation only — no code changes yet.** This document describes the three
reported problems and their reproductions. Root-cause analysis and proposed
fixes will land in follow-up edits after the code walkthrough.

---

## Issue 1 — Default identity declared in an imported stack file is not recognized

**Source:** [#2293](https://github.com/cloudposse/atmos/issues/2293)

### Problem

When `auth.identities.<name>.default: true` is declared in an imported stack
file (for example `_defaults.yaml` that a stack manifest imports via
`import:`), Atmos does not pick it up during identity resolution. Instead,
Atmos prompts the user to select an identity interactively even though a
default is configured — and in non-interactive contexts this surfaces as
"no default identity configured."

The same identity resolves correctly when its `auth:` block is placed
**directly** in the top-level stack manifest rather than in an imported
defaults file.

### Reproduction

Defaults file declaring the default identity:

```yaml
# stacks/orgs/acme/dev/_defaults.yaml
import:
  - ../_defaults

vars:
  stage: dev

auth:
  identities:
    acme-dev:
      default: true
```

Stack manifest that imports it:

```yaml
# stacks/orgs/acme/dev/us-east-1/foundation.yaml
import:
  - ../_defaults
  - mixins/region/us-east-1
```

Running any component command in that stack:

```bash
$ atmos terraform plan my-component -s acme-dev-us-east-1

No default identity configured. Please choose an identity:
> acme-dev
  core-root
  ...
```

Debug output confirms the imported `auth:` block is invisible to auth
resolution:

```text
DEBU  Loading stack configs for auth identity defaults
DEBU  Loading stack files for auth defaults count=16
DEBU  No default identities found in stack configs
```

### Expected behavior

Atmos should resolve the default identity from the **merged** stack config,
honoring the same `import:` / `_defaults.yaml` inheritance semantics that
`vars:` and `components:` already obey. Placing `auth:` in a defaults file
should not require duplication in every manifest that imports it.

### Current workaround

Duplicate the `auth:` block in every top-level stack manifest rather than
declaring it once in `_defaults.yaml`. This works but defeats the purpose
of Atmos's import-based inheritance model.

### Related links called out in the issue

`#1950`, `#2071`, `#2081`, `#2125`, and PR `#1865` — flagged as the same
root-cause class.

---

## Issue 2 — Default identity declared in one stack manifest leaks to all stacks across all OUs

**Source:** [Discussion #122 — "Auth inheritance not scoping to stack"](https://github.com/orgs/cloudposse/discussions/122)

### Problem

When a stack manifest declares:

```yaml
auth:
  identities:
    <org>-<tenant>/terraform:
      default: true
```

…that `default: true` assignment is treated as **global** rather than
scoped to the stack that declared it. Every subsequent `atmos terraform
plan/apply` invocation — regardless of which stack the user selects —
loads that same identity as the default, even for stacks in a completely
different OU, tenant, or environment.

The user tested this across Atmos `1.210`, `1.211`, and `1.213`; the
behavior reproduces on all three.

### Reproduction

1. Stack tree with multiple OUs / tenants, e.g.

   ```text
   stacks/orgs/gold/
     data/staging/us-east-1/monitoring-agent.yaml
     plat/staging/us-east-1/eks-cluster.yaml
     plat/prod/us-east-1/eks-cluster.yaml
   ```

2. Add a default-identity `auth:` block to **one** manifest, e.g.

   ```yaml
   # stacks/orgs/gold/data/staging/us-east-1/monitoring-agent.yaml
   auth:
     identities:
       data-staging/terraform:
         default: true
   ```

3. Run any terraform command against a **different** stack:

   ```bash
   $ atmos terraform plan eks/test-eks-agent -s plat-use1-staging
   ```

4. Atmos loads the `data-staging/terraform` identity for the
   `plat-use1-staging` stack command. Debug output (trimmed):

   ```text
   Found component 'eks/test-eks-agent' in the stack 'plat-use1-staging'
     in the stack manifest 'orgs/gold/plat/staging/us-east-1/monitoring-test'
   CreateAndAuthenticateManager called identityName="" hasAuthConfig=true
   Loading stack configs for auth identity defaults
   Loading stack files for auth defaults count=284
   Found default identity in stack config identity=data-staging/terraform
     file=/…/stacks/orgs/gold/data/staging/us-east-1/monitoring-test.yaml
   ```

   The file Atmos picks the default from belongs to a completely
   unrelated stack (`data-staging` vs the requested `plat-use1-staging`).

### Expected behavior

`default: true` under `auth.identities.<name>` should only apply to the
stack(s) that actually import or declare that `auth:` block. Unrelated
stacks in other OUs, tenants, or environments should be unaffected.

### Probable relationship to Issue #1

Cloud Posse already suggested in the discussion comments that this may
share a root cause with [#2293](https://github.com/cloudposse/atmos/issues/2293)
— both reports describe the auth-identity resolver processing stack
files without honoring stack scoping or import inheritance. Confirming
or refuting the shared root cause is part of the investigation that
follows.

---

## Issue 3 — Component-level `auth.identity` override is silently ignored

**Source:** User report (Slack)

### Problem

A stack has a **default identity** that is correctly used to read and write
the Terraform backend state (the backend role). An individual component in
that stack specifies a **different** identity via
`components.terraform.<name>.auth.identity` because the component needs to
create resources in a different AWS account.

Atmos authenticates successfully as the **default** identity during
`atmos auth login` / `atmos auth whoami` — as expected. But when running
`atmos terraform apply` for that component, Atmos **continues to use the
default identity** for provider-level AWS calls (resource creation)
instead of switching to the component-level override. Terraform then
fails when the default identity's role lacks permission to create the
target resource in the target account.

In short: the component-level `auth.identity` override has no effect on
the AWS provider credentials Terraform sees at apply time.

### Reproduction

Global auth config (`atmos.yaml`):

```yaml
auth:
  providers:
    corp-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://example.awsapps.com/start
      auto_provision_identities: true

  identities:
    tenant-a:
      default: true
      kind: aws/permission-set
      via.provider: corp-sso
      principal:
        name: role-for-tf-state
        account.id: "111111111111"

    tenant-shared/role-for-create-resources:
      kind: aws/permission-set
      via.provider: corp-sso
      principal:
        name: role-for-create-resource
        account.id: "222222222222"
```

Stack manifest for an S3 bucket component:

```yaml
# stacks/…/s3bucket-dev-ue1.yaml
import:
  - catalog/infra/s3-bucket
  - mixins/env/dev
  - mixins/region/us-east-1

components:
  terraform:
    s3-bucket:
      auth:
        identity: tenant-shared/role-for-create-resources
```

Org-level backend defaults (`_defaults.yaml`):

```yaml
terraform:
  backend_type: s3
  backend:
    s3:
      bucket: atmos-tf-state
      key: terraform.tfstate
      region: us-east-1
      role_arn: "arn:aws:iam::111111111111:role/role-for-tf-state"
      dynamodb_table: atmos-tf-state-lock
      encrypt: true
```

Observed behavior:

```bash
$ atmos auth login
$ atmos auth whoami
# -> assumed role-for-tf-state in account 111111111111   (correct default identity)

$ atmos terraform apply s3-bucket -s s3bucket-dev-ue1
# Error: arn:aws:iam::111111111111:role/role-for-tf-state
#        is not allowed to perform s3:CreateBucket action
```

### Expected behavior

At apply time, the AWS provider used by Terraform should be credentialed
with the component-level identity (`tenant-shared/role-for-create-resources`,
account `222222222222`) — **not** the default identity
(`tenant-a`, account `111111111111`). The backend role, which is used for
state read/write, should still be the default identity as configured in
the backend block.

In other words: Atmos should support **two separate credential contexts**
in one command — one for the Terraform backend (state), one for the
Terraform provider (resource operations) — with the component-level
`auth.identity` override controlling the latter.

---

## Next steps

1. Walk through the auth-identity resolution code path
   (`CreateAndAuthenticateManager`, stack-config loading for auth
   defaults, component-level auth override handling in
   `ExecuteTerraform` / the credential export path).
2. Confirm whether Issues 1 and 2 share a single root cause (Cloud
   Posse's working theory) or are two independent bugs that merely
   surface in the same log line.
3. For Issue 3, trace how `components.terraform.<name>.auth.identity`
   propagates (or fails to propagate) from the stack config into the
   environment variables Terraform sees, and compare it to how the
   backend `role_arn` is passed through.
4. Propose and land the fix(es), with regression tests for each
   reported scenario.
