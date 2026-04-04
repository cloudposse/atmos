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

## Score: 35/42 Controls Passing (83%)

### Failing Controls

| Control      | Title                                                                    | Severity |
|--------------|--------------------------------------------------------------------------|----------|
| Config.1     | AWS Config should be enabled with service-linked role                    | CRITICAL |
| EC2.14       | Security groups should not allow ingress from 0.0.0.0/0 to port 3389     | HIGH     |
| EC2.13       | Security groups should not allow ingress from 0.0.0.0/0 to port 22       | HIGH     |
| S3.1         | S3 buckets should have block public access settings enabled              | MEDIUM   |
| EC2.6        | VPC flow logging should be enabled in all VPCs                           | MEDIUM   |
| IAM.17       | Ensure IAM password policy expires passwords within 90 days              | LOW      |
| CloudTrail.7 | Ensure S3 bucket access logging is enabled on CloudTrail S3 bucket       | LOW      |
```

The compliance report queries Security Hub enabled standards, counts total controls via
`ListSecurityControlDefinitions`, and computes the pass/fail score. Supports `--framework`
filter (`cis-aws`, `pci-dss`, `soc2`, `hipaa`, `nist`) and all output formats.

### Compliance report with `--ai`

```text
$ atmos aws compliance report --ai

✓ AI analysis complete — AWS Compliance Report: CIS Foundations Benchmark

## Overall Status: 🟡 83% Compliant (35/42 controls passing)

7 failing controls, including critical/high severity issues that need
immediate attention.

## 🚨 Priority Issues (Fix First)

### CRITICAL
| Control  | Issue                                   | Action                               |
|----------|-----------------------------------------|--------------------------------------|
| Config.1 | AWS Config not enabled or missing role  | Enable in all regions, attach role   |

### HIGH
| Control | Issue                             | Action                                  |
|---------|-----------------------------------|-----------------------------------------|
| EC2.14  | RDP (port 3389) open to 0.0.0.0/0 | Restrict to known IP ranges or VPN      |
| EC2.13  | SSH (port 22) open to 0.0.0.0/0   | Use SSM Session Manager instead of SSH  |

⚠️ Open SSH/RDP to the world is a common attack vector.

## 🟠 Medium Priority
• S3.1 — Enable S3 Block Public Access at the account level
• EC2.6 — Enable VPC Flow Logs for all VPCs

## 🟢 Low Priority
• IAM.17 — Set IAM password policy MaxPasswordAge to ≤ 90 days
• CloudTrail.7 — Enable S3 access logging on CloudTrail bucket

## Recommended Next Steps
1. Lock down security groups for ports 22/3389 — use VPN or bastion CIDR
2. Enable AWS Config — also helps detect future drift automatically
3. Run `atmos terraform apply` on security-groups, vpc, config components
4. Re-run this report after remediation to verify score improves
```

## Related Examples

- **[AI with API Providers](../ai/)** — Multi-provider AI configuration with sessions and tools.
- **[AI with Claude Code CLI](../ai-claude-code/)** — Use your Claude subscription with MCP server pass-through.
- **[MCP Server Integrations](../mcp/)** — Connect to AWS MCP servers for billing, IAM, and documentation.

## Key Files

| File         | Purpose                                         |
|--------------|-------------------------------------------------|
| `atmos.yaml` | Security config, auth, AI provider, tag mapping |
