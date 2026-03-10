# Atmos AWS Security & Compliance - Product Requirements Document

**Status:** In Progress
**Version:** 0.5
**Last Updated:** 2026-03-10

---

## Executive Summary

The main purpose of Atmos AWS Security & Compliance is to make it **very easy and fast** for users to
detect, review, analyze, and ask questions about any security and compliance findings from all AWS
security and compliance services — directly from the Atmos CLI.

It connects to AWS Security Hub, AWS Config, Amazon Inspector, Amazon GuardDuty, Amazon Macie, and
IAM Access Analyzer via Atmos Auth, maps findings back to Atmos components and stacks, and generates
actionable remediation reports. AI analysis is **opt-in** via the `--ai` flag — the commands work
without any AI provider configured.

**AI is optional.** By default, `atmos aws security` and `atmos aws compliance` work purely with AWS
APIs — no AI provider needed. When the `--ai` flag is passed, AI-powered analysis adds root cause
analysis, remediation guidance, and deploy commands using **any AI provider that Atmos AI supports**.

**Supported AI providers (for `--ai`):** Anthropic (Claude), OpenAI (GPT), Google Gemini, Azure OpenAI,
AWS Bedrock, Ollama (local/on-premise), and Grok (xAI). See the
[AWS Bedrock AI Provider](aws-bedrock-ai-provider.md) reference for enterprise data residency setup.

**Cloud-specific namespace.** These commands live under `atmos aws` (not `atmos ai`) because they are
AWS-specific. This design enables future `atmos azure security` and `atmos gcp security` commands
for other cloud providers.

The key differentiator: Atmos owns the component-to-stack relationship, so it can trace a security
finding on an AWS resource all the way back to the exact Terraform code and stack configuration that
created it — and generate a targeted fix.

### Why This Matters

Today, reviewing security findings requires navigating multiple AWS console pages, cross-referencing
resources with Terraform code, and manually figuring out which configuration caused the issue. This is
slow, error-prone, and requires deep AWS + Terraform expertise.

With Atmos AWS Security & Compliance, a single command replaces that entire workflow:

```shell
atmos aws security --stack prod-us-east-1
```

This fetches findings, maps them to Atmos components, and shows which code manages each affected
resource. Add `--ai` for AI-powered remediation guidance:

```shell
atmos aws security --stack prod-us-east-1 --ai
```

In seconds, the user gets a complete picture: what's wrong, which component caused it, exactly what
code to change, and how to deploy the fix.

For CI/CD pipelines and automation, all commands support structured output formats (`--format json`,
`--format yaml`, `--format csv`) so findings can be piped to dashboards, ticketing systems, Slack
notifications, or compliance reporting tools.

---

## Goals

1. **Discover** — Pull security findings from all AWS security services for a given stack (or all stacks)
2. **Trace** — Map each finding's affected AWS resource back to the Atmos component and stack that manages it
3. **Analyze** — Use the configured AI provider (Anthropic, OpenAI, Gemini, Azure OpenAI, Bedrock, Ollama, or Grok)
   to understand the finding, read the component source code, and determine the root cause in the Terraform/Atmos
   configuration
4. **Report** — Generate reports in multiple formats (Markdown for humans, JSON/YAML/CSV for automation)
   with findings, severity, affected components, and step-by-step remediation instructions
5. **Ask** — Enable interactive AI-powered Q&A about findings (`atmos ai chat` with security context)
6. **Authenticate** — Use Atmos Auth for AWS access (Security Hub, Config, Inspector, GuardDuty,
   Macie, Access Analyzer) and use the configured AI provider's credentials for AI analysis

### Non-Goals (v1)

- Auto-applying fixes (user reviews and applies manually)
- PR creation (future enhancement)
- Non-AWS cloud providers (Azure, GCP — future)
- Real-time monitoring or webhooks
- Web UI or dashboard

---

## Architecture Overview

```text
┌─────────────────────────────────────────────────────────────────────┐
│                    atmos aws security <stack>                        │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  ┌───────────┐    ┌───────────┐    ┌───────────┐    ┌───────────┐   │
│  │  Atmos    │    │  AWS      │    │  AWS      │    │  Atmos    │   │
│  │  Auth     │───▶│  Security │───▶│    AI     │───▶│  Atmos    │   │
│  │           │    │  Services │    │ Provider  │    │  Report   │   │
│  └───────────┘    └───────────┘    └───────────┘    └───────────┘   │
│       │                │                │                │          │
│       ▼                ▼                ▼                ▼          │
│  AWS Creds        Findings         Analysis        Markdown         │
│  (SSO/Role)       (JSON)           (AI)            (CLI Output)     │
│                                                                     │
│  ┌──────────────────────────────────────────────────────────────┐   │
│  │                  Finding-to-Code Mapping                     │   │
│  │                                                              │   │
│  │  Security Finding ──▶ AWS Resource ──▶ Resource Tags ──▶     │   │
│  │  Atmos Stack ──▶ Atmos Component ──▶ Terraform Source ──▶    │   │
│  │  Root Cause ──▶ Remediation                                  │   │
│  └──────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Authenticate** — Atmos Auth obtains AWS credentials (SSO, assume-role, etc.)
2. **Fetch Findings** — Query Security Hub / Config / Inspector / GuardDuty for the target stack's resources
3. **Map to Components** — Use resource tags (`atmos:stack`, `atmos:component`) to trace findings back to IaC
4. **AI Analysis** — Send finding details + component source + stack config to the configured AI provider for root cause analysis
5. **Generate Report** — Render findings with remediation steps as Markdown in the terminal

---

## Finding-to-Code Mapping: Two Paths

The system supports two mapping strategies depending on whether the user's infrastructure has
Atmos resource tags. Both paths are fully functional — tags make mapping instant and deterministic,
while the tagless path uses multiple heuristics and AI inference to achieve the same result.

### Path A: Tag-Based Mapping (Fast, Deterministic)

If the user's infrastructure has `atmos:*` resource tags, mapping is instant and exact. A single
API call to the AWS Resource Groups Tagging API resolves any resource ARN to its Atmos component
and stack.

**Required tags:**

| Tag Key             | Value Example    | Purpose                          |
|---------------------|------------------|----------------------------------|
| `atmos:stack`       | `prod-us-east-1` | Full stack name                  |
| `atmos:component`   | `vpc`            | Component name                   |
| `atmos:tenant`      | `acme`           | Tenant (if using tenant pattern) |
| `atmos:environment` | `prod`           | Environment                      |
| `atmos:stage`       | `us-east-1`      | Stage                            |
| `atmos:workspace`   | `prod-use1-vpc`  | Terraform workspace              |

**Tag configuration:**

```yaml
# stacks/catalog/defaults.yaml
vars:
  default_tags:
    atmos:stack: "{{ .atmos_stack }}"
    atmos:component: "{{ .atmos_component }}"
    atmos:tenant: "{{ .vars.tenant }}"
    atmos:environment: "{{ .vars.environment }}"
    atmos:stage: "{{ .vars.stage }}"
```

```hcl
# components/terraform/_defaults/provider.tf
provider "aws" {
  default_tags {
    tags = var.default_tags
  }
}
```

**Flow:** Finding → Resource ARN → Tag lookup → `atmos:component` + `atmos:stack` → Done.

### Path B: Tagless Mapping (Heuristic + AI Inference)

Many existing infrastructure deployments do not have `atmos:*` tags. The system must still work
for these users — it just takes a different, multi-strategy approach to figure out which Atmos
component and stack own the affected resource.

The tagless mapping pipeline tries each strategy in order, stopping at the first confident match:

#### Strategy 1: Terraform State Search

Search Terraform state files for the resource ARN. Atmos knows every component/stack combination
and can locate their state files (local or remote backend).

```text
For each stack:
  For each component in stack:
    Read terraform.tfstate (or query remote backend)
    Search for resource ARN in state resources
    If found → match (component, stack)
```

This is the most reliable tagless strategy because the state file is the source of truth for what
Terraform manages.

**Implementation:** Reuse the existing `!terraform.state` YAML function infrastructure. Atmos
already has code that reads Terraform state files for any component/stack directly from the remote
backend (S3 bucket in AWS). The same state-reading logic can be used to scan state resources for
a given ARN. This avoids duplicating backend authentication, state parsing, and remote state access.

**Optimization:** Build a reverse index (resource ARN → component/stack) on first run and cache it.
Invalidate on `atmos terraform apply`.

#### Strategy 2: Resource Naming Convention Analysis

Many organizations follow naming conventions that embed component and environment information in
resource names. For example:

| Resource Name                   | Inferred Component | Inferred Stack   |
|---------------------------------|--------------------|------------------|
| `acme-prod-use1-vpc`            | `vpc`              | `prod-us-east-1` |
| `acme-prod-use1-s3-bucket-data` | `s3-bucket`        | `prod-us-east-1` |
| `prod-rds-primary`              | `rds`              | `prod-*`         |

The system builds a naming pattern from Atmos's `name_template` and stack variables, then matches
resource names against these patterns. This works well for organizations using `cloudposse/label`
or similar naming modules.

```text
For each component in known stacks:
  Generate expected resource name prefix from name_template + vars
  Match against resource name from finding
  If prefix matches → candidate (component, stack) with confidence score
```

#### Strategy 3: Resource Type to Component Mapping

Map AWS resource types to likely Atmos components based on the component's Terraform source code.
For example, a component that contains `aws_s3_bucket` resources is a candidate for S3-related
findings.

```text
Build index: scan all component source files for AWS resource type declarations
  e.g., components/terraform/vpc/main.tf contains "aws_vpc", "aws_subnet", "aws_route_table"

When finding arrives with resource type "AwsEc2Vpc":
  Look up which components declare "aws_vpc" resources
  Return candidate components with confidence score
```

This strategy is less precise (multiple components may create the same resource type) but narrows
the search space significantly.

#### Strategy 4: AI-Assisted Inference

When strategies 1-3 produce no confident match, or produce multiple candidates, send the finding
details + candidate list + component catalog to the configured AI provider for AI-assisted resolution:

```text
Prompt to AI:
  "Given this security finding on resource [ARN] of type [type] in account [id] / region [region],
   and the following Atmos components: [list with descriptions],
   which component most likely manages this resource? Explain your reasoning."
```

The AI uses contextual clues: resource naming, account/region alignment with stack patterns,
resource type alignment with component source code, and finding metadata.

#### Strategy 5: Manual Override / Unmapped

If no strategy produces a match, the finding is reported as **unmapped** with:

- The resource ARN and type
- The strategies attempted and why they failed
- A suggestion to add `atmos:*` tags for future automatic mapping
- An option for the user to manually specify the component/stack via `--component` flag

**Mapping confidence levels:**

| Confidence | Source                  | Displayed As    |
|------------|-------------------------|-----------------|
| **Exact**  | Tag-based (Path A)      | Confirmed match |
| **High**   | Terraform state match   | Confirmed match |
| **Medium** | Naming convention match | Likely match    |
| **Low**    | Resource type + AI      | Possible match  |
| **None**   | No match found          | Unmapped        |

The report displays the confidence level for each finding's component mapping so users know how
reliable the mapping is.

### Dual-Path Summary

```text
Finding arrives with resource ARN
        │
        ▼
   Look up resource tags
        │
   ┌────┴────┐
   │ Has     │ No atmos:*
   │ atmos:* │ tags found
   │ tags    │
   └────┬────┘────────────────────────────────────┐
        │                                          │
        ▼                                          ▼
  Path A: Instant                    Path B: Heuristic Pipeline
  tag-based mapping                         │
        │                          ┌────────┼────────┐
        │                          ▼        ▼        ▼
        │                     State    Naming    Resource
        │                     search   pattern   type index
        │                          │        │        │
        │                          └────┬───┘────────┘
        │                               │
        │                     Any confident match?
        │                          │           │
        │                         Yes          No
        │                          │           │
        │                          ▼           ▼
        │                     Use best    AI inference
        │                     match       (AI provider)
        │                          │           │
        │                          │      Match found?
        │                          │      │         │
        │                          │     Yes        No
        │                          │      │         │
        ▼                          ▼      ▼         ▼
  Component + Stack          Component   Match   Unmapped
  (exact confidence)         + Stack     + Stack (suggest
                             (high/med)  (low)    adding tags)
```

---

## Authentication: Atmos Auth + AWS

All AWS API calls use Atmos Auth for credential management. This provides a unified authentication
experience — the same credentials used for `atmos terraform apply` are used for security scanning.
When the `--ai` flag is used, AI analysis uses whatever provider is configured in `ai.default_provider`.

### Required AWS Permissions

The IAM role/user needs these permissions:

```json
{
  "Version": "2012-10-17",
  "Statement": [
    {
      "Sid": "SecurityFindings",
      "Effect": "Allow",
      "Action": [
        "securityhub:GetFindings",
        "securityhub:BatchGetSecurityControls",
        "securityhub:ListSecurityControlDefinitions",
        "config:GetComplianceDetailsByResource",
        "config:DescribeComplianceByResource",
        "config:SelectAggregateResourceConfig",
        "inspector2:ListFindings",
        "inspector2:GetFindingsReportStatus",
        "guardduty:ListFindings",
        "guardduty:GetFindings"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ResourceTagLookup",
      "Effect": "Allow",
      "Action": [
        "tag:GetResources",
        "tag:GetTagKeys",
        "tag:GetTagValues"
      ],
      "Resource": "*"
    }
  ]
}
```

> **Note:** When using AWS Bedrock as the AI provider, additional `bedrock:InvokeModel` permissions
> are required. See [AWS Bedrock AI Provider](aws-bedrock-ai-provider.md) for details.

### Atmos Auth Configuration

```yaml
# atmos.yaml
auth:
  providers:
    aws-security:
      type: aws-sso
      config:
        sso_start_url: "https://myorg.awsapps.com/start"
        sso_region: "us-east-1"
  identities:
    security-audit:
      provider: aws-security
      type: permission-set
      config:
        permission_set: "SecurityAudit"
        account_id: "123456789012"
```

---

## AI Provider Configuration

Atmos AI Security & Compliance works with **all AI providers supported by Atmos AI**. AI is
opt-in via the `--ai` flag — commands work without any AI provider configured.

| Provider         | Best For                                            | Data Residency        |
|------------------|-----------------------------------------------------|-----------------------|
| **Anthropic**    | Best overall quality (Claude direct API)             | Anthropic servers     |
| **OpenAI**       | Organizations standardized on GPT models             | OpenAI servers        |
| **Google Gemini** | Large context windows, cost-effective               | Google Cloud          |
| **Azure OpenAI** | Enterprise Azure customers                           | Your Azure tenant     |
| **AWS Bedrock**  | Enterprise/compliance — data stays in-account        | Your AWS account      |
| **Ollama**       | Air-gapped/offline, no external API calls            | Your infrastructure   |
| **Grok (xAI)**   | Alternative provider                                 | xAI servers           |

For enterprise data residency requirements, see the
[AWS Bedrock AI Provider](aws-bedrock-ai-provider.md) reference (setup, Terraform component,
pricing, regional availability).

### Configuration Example

```yaml
# atmos.yaml
ai:
  enabled: true
  default_provider: "anthropic"
  providers:
    anthropic:
      model: "claude-sonnet-4-6"
      api_key: !env "ANTHROPIC_API_KEY"
      max_tokens: 8192
    openai:
      model: "gpt-4o"
      api_key: !env "OPENAI_API_KEY"
    gemini:
      model: "gemini-2.5-flash"
      api_key: !env "GEMINI_API_KEY"
    ollama:
      model: "llama3.3:70b"
      base_url: "http://localhost:11434/v1"
```

By default, `atmos aws security` works without AI. The `--ai` flag enables AI-powered analysis
when a provider is configured. This opt-in design means CI/CD pipelines get structured finding
data with zero AI cost by default.

---

## CLI Commands

### `atmos aws security`

Primary command for security analysis. Fetches findings, maps to components, and generates reports.

```shell
# Analyze findings for a specific stack
atmos aws security --stack prod-us-east-1

# Analyze findings for a specific component in a stack
atmos aws security --stack prod-us-east-1 --component vpc

# Filter by severity
atmos aws security --stack prod-us-east-1 --severity critical,high

# Filter by finding source
atmos aws security --stack prod-us-east-1 --source security-hub

# Filter by compliance framework
atmos aws security --stack prod-us-east-1 --framework cis-aws,pci-dss

# Analyze all stacks
atmos aws security

# Output as JSON (for piping to dashboards, ticketing systems, etc.)
atmos aws security --stack prod-us-east-1 --format json

# Output as YAML
atmos aws security --stack prod-us-east-1 --format yaml

# Output as CSV (for spreadsheets, compliance reporting)
atmos aws security --stack prod-us-east-1 --format csv

# Pipe JSON to jq for filtering
atmos aws security --stack prod-us-east-1 --format json | jq '.findings[] | select(.severity == "CRITICAL")'

# Feed into Slack notification
atmos aws security --stack prod-us-east-1 --format json | notify-slack --channel security-alerts

# Generate CSV for compliance audit trail
atmos aws security --format csv > findings-$(date +%Y-%m-%d).csv
```

#### Flags

| Flag             | Type   | Default          | Description                                                                |
|------------------|--------|------------------|----------------------------------------------------------------------------|
| `--stack`        | string | (all stacks)     | Target stack to analyze                                                    |
| `--component`    | string | (all components) | Target component within the stack                                          |
| `--severity`     | string | `critical,high`  | Comma-separated severity filter                                            |
| `--source`       | string | `all`            | Finding source: `security-hub`, `config`, `inspector`, `guardduty`, `all`  |
| `--framework`    | string | (all)            | Compliance framework filter: `cis-aws`, `pci-dss`, `soc2`, `hipaa`, `nist` |
| `--format`       | string | `markdown`       | Output format: `markdown`, `json`, `yaml`, `csv`                           |
| `--max-findings` | int    | `50`             | Maximum findings to analyze (AI cost control)                              |
| `--ai`           | bool   | `false`          | Enable AI-powered analysis (requires `ai.enabled: true`)                   |
| `--region`       | string | (from stack)     | AWS region override                                                        |

#### Output: Markdown Report

The default output is a rich Markdown report rendered in the terminal:

```text
# Security Report: prod-us-east-1

**Generated:** 2026-03-09 14:30:00 UTC
**Stack:** prod-us-east-1
**Findings:** 12 (4 Critical, 5 High, 3 Medium)

---

## Critical Findings

### 1. S3 Bucket Public Access Enabled

| Field          | Value                                              |
|----------------|----------------------------------------------------|
| **Severity**   | CRITICAL                                           |
| **Source**      | Security Hub (CIS AWS 2.1.2)                      |
| **Resource**    | arn:aws:s3:::acme-prod-data-bucket                |
| **Component**   | s3-bucket                                         |
| **Stack**       | prod-us-east-1                                    |
| **File**        | components/terraform/s3-bucket/main.tf:42         |

#### Finding Details

The S3 bucket `acme-prod-data-bucket` has public access enabled via its
bucket policy. This allows unauthenticated read access to all objects.

#### Root Cause

In `components/terraform/s3-bucket/main.tf`, the `block_public_access`
settings are not fully enabled:

    resource "aws_s3_bucket_public_access_block" "this" {
      bucket                  = aws_s3_bucket.this.id
      block_public_acls       = true
      block_public_policy     = false  # <-- This should be true
      ignore_public_acls      = true
      restrict_public_buckets = false  # <-- This should be true
    }

#### Remediation

**Step 1:** Update the component variable in the stack config:

    # stacks/deploy/prod/us-east-1/s3-bucket.yaml
    components:
      terraform:
        s3-bucket:
          vars:
            block_public_policy: true       # Changed from false
            restrict_public_buckets: true   # Changed from false

**Step 2:** If the component doesn't expose these variables, update the
Terraform source:

    # components/terraform/s3-bucket/main.tf (line 42)
    resource "aws_s3_bucket_public_access_block" "this" {
      bucket                  = aws_s3_bucket.this.id
      block_public_acls       = true
      block_public_policy     = true       # Changed
      ignore_public_acls      = true
      restrict_public_buckets = true       # Changed
    }

**Step 3:** Deploy the fix:

    atmos terraform apply s3-bucket -s prod-us-east-1

---

## Summary

| Severity | Count | Mapped to Component | Unmapped |
|----------|-------|---------------------|----------|
| Critical | 4     | 3                   | 1        |
| High     | 5     | 4                   | 1        |
| Medium   | 3     | 3                   | 0        |
| **Total**| **12**| **10**              | **2**    |

> 2 findings could not be mapped to Atmos components. These resources may
> be managed outside of Atmos or may be missing `atmos:*` tags.
```

### `atmos aws compliance`

Generates compliance posture reports against specific frameworks.

```shell
# CIS AWS Foundations Benchmark report
atmos aws compliance --framework cis-aws --stack prod-us-east-1

# PCI DSS compliance status
atmos aws compliance --framework pci-dss

# All frameworks
atmos aws compliance --stack prod-us-east-1
```

#### Flags

| Flag          | Type   | Default      | Description                                              |
|---------------|--------|--------------|----------------------------------------------------------|
| `--stack`     | string | (all stacks) | Target stack                                             |
| `--framework` | string | (all)        | Framework: `cis-aws`, `pci-dss`, `soc2`, `hipaa`, `nist` |
| `--format`    | string | `markdown`   | Output format: `markdown`, `json`, `yaml`, `csv`         |
| `--controls`  | string | (all)        | Specific control IDs to check                            |

#### Output

```text
# Compliance Report: CIS AWS Foundations Benchmark v3.0

**Stack:** prod-us-east-1
**Framework:** CIS AWS Foundations Benchmark v3.0
**Date:** 2026-03-09

## Score: 87/100 Controls Passing (87%)

### Failing Controls

| Control   | Title                              | Severity | Component     | Remediation Available |
|-----------|------------------------------------|----------|---------------|-----------------------|
| CIS 2.1.2 | S3 bucket public access           | Critical | s3-bucket     | Yes                   |
| CIS 2.2.1 | EBS encryption default            | High     | ebs-defaults  | Yes                   |
| CIS 3.1   | CloudTrail enabled                | High     | cloudtrail    | Yes                   |

### Remediation Details
(AI-generated remediation for each failing control, same format as security report)
```

---

## Output Formats

All commands support the `--format` flag for different use cases. The default is `markdown` for
interactive CLI use; structured formats are designed for automation and CI/CD integration.

### `markdown` (default)

Rich Markdown rendered in the terminal with colors, tables, and code blocks. Best for human
review in interactive sessions. Uses `pkg/ui/` Markdown rendering with theme-aware colors.

### `json`

Structured JSON for programmatic consumption. Each finding is a JSON object with consistent
field names:

```json
{
  "report": {
    "generated_at": "2026-03-09T14:30:00Z",
    "stack": "prod-us-east-1",
    "total_findings": 12,
    "severity_counts": {
      "CRITICAL": 4,
      "HIGH": 5,
      "MEDIUM": 3
    }
  },
  "findings": [
    {
      "id": "arn:aws:securityhub:us-east-1:123456789012:finding/abc123",
      "title": "S3 Bucket Public Access Enabled",
      "severity": "CRITICAL",
      "source": "security-hub",
      "compliance_standard": "CIS AWS 2.1.2",
      "resource_arn": "arn:aws:s3:::acme-prod-data-bucket",
      "resource_type": "AwsS3Bucket",
      "mapping": {
        "stack": "prod-us-east-1",
        "component": "s3-bucket",
        "component_path": "components/terraform/s3-bucket",
        "mapped": true
      },
      "remediation": {
        "description": "Enable block_public_policy and restrict_public_buckets",
        "stack_changes": {
          "block_public_policy": true,
          "restrict_public_buckets": true
        },
        "deploy_command": "atmos terraform apply s3-bucket -s prod-us-east-1"
      }
    }
  ]
}
```

Use cases: dashboards, ticketing systems (Jira, Linear), custom automation, Slack/PagerDuty
notifications, data lakes.

### `yaml`

Same structure as JSON but in YAML format. Useful for Atmos/Terraform-native workflows where
YAML is the standard format.

### `csv`

Flat tabular format for spreadsheets and compliance auditing tools. One row per finding:

```csv
id,title,severity,source,resource_arn,resource_type,stack,component,mapped,remediation
abc123,S3 Bucket Public Access Enabled,CRITICAL,security-hub,arn:aws:s3:::acme-prod-data-bucket,AwsS3Bucket,prod-us-east-1,s3-bucket,true,Enable block_public_policy
def456,EBS Default Encryption Disabled,HIGH,security-hub,arn:aws:ec2:us-east-1:123456789012:volume/vol-xyz,AwsEc2Volume,prod-us-east-1,ebs-defaults,true,Set encrypted = true
```

Use cases: compliance audit trails, Excel/Google Sheets analysis, import into GRC tools,
historical tracking.

---

## New Atmos AI Tools

These tools are registered in the AI tool system and are available to both the CLI commands and
MCP clients.

### `atmos_list_findings`

List security findings from AWS security services, optionally filtered by stack.

```go
Name: "atmos_list_findings"

Parameters:
- stack (string, optional): Filter by stack name using resource tags
- component (string, optional): Filter by component name
- severity (string, optional): Filter by severity (CRITICAL, HIGH, MEDIUM, LOW)
- source (string, optional): Finding source (security-hub, config, inspector, guardduty)
- max_results (integer, optional): Maximum findings to return (default: 50)

Returns:
- List of findings with resource ARN, severity, title, source, and mapped component/stack
```

### `atmos_describe_finding`

Get detailed information about a specific finding, including resource context and component mapping.

```go
Name: "atmos_describe_finding"

Parameters:
- finding_id (string, required): Finding ID or ARN
- source (string, optional): Finding source if ambiguous

Returns:
- Full finding details, affected resource, resource tags, mapped component/stack,
component source file path, relevant Terraform configuration
```

### `atmos_analyze_finding`

Use AI to analyze a finding and generate remediation recommendations.

```go
Name: "atmos_analyze_finding"

Parameters:
- finding_id (string, required): Finding ID or ARN
- component (string, optional): Override component mapping
- stack (string, optional): Override stack mapping

Returns:
- AI-generated root cause analysis, remediation steps with code changes,
deploy commands, and risk assessment
```

### `atmos_compliance_report`

Generate a compliance posture report for a framework.

```go
Name: "atmos_compliance_report"

Parameters:
- framework (string, required): Compliance framework (cis-aws, pci-dss, soc2, hipaa, nist)
- stack (string, optional): Filter by stack
- format (string, optional): Output format (markdown, json)

Returns:
- Compliance score, passing/failing controls, failing control details with remediation
```

---

## Finding-to-Code Mapping: Implementation Details

This section describes the technical implementation of the mapping algorithm. For the high-level
dual-path architecture, see [Finding-to-Code Mapping: Two Paths](#finding-to-code-mapping-two-paths).

### Resource Identifier Extraction

Each AWS security service returns findings with resource identifiers in different formats:

- **Security Hub:** `Resources[].Id` (ARN) — normalized ASFF format
- **Config:** `ResourceId` + `ResourceType` — may be ARN or resource-specific ID
- **Inspector:** `Resources[].Id` (ARN) + package/network details
- **GuardDuty:** `Resource.InstanceDetails`, `Resource.S3BucketDetails`, `Resource.EksClusterDetails`, etc.
- **Macie:** `ResourcesAffected.S3Bucket` + `S3Object` details
- **Access Analyzer:** `Resource` (ARN) + `ResourceType`

The system normalizes all identifiers to ARNs where possible. For resources without ARNs (e.g.,
GuardDuty IP-based findings), it uses account + region + resource type + identifier as a composite key.

### Component Resolution

Once a component/stack candidate is identified (via Path A or Path B), resolve the full context:

```
atmos describe component <component> -s <stack>
```

This provides:

- Component source path (`components/terraform/s3-bucket/`)
- Stack configuration (vars, settings, overrides, inheritance chain)
- Terraform workspace name
- Backend configuration (for state file access)

### Source Code Reading

Read the relevant Terraform files for the matched component:

```
read_component_file(component: "s3-bucket", file: "main.tf")
read_component_file(component: "s3-bucket", file: "variables.tf")
read_component_file(component: "s3-bucket", file: "outputs.tf")
```

### AI Analysis

Send to the configured AI provider for root cause analysis and remediation:

**Input context:**

- Finding details (type, severity, affected resource, compliance standard)
- Component Terraform source code (main.tf, variables.tf)
- Stack variable configuration (current vars and overrides)
- Mapping confidence level and method used
- For tagless matches: explanation of how the match was inferred

**AI returns:**

- Root cause explanation tied to specific lines of code
- Whether the fix should be in stack vars (preferred) or component source
- Specific code changes with before/after diffs
- `atmos terraform apply` deployment command
- Risk assessment of the change (blast radius, potential side effects)
- If mapping confidence is low: caveats and suggestion to verify the match

---

## Configuration

### `atmos.yaml` Configuration

```yaml
# atmos.yaml
ai:
  enabled: true
  default_provider: "anthropic"  # or "bedrock", "openai", "gemini", "azureopenai", "ollama", "grok"

  providers:
    # Direct Anthropic API (recommended for general use)
    anthropic:
      model: "claude-sonnet-4-6"
      api_key: !env "ANTHROPIC_API_KEY"
      max_tokens: 8192

    # Other providers (configure as needed)
    # See provider-specific docs for setup:
    # - AWS Bedrock: docs/prd/aws-bedrock-ai-provider.md
    # openai:
    #   model: "gpt-4o"
    #   api_key: !env "OPENAI_API_KEY"
    # gemini:
    #   model: "gemini-2.5-flash"
    #   api_key: !env "GEMINI_API_KEY"
    # ollama:
    #   model: "llama3.3:70b"
    #   base_url: "http://localhost:11434/v1"

  tools:
    enabled: true

# AWS-specific settings
aws:
  # Security & compliance settings
  security:
    enabled: true

    # AWS security services to query
    sources:
      security_hub: true
      config: true
      inspector: true
      guardduty: true

    # Default severity filter
    default_severity:
      - CRITICAL
      - HIGH

    # Maximum findings per analysis run
    max_findings: 50

    # Tag keys used for finding-to-code mapping
    tag_mapping:
      stack_tag: "atmos:stack"
      component_tag: "atmos:component"
      tenant_tag: "atmos:tenant"
      environment_tag: "atmos:environment"
      stage_tag: "atmos:stage"

    # Compliance frameworks to track
    frameworks:
      - cis-aws
      - pci-dss
```

---

## Implementation Plan

### Phase 1: Foundation — DONE

1. **Schema additions** — ✅ Added `aws.security` section to `pkg/schema/aws.go` and
   `pkg/schema/aws_security.go` (`AWSSettings`, `AWSSecuritySettings`, `AWSSecuritySources`,
   `AWSSecurityTagMapping`). Added `AWS AWSSettings` field to `AtmosConfiguration` in `pkg/schema/schema.go`.
2. **AWS security client** — ✅ Created `pkg/aws/security/` package (moved from `pkg/ai/security/`).
   Contains: `aws_clients.go` (Security Hub, Config, Inspector, GuardDuty API clients),
   `finding_fetcher.go`, `component_mapper.go`, `analyzer.go`, `report_renderer.go`, `cache.go`, `types.go`.
3. **Tag-based mapping** — ✅ Implemented in `pkg/aws/security/component_mapper.go`. Supports
   `atmos:stack`, `atmos:component` tags via Resource Groups Tagging API (Path A) and heuristic
   naming convention matching (Path B).
4. **CLI command scaffold** — ✅ Registered `atmos aws security` and `atmos aws compliance` commands
   using the command registry pattern in `cmd/aws/security.go` and `cmd/aws/compliance.go`.
   Uses `flags.NewStandardParser()` for flag handling per mandatory patterns.

### Phase 2: Core Analysis — DONE

5. **Finding fetcher** — ✅ Implemented in `pkg/aws/security/finding_fetcher.go`. Retrieves findings
   from Security Hub (primary) with support for Config, Inspector, GuardDuty sources. Includes
   severity filtering, source filtering, and max-findings limits.
6. **Component mapper** — ✅ Implemented in `pkg/aws/security/component_mapper.go`. Dual-path
   mapping: tag-based (Path A) and heuristic naming convention (Path B). Includes confidence levels
   (Exact, High, Medium, Low, None).
7. **Report generator** — ✅ Implemented in `pkg/aws/security/report_renderer.go`. Supports all
   four output formats: Markdown (terminal), JSON, YAML, CSV.
8. **AI provider integration** — ✅ Implemented in `pkg/aws/security/analyzer.go`. Uses configured
   AI provider for finding analysis. AI is opt-in via `--ai` global flag.

### Phase 3: AI Tools — DONE

9. **`atmos_list_findings` tool** — ✅ Registered in `pkg/ai/tools/atmos/list_findings.go`
10. **`atmos_describe_finding` tool** — ✅ Registered in `pkg/ai/tools/atmos/describe_finding.go`
11. **`atmos_analyze_finding` tool** — ✅ Registered in `pkg/ai/tools/atmos/analyze_finding.go`
12. **`atmos_compliance_report` tool** — ✅ Registered in `pkg/ai/tools/atmos/compliance_report.go`

All AI tools use error builder pattern (`errUtils.Build(sentinel).WithHint().WithExitCode().Err()`)
for error handling and reference `aws.security.enabled` config.

### Phase 4: Polish — DONE

13. **JSON/YAML/CSV output** — ✅ All four formats implemented in `report_renderer.go`
14. **Caching** — ✅ Cache infrastructure implemented in `pkg/aws/security/cache.go`
15. **Documentation** — ✅ Docusaurus docs created:
    - `website/docs/cli/commands/aws/security.mdx` — CLI command reference
    - `website/docs/cli/commands/aws/compliance.mdx` — CLI command reference
    - `website/docs/cli/configuration/aws/index.mdx` — AWS config overview
    - `website/docs/cli/configuration/aws/security.mdx` — Security config reference
    - `website/docs/cli/global-flags.mdx` — Updated with `--ai` global flag
    - `cmd/aws/markdown/atmos_aws_security.md` — Embedded help text
    - `cmd/aws/markdown/atmos_aws_compliance.md` — Embedded help text
16. **Tests** — ✅ Unit tests with mocks:
    - `cmd/aws/security_test.go` — 37 tests (parse functions, report building, subcommand registration)
    - `cmd/aws/compliance_test.go` — 9 tests (framework validation, subcommand registration)
    - `pkg/aws/security/finding_fetcher_test.go` — Finding fetcher tests with mocked AWS clients
    - `pkg/aws/security/component_mapper_test.go` — Mapper tests
    - `pkg/aws/security/report_renderer_test.go` — Renderer tests
    - `pkg/aws/security/analyzer_test.go` — AI analyzer tests
    - `pkg/aws/security/cache_test.go` — Cache tests
    - Coverage: `pkg/aws/security/` at 91.8%, `cmd/aws/` at 33.5%

### Phase 5: Global AI Flag — DONE

17. **`--ai` as global persistent flag** — ✅ Added `AI bool` field to `pkg/flags/global/flags.go`
    `Flags` struct. Registered via `registerAIFlags()` in `pkg/flags/global_builder.go` with
    `ATMOS_AI` env var support. Removed command-local `--ai` from `cmd/aws/security.go`.
    The flag is now inherited by all subcommands. Currently consumed by `atmos aws security`;
    future PR will extend to all commands for AI-powered output analysis.

### Remaining Work (Future PRs)

- **Global `--ai` integration with all commands** — When `--ai` is passed to any command, send
  command output to AI provider for analysis/description. Requires intercepting command output
  in `cmd/root.go` PersistentPostRun.
- **Terraform state search (Path B Strategy 1)** — Implement state file scanning for tagless
  mapping. Reuse `!terraform.state` infrastructure.
- **AI-assisted inference (Path B Strategy 4)** — Send unmapped findings to AI for component
  inference when heuristic strategies fail.
- **Integration tests** — End-to-end tests with real AWS API calls (requires test account).
- **`cmd/aws/` coverage improvement** — RunE handlers need integration-level tests to improve
  from 33.5% coverage.
- **Interactive AI Q&A** — `atmos ai chat` with security context for follow-up questions about
  findings.

---

## Error Handling Strategy

All errors use static sentinel errors defined in `errors/errors.go` and follow the error builder
pattern per Atmos conventions.

### Sentinel Errors

The following sentinel errors are defined in `errors/errors.go` (lines 946-955):

| Sentinel Error                    | Message                               | When Used                                      |
|-----------------------------------|---------------------------------------|-------------------------------------------------|
| `ErrAISecurityNotEnabled`         | `security feature not enabled`        | `aws.security.enabled` is false in config       |
| `ErrAISecurityNoFindings`         | `no matching findings returned`       | AWS APIs return zero findings for the query      |
| `ErrAISecurityFetchFailed`        | `failed to fetch from AWS`            | AWS Security Hub/Config/Inspector API errors     |
| `ErrAISecurityMappingFailed`      | `failed to map finding to component`  | Component mapping pipeline fails                 |
| `ErrAISecurityInvalidSeverity`    | `invalid severity filter value`       | User passes unknown severity (not CRITICAL/HIGH/MEDIUM/LOW) |
| `ErrAISecurityInvalidSource`      | `invalid finding source value`        | User passes unknown source (not security-hub/config/inspector/guardduty) |
| `ErrAISecurityInvalidFramework`   | `invalid compliance framework value`  | User passes unknown framework                    |
| `ErrAISecurityInvalidFormat`      | `invalid output format value`         | User passes unknown format (not markdown/json/yaml/csv) |
| `ErrAISecurityAnalysisFailed`     | `AI analysis failed`                  | AI provider returns an error during analysis     |

### Error Wrapping Pattern

All errors in `pkg/aws/security/` wrap sentinel errors with context using `fmt.Errorf`:

```go
// Wrapping with sentinel + underlying error.
return fmt.Errorf("%w: %w", errUtils.ErrAISecurityFetchFailed, err)

// Wrapping with sentinel + additional context.
return fmt.Errorf("%w: stack=%s source=%s: %w", errUtils.ErrAISecurityFetchFailed, stack, source, err)
```

This enables callers to check error types with `errors.Is()`:

```go
if errors.Is(err, errUtils.ErrAISecurityNotEnabled) {
    // Feature is disabled — show configuration hint.
}
```

### Error Builder Usage

AI tools in `pkg/ai/tools/atmos/` use the error builder pattern for user-facing errors:

```go
return errUtils.Build(errUtils.ErrAISecurityNotEnabled).
    WithHint("Enable AWS security in atmos.yaml: aws.security.enabled: true").
    WithExitCode(1).
    Err()
```

---

## Testing Strategy

### Mock-Based Unit Testing

All AWS SDK interactions use interfaces defined in `pkg/aws/security/` for testability:

| Interface          | File                    | Purpose                                    |
|--------------------|-------------------------|--------------------------------------------|
| `SecurityHubAPI`   | `aws_clients.go`        | Security Hub API operations (GetFindings, etc.) |
| `TaggingAPI`       | `aws_clients.go`        | Resource Groups Tagging API (tag-based mapping) |
| `FindingFetcher`   | `finding_fetcher.go`    | High-level findings retrieval              |
| `ComponentMapper`  | `component_mapper.go`   | Resource-to-component mapping              |
| `FindingAnalyzer`  | `analyzer.go`           | AI-powered analysis                        |
| `ReportRenderer`   | `report_renderer.go`    | Report formatting (Markdown/JSON/YAML/CSV) |

Mock generation uses `go.uber.org/mock/mockgen` with `//go:generate` directives:

```go
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_aws_clients.go -package=security
```

### Test Files and Coverage

| Test File                            | Tests | Coverage | Notes                                    |
|--------------------------------------|-------|----------|------------------------------------------|
| `pkg/aws/security/finding_fetcher_test.go`  | Yes   | ~92%     | Mocked AWS SDK clients                  |
| `pkg/aws/security/component_mapper_test.go` | Yes   | ~90%     | Tag-based and heuristic mapping paths   |
| `pkg/aws/security/report_renderer_test.go`  | Yes   | ~95%     | All four output formats                 |
| `pkg/aws/security/analyzer_test.go`         | Yes   | ~88%     | Manual mock `mockAIClient` for AI provider |
| `pkg/aws/security/cache_test.go`            | Yes   | ~90%     | Cache TTL, invalidation, concurrency    |
| `cmd/aws/security_test.go`                  | 37    | ~34%     | Parse functions, report building, subcommand registration |
| `cmd/aws/compliance_test.go`                | 9     | ~34%     | Framework validation, subcommand registration |

**Overall coverage:** `pkg/aws/security/` at 91.8%, `cmd/aws/` at 33.5%.

### Testing Approach

- **Unit tests with mocks** for all AWS API interactions (no real AWS calls in CI).
- **Table-driven tests** for input validation (severity, source, framework, format parsing).
- **Manual mock implementations** where `mockgen` mocks don't exist yet (e.g., `mockAIClient`
  in `analyzer_test.go` satisfies the `registry.Client` interface).
- **Integration tests** (future) will require a dedicated test AWS account with Security Hub
  enabled and sample findings seeded.

---

## Security Considerations

- **Data residency options** — Choose the AI provider that matches your security requirements.
  Some providers (e.g., AWS Bedrock, Ollama) keep data within your own infrastructure, while others
  (Anthropic, OpenAI, etc.) send finding data to the provider's API for analysis.
  See [AWS Bedrock AI Provider](aws-bedrock-ai-provider.md) for enterprise data residency setup.
- **Atmos Auth credentials** — All AWS access uses Atmos Auth; no hardcoded keys
- **Read-only by default** — The security commands only read findings and source code; they never
  modify infrastructure. Users must manually apply remediation steps
- **AI cost control** — The `max_findings` setting limits how many findings are sent to AI for
  analysis, controlling costs across all providers
- **Audit trail** — Consult each provider's audit logging capabilities. For example, AWS Bedrock
  logs all invocations via CloudTrail. See provider-specific documentation for details.
- **AI is opt-in** — By default, no data is sent to any AI provider. The `--ai` flag must be
  explicitly passed to enable AI analysis. This ensures the commands work safely in environments
  where sending data to AI providers is not permitted.

---

## AWS Security Services Ecosystem

The following Cloud Posse Terraform components form the security and compliance infrastructure that
generates the findings Atmos AI analyzes. Understanding their architecture is essential for the
finding-to-code mapping system.

### Security Hub (Central Aggregation Point)

Security Hub is the **primary finding source** — it aggregates findings from all other security
services into a single pane.

- **Component:** `aws-security-hub`
- **Module:** `cloudposse/security-hub/aws`
- **Deployment:** 3-step delegated administrator (security account → root delegates → security configures org)
- **Key features:**
  - Cross-region finding aggregation into a single region
  - Organizations resource-based delegation policy
  - Enabled standards: AWS Foundational Security Best Practices v1.0.0, CIS AWS Foundations Benchmark v1.4.0
  - Product subscriptions: GuardDuty, Inspector, Config, IAM Access Analyzer
  - SNS/EventBridge integration for automated response
- **Finding format:** ASFF (AWS Security Finding Format) with severity levels: Critical, High, Medium, Low,
  Informational

### AWS Config (Resource Compliance)

- **Component:** `aws-config` (member accounts), `aws-config-bucket` (audit account)
- **Module:** `cloudposse/aws-config`
- **Deployment:** Per-account with central aggregation (member accounts FIRST, then org root LAST)
- **Key features:**
  - Conformance packs: CIS AWS Foundations Benchmark v1.4 Level 2
  - Configuration compliance: COMPLIANT / NON_COMPLIANT per resource
  - Security account acts as central aggregator
  - Known false positives documented (e.g., `IAM_NO_INLINE_POLICY_CHECK` on Service-Linked Roles)
- **Finding types:** Config rule compliance evaluations, conformance pack results

### GuardDuty (Threat Detection)

- **Component:** `aws-guardduty`
- **Module:** `cloudposse/guardduty/aws`
- **Deployment:** 3-step delegated administrator
- **Key features:**
  - ML-based anomaly detection from CloudTrail, VPC Flow Logs, DNS logs
  - 7-14 day ML baseline learning period (no findings initially is normal)
  - Protection features: S3 Data Events, Kubernetes audit logs, Lambda network, EC2/EKS runtime monitoring, Malware
    Protection, RDS Login Events
  - Auto-enable for all organization member accounts
- **Finding types:** Reconnaissance, instance compromise, credential compromise, S3 compromise, Kubernetes findings,
  malware findings

### Inspector v2 (Vulnerability Scanning)

- **Component:** `aws-inspector2`
- **Deployment:** 2-step delegated administrator (simpler than GuardDuty/Security Hub)
- **Key features:**
  - Package vulnerability scanning (CVE-based) for EC2, ECR, Lambda
  - Network reachability analysis (publicly accessible instances, security group/NACL analysis)
  - Inspector Risk Score (CVSS + network accessibility + exploitability)
  - Requires SSM Agent on EC2 instances (no additional agent)
- **Finding types:** Package vulnerabilities with CVE, severity, installed vs fixed version, remediation

### IAM Access Analyzer

- **Component:** `aws-access-analyzer`
- **Deployment:** 2-step delegated administrator
- **Key features:**
  - External access analyzer (type: ORGANIZATION): detects resources shared externally
  - Unused access analyzer (type: ORGANIZATION_UNUSED_ACCESS): detects unused IAM permissions
  - Covers: S3, IAM, KMS, Lambda, SQS, Secrets Manager, SNS, EBS, RDS, ECR, EFS
  - Policy generation from CloudTrail logs
- **Finding types:** External access (public, cross-account), unused access (roles, permissions, actions)

### CloudTrail (Audit Logging)

- **Component:** `cloudtrail` (trail), `cloudtrail-bucket` (S3 bucket in audit account)
- **Module:** `cloudposse/cloudtrail`, `cloudposse/cloudtrail-s3-bucket`
- **Deployment:** Organization trail from management account (single trail covers all accounts/regions)
- **Key features:**
  - Multi-region, organization-wide trail with log file validation
  - KMS encryption, S3 lifecycle (Standard → IA → Glacier → Delete)
  - CloudWatch Logs integration for real-time monitoring
  - Insight events for unusual activity detection
- **Role:** Foundation service — provides data to GuardDuty, Config, Access Analyzer, Audit Manager

### Audit Manager (Compliance Assessment)

- **Component:** `aws-audit-manager`, `s3-bucket/aws-audit-manager` (report storage)
- **Deployment:** 2-step delegated administrator (security FIRST, root LAST)
- **Key features:**
  - Continuous compliance evidence collection from CloudTrail, Config, Security Hub
  - Prebuilt frameworks: PCI DSS, HIPAA, SOC 2, NIST 800-53, FedRAMP, GDPR, ISO 27001, CIS, CMMC
  - Cryptographically verified assessment reports
- **Role:** Aggregates evidence for compliance audits; not a findings generator itself

### Macie (Data Security)

- **Component:** `aws-macie`
- **Deployment:** 3-step delegated administrator (same pattern as GuardDuty/Security Hub)
- **Key features:**
  - S3-focused sensitive data discovery using ML and pattern matching
  - Detects PII, financial data, government IDs, PHI, credentials
  - Policy findings: public access, encryption disabled, external sharing
- **Finding types:** Policy findings (S3 misconfigurations), sensitive data findings (PII, credentials, financial)

### Shield Advanced (DDoS Protection)

- **Component:** `aws-shield`
- **Deployment:** Per-account, per-resource (fundamentally different from org-wide services)
- **Key features:**
  - Protects ALBs, CloudFront, EIPs, Route53 zones
  - DDoS Response Team (DRT) support
  - DDoS cost protection
  - Dual-scope: Global (Route53/CloudFront) + Regional (ALBs/EIPs)
- **Finding types:** DDoS attack events with CloudWatch metrics

### WAF (Web Application Firewall)

- **Component:** `waf`
- **Module:** `cloudposse/waf/aws`
- **Deployment:** Per-scope (Regional for ALBs, CloudFront for CDN)
- **Key features:**
  - AWS Managed Rules: OWASP Top 10, Bot Control
  - Custom rules: rate-based, geo-match, IP allowlisting, byte match
  - CloudFront WAF: deny-by-default with IP allowlists (Cloudflare, VPN, office)
  - Cross-component SSM parameter store for WAF ACL ARN sharing
- **Role:** Blocking/filtering service; complements Shield at Layer 7

### Route53 Resolver DNS Firewall

- **Component:** `aws-route53-resolver-dns-firewall`, `s3-bucket` (DNS query logs)
- **Deployment:** Per-VPC in platform accounts
- **Key features:**
  - DNS-level domain filtering (ALLOW, BLOCK, ALERT)
  - Per-environment domain allowlists
  - Alert mode (monitor-first) → Block mode (after tuning)
- **Role:** DNS-level defense-in-depth; complements WAF/Shield at the network layer

### Service Deployment Architecture

All security services follow a multi-account pattern with the security account as the delegated
administrator and the audit account for centralized logging:

```text
┌──────────────┐     ┌──────────────┐     ┌──────────────┐
│  Management  │     │   Security   │     │    Audit     │
│   (Root)     │     │   Account    │     │   Account    │
│              │     │              │     │              │
│ - Delegates  │────▶│ - Admin for: │     │ - CloudTrail │
│   admin to   │     │   SecHub     │     │   bucket     │
│   security   │     │   GuardDuty  │     │ - Config     │
│              │     │   Inspector  │     │   bucket     │
│ - Org trail  │     │   Macie      │     │ - Audit Mgr  │
│ - Org config │     │   AccessAnlz │     │   reports    │
│              │     │   AuditMgr   │     │              │
└──────────────┘     └──────────────┘     └──────────────┘
                            │
                     ┌──────┴──────┐
                     ▼             ▼
              ┌───────────┐ ┌───────────┐
              │  Member   │ │  Member   │
              │ Account 1 │ │ Account N │
              │           │ │           │
              │ - Config  │ │ - Config  │
              │ - Shield  │ │ - Shield  │
              │ - WAF     │ │ - WAF     │
              │ - DNS FW  │ │ - DNS FW  │
              └───────────┘ └───────────┘
```

### Deployment Order

Security services must be deployed in this order:

1. **Foundation:** CloudTrail bucket → CloudTrail org trail
2. **Compliance Data:** Config bucket → Config (member accounts in parallel) → Config (root/org)
3. **Threat Detection:** GuardDuty (security → root → security), Inspector (root → security)
4. **Security Hub:** Security Hub (security → root → security) — last because it aggregates all others
5. **Access Analysis:** Access Analyzer (root → security)
6. **Data Security:** Macie (security → root → security)
7. **Audit:** Audit Manager (security → root)
8. **DDoS/App Protection:** Shield, WAF, DNS Firewall (per-resource, any order)

---

## Finding Source Priority

When querying findings, the system uses this priority to avoid duplicates (Security Hub aggregates
from other services):

1. **Security Hub** (preferred) — Query Security Hub first; it normalizes findings from all sources
   into the standard ASFF format with consistent severity scoring
2. **Direct service queries** — Only query Config, Inspector, GuardDuty, Macie directly when:

- Security Hub is not enabled
- More granular detail is needed (e.g., Inspector's CVE-specific remediation guidance)
- Real-time findings are needed (Security Hub has up to 15-minute delay)

### Finding Deduplication

Security Hub finding IDs contain the source service identifier. When merging findings from
multiple sources:

1. Group by resource ARN
2. Prefer Security Hub's normalized severity over source-specific severity
3. Merge additional context from direct service queries (e.g., Inspector's package fix versions)
4. Track finding source chain for auditability

---

## Dependencies

- **Atmos AI** — Provider system (Anthropic, OpenAI, Gemini, Azure OpenAI, Bedrock, Ollama, Grok), tool registry, Markdown rendering
- **Atmos Auth** — AWS credential management (SSO, assume-role)
- **AWS Security Hub** — Central finding aggregation (primary source)
- **AWS Config** — Resource compliance evaluations
- **Amazon Inspector v2** — Package vulnerability and network reachability findings
- **Amazon GuardDuty** — ML-based threat detection findings
- **Amazon Macie** — S3 data security and sensitive data findings
- **AWS IAM Access Analyzer** — External/unused access findings
- **AWS Resource Groups Tagging API** — Resource tag lookup for finding-to-code mapping
- **Resource tagging** — `atmos:*` tags on all managed resources

### Cloud Posse Terraform Components (Prerequisites)

The following components must be deployed for the security services to generate findings:

| Component                           | Required    | Purpose                    |
|-------------------------------------|-------------|----------------------------|
| `cloudtrail` + `cloudtrail-bucket`  | Yes         | Audit logging (foundation) |
| `aws-config` + `aws-config-bucket`  | Yes         | Resource compliance        |
| `aws-security-hub`                  | Yes         | Finding aggregation        |
| `aws-guardduty`                     | Yes         | Threat detection           |
| `aws-inspector2`                    | Recommended | Vulnerability scanning     |
| `aws-access-analyzer`               | Recommended | IAM access analysis        |
| `aws-macie`                         | Optional    | S3 data security           |
| `aws-audit-manager`                 | Optional    | Compliance evidence        |
| `aws-shield`                        | Optional    | DDoS protection            |
| `waf`                               | Optional    | Web application firewall   |
| `aws-route53-resolver-dns-firewall` | Optional    | DNS-level filtering        |

### Go Dependencies

- `github.com/aws/aws-sdk-go-v2/service/securityhub`
- `github.com/aws/aws-sdk-go-v2/service/configservice`
- `github.com/aws/aws-sdk-go-v2/service/inspector2`
- `github.com/aws/aws-sdk-go-v2/service/guardduty`
- `github.com/aws/aws-sdk-go-v2/service/macie2`
- `github.com/aws/aws-sdk-go-v2/service/accessanalyzer`
- `github.com/aws/aws-sdk-go-v2/service/resourcegroupstaggingapi`
- AI providers use existing Atmos AI provider SDKs (no additional dependencies needed)
