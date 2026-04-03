# Example: AWS Security & Compliance

Analyze AWS security findings, map them to Atmos components and stacks, and get
AI-powered remediation guidance.

Learn more in the [AWS Security documentation](https://atmos.tools/cli/commands/aws/security).

> This example requires AWS credentials with Security Hub access.
> Update the `auth` section in `atmos.yaml` with your SSO settings.

## What You'll See

- [Security finding analysis](https://atmos.tools/cli/commands/aws/security) mapped to Atmos components
- [Compliance reporting](https://atmos.tools/cli/commands/aws/compliance) against CIS, PCI-DSS, SOC2
- [Atmos Auth](https://atmos.tools/cli/configuration/auth) for automatic AWS credential injection
- Optional AI remediation via `--ai` flag (root cause, code changes, deploy commands)

## Prerequisites

1. **Atmos Auth** — update `auth` in `atmos.yaml` with your SSO start URL, permission set,
   and security account ID, then authenticate:
   ```bash
   export ATMOS_PROFILE=devops  # or your profile name
   atmos auth login
   ```

2. **AI provider** (optional, for `--ai`):
   ```bash
   export ANTHROPIC_API_KEY="your-api-key"
   ```

## Try It

```shell
cd examples/aws-security-compliance

# All findings (grouped by default)
atmos aws security analyze

# Filter by stack and component
atmos aws security analyze --stack prod-us-east-1
atmos aws security analyze --stack prod-us-east-1 --component vpc

# Filter by severity or source
atmos aws security analyze --severity critical,high --source guardduty

# AI-powered remediation (deduplicates findings, retries on transient errors)
atmos aws security analyze --stack prod-us-east-1 --ai

# Save to file (Markdown, JSON, YAML, or CSV)
atmos aws security analyze --format json --file findings.json
atmos aws security analyze --stack prod-us-east-1 --file report.md

# Compliance reports
atmos aws compliance report --framework cis-aws
atmos aws compliance report --framework pci-dss --format json --file compliance.json
```

## See It in Action

Tested against a multi-account AWS organization (11 accounts, Security Hub delegated admin,
500 findings fetched, 97% mapped to Atmos components).

### Without `--ai` — findings mapped to components

```text
$ atmos aws security analyze --stack plat-use2-dev --component rds/example

ℹ Fetching security findings...
ℹ Mapping 500 findings to Atmos components...
ℹ Filtered to 4 findings matching stack="plat-use2-dev" component="rds/example"

# Security Report: plat-use2-dev / rds/example

Findings: 4 (1 CRITICAL, 3 HIGH)

## CRITICAL Findings (1)

### 1. Security groups should not allow unrestricted access to ports with high risk

| Field          | Value                                                        |
|----------------|--------------------------------------------------------------|
| **Severity**   | CRITICAL                                                     |
| **Source**     | security-hub (aws-foundational-security-best-practices/1.0)  |
| **Resource**   | arn:aws:ec2:us-east-2:***:security-group/sg-***              |
| **Component**  | rds/example                                                  |
| **Stack**      | plat-use2-dev                                                |
| **Confidence** | exact                                                        |
| **Mapped By**  | finding-tag                                                  |

Resource Tags:

• atmos_stack = plat-use2-dev
• atmos_component = rds/example
• terraform_component = rds
• terraform_workspace = plat-use2-dev-rds-example
• Name = acme-plat-use2-dev-example-postgres-db
• Namespace = acme
• Tenant = plat
• Environment = use2
• Stage = dev

## HIGH Findings (3)

### 1. Security groups should only allow unrestricted incoming traffic for authorized ports
### 2. Security groups should not allow ingress from 0.0.0.0/0 to port 22
### 3. Security groups should not allow ingress from 0.0.0.0/0 to port 3389

## Summary

| Severity     | Count  | Mapped | Unmapped |
|--------------|--------|--------|----------|
| CRITICAL     | 1      | 1      | 0        |
| HIGH         | 3      | 3      | 0        |
| **Total**    | **4**  | **4**  | **0**    |
```

All 4 findings point to the same security group on the `rds/example` component, traced via
`atmos_stack` and `atmos_component` resource tags.

### With `--ai` — AI-powered remediation

```text
$ atmos aws security analyze --stack plat-use2-dev --component rds/example --ai

ℹ Fetching security findings...
ℹ Mapping 500 findings to Atmos components...
ℹ Filtered to 4 findings matching stack="plat-use2-dev" component="rds/example"
ℹ Analyzing findings with AI...

✓ AI analysis complete — Security Analysis: rds/example in plat-use2-dev

## Summary

The analysis surfaced 4 findings against a single security group — all mapped
with exact confidence to this component via Atmos tags.

| Severity   | Count |
|------------|-------|
| 🔴 CRITICAL | 1    |
| 🟠 HIGH     | 3    |

## Findings Breakdown

### 🟠 Finding 1 — EC2.18: Unrestricted Ingress on Unauthorized Port (HIGH)

Standard: AWS Foundational Security Best Practices v1.0.0

Port 5432 (PostgreSQL) is open to 0.0.0.0/0. The likely cause is
allowed_cidr_blocks being set to an overly permissive value — potentially
from commented-out lines in catalog/rds/defaults.yaml that were activated
at some point.

Fix: Set in catalog/rds/example.yaml:
    allowed_cidr_blocks: []
    publicly_accessible: false

### 🟠 Finding 2 — EC2.13: Unrestricted Ingress on Port 22/SSH (HIGH)

Standard: CIS AWS Foundations Benchmark v1.2.0

⚠️ This is anomalous — port 22 has no business being on an RDS security
group. This strongly suggests an out-of-band manual change was made directly
in the AWS Console, or a referenced SG in associate_security_group_ids
carries a port-22 rule.

Fix:
1. Immediately audit and manually remove the port-22 rule in the AWS Console
2. Audit any SGs referenced via associate_security_group_ids / security_group_ids
3. Re-apply via Terraform to restore IaC control and eliminate drift

## Root Cause (Common Thread)

Both findings stem from the same security group and share a root cause:
var.allowed_cidr_blocks being set too permissively, compounded by possible
out-of-band drift. The cloudposse/rds/aws module internally creates and
manages SG ingress rules based on this variable.

## Priority Actions

1. Immediately remove the port-22 inbound rule manually — this is likely
   out-of-band drift and poses direct unauthorized access risk

2. Update catalog/rds/example.yaml to explicitly enforce safe defaults:
     allowed_cidr_blocks: []
     publicly_accessible: false
     associate_security_group_ids: []
     use_private_subnets: true

3. Add Terraform validation guards to rds-variables.tf to prevent future
   regressions:
     validation {
       condition     = !contains(var.allowed_cidr_blocks, "0.0.0.0/0")
                       && !contains(var.allowed_cidr_blocks, "::/0")
       error_message = "allowed_cidr_blocks must not contain 0.0.0.0/0 or ::/0."
     }

4. Clean up catalog/rds/defaults.yaml — permanently remove (don't just
   comment out) any lines with 0.0.0.0/0 or publicly_accessible: true

5. Plan then apply:
     atmos terraform plan rds/example -s plat-use2-dev
     atmos terraform apply rds/example -s plat-use2-dev

## Risk Assessment

| Finding              | Risk   | Note                                              |
|----------------------|--------|---------------------------------------------------|
| EC2.18 (port 5432)   | Medium | Removing rule breaks direct internet connections  |
|                      |        | to DB; client SG-based connections are unaffected |
| EC2.13 (port 22/SSH) | Low    | No RDS traffic should depend on SSH; removing     |
|                      |        | has no expected legitimate impact                 |
```

The AI used multi-turn tools (`atmos_describe_component`, `read_component_file`) to read
the actual Terraform source and stack config, detected that port 22 on an RDS security group
is anomalous (likely AWS Console drift), identified the common root cause in
`allowed_cidr_blocks`, and generated targeted remediation with Terraform validation guards
to prevent future regressions. Duplicate findings are deduplicated before AI analysis.

### Compliance report

```text
$ atmos aws compliance report

ℹ Generating compliance report...

# Compliance Report: CIS AWS Foundations Benchmark

Date: 2026-04-03  Framework: CIS AWS Foundations Benchmark

## Score: 40/42 Controls Passing (95%)

### Failing Controls

| Control                                         | Title                                                                              | Severity |
|-------------------------------------------------|------------------------------------------------------------------------------------|----------|
| ruleset/cis-aws-foundations-benchmark/v/1.2.0   | AWS Config should be enabled and use the service-linked role for resource recording | CRITICAL |
| standards/aws-foundational-security-best-practices/v/1.0.0 | S3 general purpose buckets should have block public access settings enabled | MEDIUM   |
```

The compliance report queries Security Hub enabled standards, counts total controls via
`ListSecurityControlDefinitions`, and computes the pass/fail score. Supports `--framework`
filter (`cis-aws`, `pci-dss`, `soc2`, `hipaa`, `nist`) and all output formats.

### Compliance report with `--ai`

```text
$ atmos aws compliance report --ai

✓ AI analysis complete

## ✅ AWS Compliance Report Summary

Your infrastructure is passing 40 out of 42 controls (95%).

## ⚠️ 2 Failing Controls to Address

### 1. 🔴 CRITICAL — AWS Config Not Properly Configured

Control: cis-aws-foundations-benchmark/v/1.2.0

AWS Config is either disabled or not using the required service-linked
role for resource recording.

How to fix:
• Ensure AWS Config is enabled in all active regions
• Attach the AWSServiceRoleForConfig service-linked role
• Verify your aws_config_configuration_recorder resource:
    recording_group {
      all_supported                 = true
      include_global_resource_types = true
    }

### 2. 🟡 MEDIUM — S3 Bucket(s) Missing Public Access Block

Control: aws-foundational-security-best-practices/v/1.0.0

How to fix:
• Apply to all S3 buckets in your Terraform components:
    resource "aws_s3_bucket_public_access_block" "this" {
      block_public_acls       = true
      block_public_policy     = true
      ignore_public_acls      = true
      restrict_public_buckets = true
    }
• Consider enforcing via an SCP at the org level

## 📋 Recommended Next Steps

| Priority      | Action                                                     |
|---------------|------------------------------------------------------------|
| 🔴 Immediate  | Enable AWS Config with service-linked role in all regions  |
| 🟡 Soon       | Audit and remediate S3 public access block settings        |
| 🔵 Ongoing    | Re-run `atmos aws compliance report` to confirm 42/42      |

💡 You're at 95% — fixing these two controls achieves a perfect score.
```

## Related Examples

- **[AI with API Providers](../ai/)** — Multi-provider AI configuration with sessions and tools.
- **[AI with Claude Code CLI](../ai-claude-code/)** — Use your Claude subscription with MCP server pass-through.
- **[MCP Server Integrations](../mcp/)** — Connect to AWS MCP servers for billing, IAM, and documentation.

## Key Files

| File         | Purpose                                         |
|--------------|-------------------------------------------------|
| `atmos.yaml` | Security config, auth, AI provider, tag mapping |
