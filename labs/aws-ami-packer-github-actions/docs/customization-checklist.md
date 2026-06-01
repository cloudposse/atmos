# Customization Checklist

Work through this list when adapting the Lab to your environment. Every item maps
to a value you set in the stack, a repository variable, or a one-time AWS/GitHub
setup step. Nothing here is hardcoded in the Packer template or scripts.

## 1. Stack values (`stacks/al2023.yaml`)

- [ ] `region` — the AWS region to build in.
- [ ] `vpc_id` / `subnet_id` — leave empty for the default VPC, or set a private subnet.
- [ ] `temporary_security_group_source_cidrs` — narrow from `0.0.0.0/0` to your runner egress.
- [ ] `kms_key_arn` — empty for the default EBS key, or a CMK ARN for cross-account sharing.
- [ ] `assume_role_arn` — empty in CI (OIDC role provides creds); set for local builds.
- [ ] `share_account_ids` — comma-separated account IDs to share approved AMIs with.
- [ ] `ami_tags` — your tagging convention (keep `ScanStatus: pending`).
- [ ] `provisioner_shell_scripts` — reorder/trim the build steps.
- [ ] `provisioner_env_vars` — toggle optional hardening / scan agent.
- [ ] `install-packages.sh` `PACKAGES` list — the software your image ships with.

## 2. Repository variables (GitHub → Settings → Variables)

- [ ] `AWS_OIDC_ROLE_ARN` — ARN of the IAM role GitHub Actions assumes.
- [ ] `AWS_REGION` — region for the pipeline (match the stack `region`).

## 3. One-time AWS setup

- [ ] Create the GitHub OIDC identity provider in your account.
- [ ] Create the build role with `docs/oidc-trust-policy.json` (trust) and
      `docs/packer-build-iam-policy.json` (permissions).
- [ ] (Optional) Attach `docs/launch-restriction-scp.json` to enforce
      "launch only approved AMIs" org-wide.

## 4. One-time GitHub setup

- [ ] Create an Environment named `ami-approval`.
- [ ] Add required reviewers to that Environment (these people approve each AMI).

## 5. Optional features

- [ ] Vulnerability scan: set `ENABLE_SCAN_AGENT=true` and `SCAN_AGENT_REPO_URL`
      in the stack, fill in `install-scan-agent.sh`, and pass `enable_scan: true`.
- [ ] Auto-rebuild on new base images: keep `detect-base-image-update.yml`
      (grant the workflow `contents: write` + `actions: write`).
