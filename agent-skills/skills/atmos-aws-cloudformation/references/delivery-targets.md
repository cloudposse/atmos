# Delivery Targets, Backend Management, and Stack Sets

Detailed reference for `atmos-aws-cloudformation`'s `provision.targets` mechanics. See the main
[SKILL.md](../SKILL.md) for the component shape overview.

## Delivery Targets (Backend Management)

By default `apply`/`deploy` deploy directly to the account/region resolved for the component (see
Region Resolution in the main skill doc) — the implicit target when `--target` is omitted and no
`provision.default` is set. A component can declare additional named targets under
`provision.targets`, selected with `--target`:

| Kind | Purpose |
|---|---|
| `aws/s3` | Uploads the template to S3. Selected directly (`--target <name>`), it's publish-only (upload and stop — a review step, or a template too large to pass inline). It's also used **automatically** to package any template that exceeds CloudFormation's 51,200-byte inline size limit, regardless of which target is selected for the deploy. `bucket` and `region` are required (`region` builds the `https://` URL `CreateChangeSet`'s `TemplateURL` needs — a bare `s3://` URI is rejected). |
| `git` | Commits the template YAML to a `git.repositories`-declared repository instead of deploying — a review/GitOps pipeline that applies from the committed template separately. Same shape native Helm/Kubernetes use. |
| `aws/stackset` | Multi-account/multi-region delivery — **not** selectable via `apply --target`; used exclusively by the `stackset` verb group (see Stack Sets below). |

"Backend," for this component type, means the **S3 artifact bucket** declared by a `kind: aws/s3`
target — CloudFormation's own stack state is service-managed, so the artifact bucket is the type's
only supporting infrastructure. It follows the same convention as `atmos terraform backend`:

```shell
atmos aws cloudformation backend create vpc -s dev
atmos aws cloudformation backend describe vpc -s dev
atmos aws cloudformation backend update vpc -s dev
atmos aws cloudformation backend delete vpc -s dev
atmos aws cloudformation backend list
```

The bucket must exist before a packaged `apply`/`deploy` uploads to it — either run `backend create`
explicitly, or set `provision.backend.enabled: true` (a sibling of `targets`, not nested under one)
to auto-provision it the first time it's missing, via the same S3 backend provisioner Terraform's
`provision.backend.enabled` uses:

```yaml
components:
  "aws/cloudformation":
    vpc:
      provision:
        backend:
          enabled: true
        targets:
          artifacts:
            kind: aws/s3
            bucket: acme-plat-cfn-artifacts
            region: us-east-2
```

Auto-provisioning only checks existence up front; it never reconciles a bucket that already exists.
`backend create`/`update`, run explicitly, always re-apply secure defaults (versioning, encryption,
public-access blocking, tags).

**Packaging scope today**: automatic packaging uploads the **template body itself** when it exceeds
the inline size limit. It does not currently rewrite local-asset references inside the template
(Lambda source zips, nested-stack templates referenced by relative path) the way `aws cloudformation
package`/Rain's `pkg` do — pre-upload those assets out-of-band and reference the resulting S3
location directly in the template until a future phase closes this gap.

## Stack Sets

Multi-account/multi-region orchestration, resolved from a `kind: aws/stackset` provision target:

```yaml
components:
  "aws/cloudformation":
    vpc:
      provision:
        targets:
          multi-account:
            kind: aws/stackset
            accounts: ["111111111111", "222222222222"]
            regions: ["us-east-1", "us-west-2"]
            permission_model: SELF_MANAGED
            administration_role_arn: arn:aws:iam::111111111111:role/AWSCloudFormationStackSetAdministrationRole
            execution_role_name: AWSCloudFormationStackSetExecutionRole
```

```shell
atmos aws cloudformation stackset create vpc -s dev --target=multi-account
atmos aws cloudformation stackset update vpc -s dev --target=multi-account
atmos aws cloudformation stackset delete vpc -s dev
atmos aws cloudformation stackset instances vpc -s dev
```

`stackset create` creates the StackSet's initial stack instances only when both `accounts` and
`regions` are set on the target; `stackset update` propagates a template/parameter/capability change
to every existing instance without changing which accounts/regions have one. `stackset delete` and
`stackset instances` act directly on `stack_name` and don't resolve or require a `provision.targets`
entry. `create`/`update`/`delete` prompt for confirmation (skip with `--auto-approve`).
