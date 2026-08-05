# Reference policies

These JSON files are **paste-ready AWS policy documents** — they contain only the policy
elements AWS accepts (`Version`, `Statement`), with no comment keys, so you can copy them
straight into the console or CLI. AWS rejects any extra top-level keys (such as a
`_comment`) with a `MalformedPolicyDocument` error, which is why the explanatory notes
live here instead of inside the JSON.

Replace every placeholder (account IDs, `YOUR_ORG/YOUR_REPO`, `YOUR_PACKER_BUILD_ROLE`,
`EXAMPLE-KEY-ID`, regions) before using these in your environment.

## `oidc-trust-policy.json`

Trust policy for the IAM role that GitHub Actions assumes via OIDC. Attach it as the
role's trust relationship.

- Replace `123456789012` with your account ID, `YOUR_ORG/YOUR_REPO` with your GitHub
  org/repo, and `main` with the branch that runs this pipeline.
- The `sub` condition pins access to a **single repo AND branch**. Do **not** use the
  broad `repo:YOUR_ORG/YOUR_REPO:*`, which lets any branch, tag, or PR in the repo assume
  the role.
- To scope to a deployment environment instead, use
  `repo:YOUR_ORG/YOUR_REPO:environment:YOUR_ENVIRONMENT`. Note that only jobs declaring
  `environment:` receive that claim — the `build`, `health-check`, and `cleanup` jobs in
  `ami.yml` do not, so they need a branch-scoped statement like the one provided.

## `packer-build-iam-policy.json`

Reference identity (permissions) policy for the Packer build role that GitHub OIDC
assumes. Grants the EC2/AMI permissions Packer needs to build, tag, share, and clean up.

- Scope it down further for production (e.g. restrict regions, require tag conditions on
  `RunInstances`).
- The KMS statements are only needed when building **encrypted AMIs with a
  customer-managed key**. `kms:CreateGrant` is constrained with
  `kms:GrantIsForAWSResource=true` — the least-privilege pattern for grants EC2/EBS create
  on your behalf.
- The optional `atmos ami share --kms-grant` makes a **direct** `kms:CreateGrant` to share
  the key cross-account, which does **not** satisfy that condition. If you enable it, add a
  separate `CreateGrant` statement scoped to the target accounts (e.g. with a
  `kms:GranteePrincipal` condition).
- The `ReadBaseImageFromSSM` statement is optional. This template resolves the base AMI by
  name via `ec2:DescribeImages`; the statement is retained so you can switch to resolving
  the base AMI from the public AWS SSM parameter
  (e.g. `/aws/service/ami-amazon-linux-latest/*`) without editing the policy.

## `launch-restriction-scp.json`

Organization Service Control Policy (SCP) that enforces the governance rule: instances may
only be launched from AMIs tagged `ScanStatus=approved`. Attach it to an OU/account in AWS
Organizations.

- The first statement denies `RunInstances` when the AMI's `ScanStatus` tag is anything
  other than `approved`; the second denies it when the tag is absent entirely. Together
  they **fail closed**. The condition keys reference the AMI via `ec2:ResourceTag` on the
  image.
- **Both statements exempt the Packer build/test role** via `aws:PrincipalArn`
  (`ArnNotLike`) because the pipeline's health-check step must launch the freshly built,
  **not-yet-approved** AMI — without the exemption a blanket deny breaks the build.
- Replace `YOUR_PACKER_BUILD_ROLE` with your build role name (and tighten the account/path)
  before attaching.
