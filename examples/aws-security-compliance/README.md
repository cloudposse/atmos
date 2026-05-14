# Example: AWS Security & Compliance

Analyze AWS security findings, map them to Atmos components and stacks, and get
AI-powered remediation guidance.

Learn more in the [AWS Security documentation](https://atmos.tools/cli/commands/aws/security).

> This example requires AWS credentials with Security Hub access.
> Update the `auth` section in `atmos.yaml` with your SSO settings.

## Prerequisites

1. **Atmos Auth** — update `auth` in `atmos.yaml` with your SSO start URL, permission set,
   and security account ID, then authenticate:
   ```bash
   atmos auth login
   ```

2. **AI provider** (optional, for `--ai`):
   ```bash
   export ANTHROPIC_API_KEY="your-api-key"
   ```

## Try It

```shell
cd examples/aws-security-compliance

# All findings
atmos aws security analyze

# Filter by stack and component
atmos aws security analyze --stack prod-us-east-1 --component vpc

# AI-powered remediation
atmos aws security analyze --stack prod-us-east-1 --ai

# Save as JSON
atmos aws security analyze --format json --file findings.json

# Compliance reports
atmos aws compliance report --framework cis-aws
atmos aws compliance report --ai
```

## See It in Action

### Security findings mapped to components

```text
$ atmos aws security analyze --stack plat-use2-dev --component rds/example

# Security Report: plat-use2-dev / rds/example — 4 findings (1 CRITICAL, 3 HIGH)

| Field          | Value                                                       |
|----------------|-------------------------------------------------------------|
| **Component**  | rds/example                                                 |
| **Stack**      | plat-use2-dev                                               |
| **Confidence** | exact                                                       |
| **Mapped By**  | finding-tag                                                 |

Resource Tags: atmos_stack=plat-use2-dev, atmos_component=rds/example,
  Namespace=acme, Tenant=plat, Environment=use2, Stage=dev

| Severity  | Count | Mapped |
|-----------|-------|--------|
| CRITICAL  | 1     | 1      |
| HIGH      | 3     | 3      |
```

### With `--ai` — AI-powered remediation

```text
$ atmos aws security analyze --stack plat-use2-dev --component rds/example --ai

✓ AI analysis complete — rds/example in plat-use2-dev

## EC2.18: Port 5432 open to 0.0.0.0/0 (HIGH)
Fix: allowed_cidr_blocks: [], publicly_accessible: false

## EC2.13: Port 22/SSH open on RDS SG (HIGH)
⚠️ Anomalous — likely out-of-band console drift. Remove manually.

## Priority Actions
1. Remove port-22 rule manually (drift)
2. Update catalog/rds/example.yaml:
     allowed_cidr_blocks: []
     publicly_accessible: false
     use_private_subnets: true
3. Add Terraform validation guard for allowed_cidr_blocks
4. atmos terraform apply rds/example -s plat-use2-dev

| Finding            | Risk   |
|--------------------|--------|
| EC2.18 (port 5432) | Medium |
| EC2.13 (port 22)   | Low    |
```

### Compliance report

```text
$ atmos aws compliance report

## Score: 35/42 Controls Passing (83%)

| Control      | Title                                              | Severity |
|--------------|----------------------------------------------------|----------|
| Config.1     | AWS Config should be enabled                       | CRITICAL |
| EC2.14       | SG allows ingress from 0.0.0.0/0 to port 3389     | HIGH     |
| EC2.13       | SG allows ingress from 0.0.0.0/0 to port 22       | HIGH     |
| S3.1         | S3 block public access not enabled                 | MEDIUM   |
| EC2.6        | VPC flow logging not enabled                       | MEDIUM   |
| IAM.17       | Password policy doesn't expire in 90 days          | LOW      |
| CloudTrail.7 | S3 access logging not enabled on CloudTrail bucket | LOW      |
```

### Compliance with `--ai`

```text
$ atmos aws compliance report --ai

✓ 83% Compliant (35/42) — 7 failing controls

🔴 Config.1: Enable AWS Config with service-linked role
🟠 EC2.14/EC2.13: Lock down SG ports 22/3389 — use VPN or SSM
🟡 S3.1: Enable Block Public Access | EC2.6: Enable VPC Flow Logs
🟢 IAM.17: Set password expiry ≤90d | CloudTrail.7: Enable S3 access logging

Next: atmos terraform apply on security-groups, vpc, config components
```

## Key Files

| File         | Purpose                                         |
|--------------|-------------------------------------------------|
| `atmos.yaml` | Security config, auth, AI provider, tag mapping |
