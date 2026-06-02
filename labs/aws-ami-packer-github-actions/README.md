# Build, Scan, Approve & Share AWS AMIs with Atmos + Packer + GitHub Actions

A complete, copy-and-run Atmos project that builds a hardened **Amazon Linux 2023**
AMI with Packer, validates it on a live test instance, optionally scans it, gates
promotion behind a **manual approval**, tags the approved image `ScanStatus=approved`,
and **shares it across AWS accounts** — all orchestrated by Atmos and driven from a
GitHub Actions pipeline.

This Hands-on Lab combines several Atmos features into a single production-shaped workflow you can clone and adapt.

## What it teaches

This Lab combines, in one project:

- **Packer components in Atmos** — `components.packer` config and `atmos packer init/build/output`.
- **Stacks for Packer** — every build input (source AMI, networking, encryption, tags, provisioner list) is a stack var,
  not hardcoded HCL.
- **Go templating in stacks** — the source AMI name resolves from an environment variable at build time.
- **Nested custom commands** — an `atmos ami <subcommand>` command tree (get-ami-id, tag, list-tags, get-tag,
  launch-instance, list/terminate-instances, share) that wraps small, reviewable scripts.
- **CI/CD with a governance gate** — a GitHub Actions pipeline using OIDC auth, ephemeral runners, and a manual approval
  Environment.
- **Tag-based launch governance** — a reference IAM/SCP policy that restricts EC2 launches to AMIs tagged
  `ScanStatus=approved`.

## Architecture

```text
        ┌──────────────────────────────────────────────────────────────┐
        │ GitHub Actions Pipeline (.github/workflows/ami.yml)          │
        ├──────────────────────────────────────────────────────────────┤
        │ build (Packer via Atmos) → launch test instance              │
        │   → health check → [optional] scan                           │
        │   → ⏸ manual approval gate (GitHub Environment)              │
        │   → tag ScanStatus=approved → share AMI → cleanup            │
        └───────────────┬──────────────────────────────────────────────┘
                        │ atmos packer build / atmos ami …
        ┌───────────────▼──────────────────────────────────────────────┐
        │ Atmos                                                        │
        │  • Packer component (components/packer/al2023/main.pkr.hcl)  │
        │  • Stack (stacks/al2023.yaml) — all build inputs as vars     │
        │  • Custom commands (atmos ami …) → scripts/atmos/*.sh        │
        └───────────────┬──────────────────────────────────────────────┘
                        │ packer build
        ┌───────────────▼──────────────────────────────────────────────┐
        │ Packer (amazon-ebs)                                          │
        │  provisioners: patch-os → harden → [optional] scan agent     │
        │               → install-packages → finalize                  │
        │  post-processor: manifest.json                               │
        └──────────────────────────────────────────────────────────────┘
```

## Repository layout

```text
aws-ami-packer-github-actions/
├── atmos.yaml                              # Packer config + `atmos ami` custom command tree
├── components/packer/al2023/
│   ├── main.pkr.hcl                        # Parameterized amazon-ebs Packer template
│   └── scripts/                            # Provisioners run *inside* the image (in order)
│       ├── patch-os.sh
│       ├── harden.sh
│       ├── install-scan-agent.sh           # OPTIONAL, off by default
│       ├── install-packages.sh             # Edit this list for your image
│       └── finalize.sh
├── stacks/al2023.yaml                      # All build inputs as vars (placeholders marked)
├── scripts/atmos/                          # Host-side helpers backing `atmos ami` commands
│   ├── _lib.sh                             # Shared: resolve_ami_id, require_cmd/env
│   └── *.sh
├── .github/
│   ├── actions/setup-tools/                # Composite action: install Atmos + Packer
│   ├── actions/setup-aws-credentials/      # Composite action: OIDC role assumption
│   └── workflows/
│       └── ami.yml                         # Main governed pipeline
└── docs/                                   # Reference IAM/SCP policies + customization checklist
```

## Prerequisites

| Tool                                                                                     | Version (pinned in CI) | Purpose             |
|------------------------------------------------------------------------------------------|------------------------|---------------------|
| [Atmos](https://atmos.tools/install)                                                     | 1.220.0                | Orchestration       |
| [Packer](https://developer.hashicorp.com/packer/install)                                 | 1.15.3                 | Image build         |
| [AWS CLI](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html) | v2                     | `atmos ami` helpers |

You also need a standard **AWS account** with permission to build AMIs and launch
EC2 instances. A reference IAM policy is in [`docs/packer-build-iam-policy.json`](docs/packer-build-iam-policy.json).

> The default path runs with **only** a standard AWS account. The optional scan
> step (`install-scan-agent.sh`) needs a private package repo and a scanner
> subscription — it is disabled by default.

## Run it locally

```bash
# 1. Copy the Lab into a new repo of your own
cp -r labs/aws-ami-packer-github-actions/ my-ami-pipeline/
cd my-ami-pipeline/

# 2. Edit stacks/al2023.yaml — set region, networking, KMS, sharing targets, tags
#    (see docs/customization-checklist.md)

# 3. Initialize the Packer plugins and build
atmos packer init  al2023 -s al2023
atmos packer build al2023 -s al2023

# 4. Inspect / operate the result with the custom commands
atmos ami get-ami-id        al2023 -s al2023
atmos ami list-tags         al2023 -s al2023
atmos ami launch-instance   al2023 -s al2023 --type t3.small
atmos ami list-instances    al2023 -s al2023

# 5. Promote and share (normally done by the pipeline after approval)
atmos ami tag   al2023 -s al2023 --key ScanStatus --value approved
atmos ami share al2023 -s al2023 --accounts 123456789012,123456789013
```

## Run the governed pipeline (GitHub Actions)

1. Set repository variables `AWS_OIDC_ROLE_ARN` and `AWS_REGION`.
2. Create the OIDC build role using [`docs/oidc-trust-policy.json`](docs/oidc-trust-policy.json) and [
   `docs/packer-build-iam-policy.json`](docs/packer-build-iam-policy.json).
3. Create a GitHub Environment named **`ami-approval`** and add required reviewers.
4. Run the **AMI Pipeline** workflow (`workflow_dispatch`). It builds, health-checks,
   waits for approval, then tags and shares the AMI.

The full setup list is in [`docs/customization-checklist.md`](docs/customization-checklist.md).

## Customize

- **Build steps** — reorder/trim `provisioner_shell_scripts` and edit `install-packages.sh`.
- **Hardening** — toggle `ENABLE_FIREWALL` / `ENABLE_SELINUX_ENFORCING` via `provisioner_env_vars`.
- **Scanning** — set `ENABLE_SCAN_AGENT=true` + `SCAN_AGENT_REPO_URL` and fill in `install-scan-agent.sh`.
- **Governance** — attach [`docs/launch-restriction-scp.json`](docs/launch-restriction-scp.json) to enforce "launch only
  approved AMIs".

## Clean up

```bash
# Terminate any test instances launched from the AMI
atmos ami terminate-instances al2023 -s al2023

# Capture the AMI ID and region you want to remove
ami_id="$(atmos ami get-ami-id al2023 -s al2023)"
region="us-east-2"

# Find the EBS snapshot(s) backing the AMI *before* deregistering it
snapshot_ids="$(aws ec2 describe-images --region "$region" --image-ids "$ami_id" \
  --query 'Images[0].BlockDeviceMappings[].Ebs.SnapshotId' --output text)"

# Deregister the AMI
aws ec2 deregister-image --region "$region" --image-id "$ami_id"

# Delete the backing snapshot(s) — deregistering the AMI does NOT remove them,
# and leftover snapshots keep incurring EBS storage charges.
for snap in $snapshot_ids; do
  aws ec2 delete-snapshot --region "$region" --snapshot-id "$snap"
done
```

## Learn More

- [Atmos Packer commands](https://atmos.tools/cli/commands/packer/build)
- [Custom commands](https://atmos.tools/cli/configuration/commands)
- [Go templating in stacks](https://atmos.tools/templates)
- [GitHub Actions integration](https://atmos.tools/integrations/github-actions/setup-atmos)
- Related Examples: [`custom-commands`](https://atmos.tools/examples/custom-commands), [`demo-stacks`](https://atmos.tools/examples/demo-stacks)
