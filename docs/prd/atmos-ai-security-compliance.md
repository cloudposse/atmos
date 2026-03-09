# Atmos AI Security & Compliance - Product Requirements Document

**Status:** Draft
**Version:** 0.2
**Last Updated:** 2026-03-09

---

## Executive Summary

The main purpose of Atmos AI Security & Compliance is to make it **very easy and fast** for users to
detect, review, analyze, and ask questions about any security and compliance findings from all AWS
security and compliance services — directly from the Atmos CLI.

It connects to AWS Security Hub, AWS Config, Amazon Inspector, Amazon GuardDuty, Amazon Macie, and
IAM Access Analyzer via Atmos Auth, uses **any AI provider that Atmos AI supports** to analyze findings,
maps them back to Atmos components and stacks, and generates actionable remediation reports with concrete
Terraform/Atmos changes.

**Supported AI providers:** Anthropic (Claude), OpenAI (GPT), Google Gemini, Azure OpenAI, AWS Bedrock,
Ollama (local/on-premise), and Grok (xAI). Enterprise customers who require data residency within their
AWS account should use **AWS Bedrock** — all data stays in-account with no external API calls. All other
providers work equally well for AI analysis; the choice depends on your organization's security posture,
compliance requirements, and preference.

The key differentiator: Atmos owns the component-to-stack relationship, so it can trace a security
finding on an AWS resource all the way back to the exact Terraform code and stack configuration that
created it — and generate a targeted fix.

### Why This Matters

Today, reviewing security findings requires navigating multiple AWS console pages, cross-referencing
resources with Terraform code, and manually figuring out which configuration caused the issue. This is
slow, error-prone, and requires deep AWS + Terraform expertise.

With Atmos AI Security & Compliance, a single command replaces that entire workflow:

```shell
atmos ai security --stack prod-us-east-1
```

In seconds, the user gets a complete picture: what's wrong, which component caused it, exactly what
code to change, and how to deploy the fix. The AI handles the analysis; the user reviews and decides.

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
│                    atmos ai security <stack>                        │
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
AI analysis uses whatever provider is configured in `ai.default_provider` (Bedrock for enterprise
customers who need data residency, or any other supported provider).

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
    },
    {
      "Sid": "BedrockInvoke",
      "Effect": "Allow",
      "Action": [
        "bedrock:InvokeModel",
        "bedrock:InvokeModelWithResponseStream"
      ],
      "Resource": "arn:aws:bedrock:*::foundation-model/*",
      "Comment": "Only required when using Bedrock as the AI provider"
    }
  ]
}
```

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
    # Only needed when using Bedrock as the AI provider
    bedrock-user:
      provider: aws-security
      type: permission-set
      config:
        permission_set: "BedrockUser"
        account_id: "123456789012"
```

---

## AI Provider Configuration

Atmos AI Security & Compliance works with **all AI providers supported by Atmos AI**:

| Provider         | Best For                                                    | Data Residency           |
|------------------|-------------------------------------------------------------|--------------------------|
| **Anthropic**    | Best overall quality (Claude models direct API)             | Anthropic servers        |
| **OpenAI**       | GPT models for organizations standardized on OpenAI         | OpenAI servers           |
| **Google Gemini**| Large context windows, cost-effective                       | Google Cloud             |
| **Azure OpenAI** | Enterprise Azure customers with existing deployments        | Your Azure tenant        |
| **AWS Bedrock**  | **Enterprise/compliance** — data stays in your AWS account  | **Your AWS account**     |
| **Ollama**       | Air-gapped/offline environments, no external API calls      | **Your infrastructure**  |
| **Grok (xAI)**   | Alternative provider                                        | xAI servers              |

### Provider Selection Guide

- **Enterprise customers** requiring data residency and compliance (SOC 2, HIPAA, PCI DSS, FedRAMP)
  should use **AWS Bedrock** — all data stays within your AWS account with IAM-based access control
  and CloudTrail audit logging.
- **Air-gapped environments** should use **Ollama** for fully local inference with no external API calls.
- **General use** can use any provider — the AI analysis quality depends on the model, not the provider.
  Anthropic Claude models (via direct API or Bedrock) are recommended for best security analysis quality.

### Multi-Provider Configuration Example

```yaml
# atmos.yaml
ai:
  enabled: true
  default_provider: "anthropic"  # or "bedrock" for enterprise
  providers:
    anthropic:
      model: "claude-sonnet-4-6"
      api_key: !env "ANTHROPIC_API_KEY"
      max_tokens: 8192
    bedrock:
      model: "anthropic.claude-sonnet-4-6-20250514-v1:0"
      base_url: "us-east-1"
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

The `--no-ai` flag skips AI analysis entirely (zero AI cost) and shows raw findings with
tag-based component mapping only. This is useful for CI/CD pipelines that only need structured
finding data.

### AWS Bedrock Setup (Enterprise)

AWS Bedrock is the recommended AI provider for enterprise security workloads. It runs within your
AWS account, so data never leaves your cloud environment — a key requirement for security-sensitive
workloads.

#### Option A: Manual Setup (AWS Console)

1. **Navigate to Amazon Bedrock** in the AWS Console
2. **Request model access:**

- Go to **Model access** in the left sidebar
- Click **Manage model access**
- Select **Anthropic → Claude 4 Sonnet** (or Claude 4 Opus for complex analysis)
- Click **Request model access** and wait for approval (usually instant for Anthropic models)

3. **Verify access:**

- Go to **Playgrounds → Chat** in the Bedrock console
- Select the Claude model and send a test message

4. **Note the region** — Bedrock model availability varies by region. Use `us-east-1` or `us-west-2`
   for the broadest model selection

#### Option B: Atmos/Terraform Setup

Create a Bedrock component that enables model access and sets up the required IAM permissions:

```hcl
# components/terraform/bedrock/main.tf

# Enable Bedrock model access
# Note: Model access requests must be done via console or AWS CLI as there is
# no Terraform resource for model access approval. This component manages the
# IAM permissions and monitoring.

# IAM role for Bedrock invocation
resource "aws_iam_role" "bedrock_user" {
  name = "${var.namespace}-bedrock-user"

  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Action = "sts:AssumeRole"
        Effect = "Allow"
        Principal = {
          AWS = var.trusted_role_arns
        }
      }
    ]
  })

  tags = var.default_tags
}

resource "aws_iam_role_policy" "bedrock_invoke" {
  name = "bedrock-invoke"
  role = aws_iam_role.bedrock_user.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Effect = "Allow"
        Action = [
          "bedrock:InvokeModel",
          "bedrock:InvokeModelWithResponseStream"
        ]
        Resource = [
          "arn:aws:bedrock:${var.region}::foundation-model/anthropic.*"
        ]
      }
    ]
  })
}

# CloudWatch logging for Bedrock invocations (audit trail)
resource "aws_cloudwatch_log_group" "bedrock" {
  name = "/aws/bedrock/${var.namespace}"
  retention_in_days = var.log_retention_days

  tags = var.default_tags
}
```

```yaml
# stacks/catalog/bedrock.yaml
components:
  terraform:
    bedrock:
      metadata:
        component: bedrock
      vars:
        region: "us-east-1"
        trusted_role_arns:
          - "arn:aws:iam::123456789012:role/SecurityAudit"
        log_retention_days: 90
```

Deploy with:

```shell
atmos terraform apply bedrock -s prod-us-east-1
```

#### Atmos AI Bedrock Provider Configuration

```yaml
# atmos.yaml
ai:
  enabled: true
  default_provider: "bedrock"
  providers:
    bedrock:
      model: "anthropic.claude-sonnet-4-6-20250514-v1:0"
      base_url: "us-east-1"   # AWS region for Bedrock
      max_tokens: 8192         # Higher limit for detailed analysis
      cache:
        enabled: true
        cache_system_prompt: true
```

Bedrock uses AWS credentials from Atmos Auth — no API key is needed. The `base_url` field specifies
the AWS region where Bedrock is available.

#### Bedrock Pricing

Bedrock uses **on-demand, pay-per-token** pricing with no upfront commitments. There is no charge for
Bedrock itself — you only pay for model invocations. Pricing varies by model and region.

**Claude Models on Bedrock (On-Demand, US East / US West):**

| Model             | Input (per 1M tokens) | Output (per 1M tokens) | Context Window | Best For                            |
|-------------------|-----------------------|------------------------|----------------|-------------------------------------|
| Claude Opus 4.6   | $15.00                | $75.00                 | 200K tokens    | Deep analysis of complex findings   |
| Claude Sonnet 4.6 | $3.00                 | $15.00                 | 200K tokens    | **Recommended** — best cost/quality |
| Claude Haiku 4.5  | $0.80                 | $4.00                  | 200K tokens    | High-volume finding triage          |
| Claude Sonnet 4.5 | $3.00                 | $15.00                 | 200K tokens    | Alternative to Sonnet 4.6           |
| Claude Opus 4.5   | $15.00                | $75.00                 | 200K tokens    | Alternative to Opus 4.6             |

> Prices are approximate and may vary by region. See [AWS Bedrock Pricing](https://aws.amazon.com/bedrock/pricing/)
> for current rates.

**Prompt Caching Discounts:**

Bedrock supports prompt caching for Claude models, which significantly reduces costs for repeated
analysis patterns (e.g., the system prompt and component source code stay cached across findings):

| Operation   | Price vs On-Demand              |
|-------------|---------------------------------|
| Cache Write | 1.25× input price               |
| Cache Read  | 0.10× input price (90% savings) |

**Cost Estimation for Security Analysis:**

A typical security analysis run processes ~50 findings. Each finding analysis involves:

- System prompt + security context: ~2,000 tokens (cached after first call)
- Finding details + component source: ~3,000–5,000 tokens input
- AI analysis + remediation: ~1,000–2,000 tokens output

| Scenario                      | Model      | Estimated Cost |
|-------------------------------|------------|----------------|
| 50 findings, single stack     | Sonnet 4.6 | ~$0.50–$1.50   |
| 50 findings, single stack     | Haiku 4.5  | ~$0.15–$0.40   |
| 200 findings, all stacks      | Sonnet 4.6 | ~$2.00–$6.00   |
| 200 findings, all stacks      | Opus 4.6   | ~$10.00–$30.00 |
| Raw findings only (`--no-ai`) | N/A        | $0.00          |

The `--no-ai` flag skips AI analysis entirely (zero AI cost) and shows raw findings with tag-based
component mapping only.

**Other Pricing Tiers:**

| Tier             | Description                                              | Discount   |
|------------------|----------------------------------------------------------|------------|
| **On-Demand**    | Pay per token, no commitment. Default for Atmos.         | Baseline   |
| **Batch**        | Async processing, up to 24h turnaround.                  | ~50% off   |
| **Reserved**     | 1-month or 6-month commitment for guaranteed throughput. | Up to 50%  |
| **Cross-Region** | Route requests globally for higher availability.         | Same price |

For most Atmos security analysis workloads, on-demand pricing with prompt caching is the most
cost-effective option. Reserved throughput is only needed for organizations running continuous
compliance scans at high frequency.

#### Bedrock Regional Availability

Claude models on Bedrock are available in multiple regions. For security workloads, deploy Bedrock
in the same region as your Security Hub aggregation region to minimize latency:

**Primary regions (broadest model selection):**

- `us-east-1` (N. Virginia)
- `us-west-2` (Oregon)

**Additional regions with Claude support:**

- `eu-west-1` (Ireland), `eu-central-1` (Frankfurt), `eu-west-3` (Paris)
- `ap-northeast-1` (Tokyo), `ap-southeast-1` (Singapore), `ap-southeast-2` (Sydney)
- `ca-central-1` (Canada)

**Cross-Region Inference:** Bedrock supports global cross-region inference, which routes requests
across regions for higher throughput and built-in resilience. This is useful for organizations
with multi-region Security Hub deployments.

**GovCloud:** Claude Sonnet 4.5+ is available in AWS GovCloud (US-West) via Reserved Tier for
FedRAMP and government compliance workloads.

> For the most current regional availability, see
> [Bedrock Model Support by Region](https://docs.aws.amazon.com/bedrock/latest/userguide/models-regions.html).

#### Why Bedrock for Enterprise Security Workloads

| Requirement           | Bedrock Advantage                                               |
|-----------------------|-----------------------------------------------------------------|
| **Data residency**    | Data never leaves your AWS account or chosen region             |
| **Compliance**        | Bedrock is SOC 2, HIPAA, PCI DSS, FedRAMP compliant             |
| **No API keys**       | Uses IAM roles via Atmos Auth — no secrets to manage            |
| **Audit trail**       | All invocations logged in CloudTrail + optional CloudWatch Logs |
| **Network isolation** | VPC endpoints available — no internet access required           |
| **Access control**    | IAM policies restrict who can invoke models and which models    |
| **Cost visibility**   | Standard AWS billing — no separate AI vendor invoice            |

---

## CLI Commands

### `atmos ai security`

Primary command for security analysis. Fetches findings, maps to components, and generates reports.

```shell
# Analyze findings for a specific stack
atmos ai security --stack prod-us-east-1

# Analyze findings for a specific component in a stack
atmos ai security --stack prod-us-east-1 --component vpc

# Filter by severity
atmos ai security --stack prod-us-east-1 --severity critical,high

# Filter by finding source
atmos ai security --stack prod-us-east-1 --source security-hub

# Filter by compliance framework
atmos ai security --stack prod-us-east-1 --framework cis-aws,pci-dss

# Analyze all stacks
atmos ai security

# Output as JSON (for piping to dashboards, ticketing systems, etc.)
atmos ai security --stack prod-us-east-1 --format json

# Output as YAML
atmos ai security --stack prod-us-east-1 --format yaml

# Output as CSV (for spreadsheets, compliance reporting)
atmos ai security --stack prod-us-east-1 --format csv

# Pipe JSON to jq for filtering
atmos ai security --stack prod-us-east-1 --format json | jq '.findings[] | select(.severity == "CRITICAL")'

# Feed into Slack notification
atmos ai security --stack prod-us-east-1 --format json | notify-slack --channel security-alerts

# Generate CSV for compliance audit trail
atmos ai security --format csv > findings-$(date +%Y-%m-%d).csv
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
| `--no-ai`        | bool   | `false`          | Skip AI analysis, show raw findings only                                   |
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

### `atmos ai compliance`

Generates compliance posture reports against specific frameworks.

```shell
# CIS AWS Foundations Benchmark report
atmos ai compliance --framework cis-aws --stack prod-us-east-1

# PCI DSS compliance status
atmos ai compliance --framework pci-dss

# All frameworks
atmos ai compliance --stack prod-us-east-1
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

    # AWS Bedrock (recommended for enterprise/compliance)
    bedrock:
      model: "anthropic.claude-sonnet-4-6-20250514-v1:0"
      base_url: "us-east-1"
      max_tokens: 8192

    # Other providers (configure as needed)
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

    # Maximum findings per analysis run (controls AI costs)
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

### Phase 1: Foundation

1. **Schema additions** — Add `ai.security` section to `pkg/schema/ai.go`
2. **AWS security client** — Create `pkg/ai/security/` package with clients for Security Hub,
   Config, Inspector, GuardDuty
3. **Tag-based mapping** — Implement resource tag lookup and component resolution
4. **CLI command scaffold** — Register `atmos ai security` and `atmos ai compliance` commands
   using the command registry pattern

### Phase 2: Core Analysis

5. **Finding fetcher** — Implement finding retrieval from each AWS security service
6. **Component mapper** — Implement the full mapping algorithm (tags → heuristic → AI fallback)
7. **Report generator** — Implement Markdown report rendering using `pkg/ui/`
8. **AI provider integration** — Build the prompt templates for finding analysis (works with all Atmos AI providers)

### Phase 3: AI Tools

9. **`atmos_list_findings` tool** — Register in AI tool system
10. **`atmos_describe_finding` tool** — Register in AI tool system
11. **`atmos_analyze_finding` tool** — Register in AI tool system
12. **`atmos_compliance_report` tool** — Register in AI tool system

### Phase 4: Polish

13. **JSON/YAML output** — Structured output for CI/CD integration
14. **Caching** — Cache findings and mappings to reduce API calls
15. **Documentation** — CLI command docs, configuration docs, guide
16. **Tests** — Unit tests with mocked AWS responses, integration tests

---

## Security Considerations

- **Data residency options** — When using **AWS Bedrock**, data stays in your AWS account with no
  external API calls. When using other providers (Anthropic, OpenAI, etc.), finding data is sent to
  the provider's API for analysis. Choose the provider that matches your security requirements.
- **Atmos Auth credentials** — All AWS access uses Atmos Auth; no hardcoded keys
- **Read-only by default** — The security commands only read findings and source code; they never
  modify infrastructure. Users must manually apply remediation steps
- **AI cost control** — The `max_findings` setting limits how many findings are sent to AI for
  analysis, controlling costs across all providers
- **Audit trail** — When using Bedrock, all invocations are logged via CloudTrail + optional
  CloudWatch. For other providers, consult the provider's audit logging capabilities.
- **`--no-ai` flag** — Skips AI analysis entirely for environments where sending data to any AI
  provider is not permitted. Shows raw findings with tag-based component mapping only.

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
