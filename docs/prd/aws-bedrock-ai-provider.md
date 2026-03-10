# AWS Bedrock AI Provider - Setup & Reference

**Status:** Reference
**Version:** 0.1
**Last Updated:** 2026-03-10
**Parent PRD:** [atmos-aws-security-compliance.md](atmos-aws-security-compliance.md)

---

## Overview

AWS Bedrock is the recommended AI provider for enterprise security workloads. It runs within your
AWS account, so data never leaves your cloud environment — a key requirement for security-sensitive
workloads. This document covers Bedrock-specific setup, configuration, pricing, and regional
availability for use with Atmos AI (including `atmos aws security analyze --ai`).

For general AI provider configuration, see the [Atmos AWS Security & Compliance PRD](atmos-aws-security-compliance.md).

---

## Why Bedrock for Enterprise Security Workloads

| Requirement           | Bedrock Advantage                                               |
|-----------------------|-----------------------------------------------------------------|
| **Data residency**    | Data never leaves your AWS account or chosen region             |
| **Compliance**        | Bedrock is SOC 2, HIPAA, PCI DSS, FedRAMP compliant            |
| **No API keys**       | Uses IAM roles via Atmos Auth — no secrets to manage            |
| **Audit trail**       | All invocations logged in CloudTrail + optional CloudWatch Logs |
| **Network isolation** | VPC endpoints available — no internet access required           |
| **Access control**    | IAM policies restrict who can invoke models and which models    |
| **Cost visibility**   | Standard AWS billing — no separate AI vendor invoice            |

---

## Required AWS Permissions

When using Bedrock as the AI provider, add these permissions to the IAM role/user:

```json
{
  "Sid": "BedrockInvoke",
  "Effect": "Allow",
  "Action": [
    "bedrock:InvokeModel",
    "bedrock:InvokeModelWithResponseStream"
  ],
  "Resource": "arn:aws:bedrock:*::foundation-model/*"
}
```

---

## Atmos Auth Identity for Bedrock

```yaml
# atmos.yaml
auth:
  identities:
    bedrock-user:
      provider: aws-security
      type: permission-set
      config:
        permission_set: "BedrockUser"
        account_id: "123456789012"
```

---

## Atmos AI Provider Configuration

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

---

## Setup

### Option A: Manual Setup (AWS Console)

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

### Option B: Atmos/Terraform Setup

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

---

## Pricing

Bedrock uses **on-demand, pay-per-token** pricing with no upfront commitments. There is no charge for
Bedrock itself — you only pay for model invocations. Pricing varies by model and region.

### Claude Models on Bedrock (On-Demand, US East / US West)

| Model             | Input (per 1M tokens) | Output (per 1M tokens) | Context Window | Best For                            |
|-------------------|-----------------------|------------------------|----------------|-------------------------------------|
| Claude Opus 4.6   | $15.00                | $75.00                 | 200K tokens    | Deep analysis of complex findings   |
| Claude Sonnet 4.6 | $3.00                 | $15.00                 | 200K tokens    | **Recommended** — best cost/quality |
| Claude Haiku 4.5  | $0.80                 | $4.00                  | 200K tokens    | High-volume finding triage          |
| Claude Sonnet 4.5 | $3.00                 | $15.00                 | 200K tokens    | Alternative to Sonnet 4.6           |
| Claude Opus 4.5   | $15.00                | $75.00                 | 200K tokens    | Alternative to Opus 4.6             |

> Prices are approximate and may vary by region. See [AWS Bedrock Pricing](https://aws.amazon.com/bedrock/pricing/)
> for current rates.

### Prompt Caching Discounts

Bedrock supports prompt caching for Claude models, which significantly reduces costs for repeated
analysis patterns (e.g., the system prompt and component source code stay cached across findings):

| Operation   | Price vs On-Demand              |
|-------------|---------------------------------|
| Cache Write | 1.25x input price               |
| Cache Read  | 0.10x input price (90% savings) |

### Cost Estimation for Security Analysis

A typical security analysis run processes ~50 findings. Each finding analysis involves:

- System prompt + security context: ~2,000 tokens (cached after first call)
- Finding details + component source: ~3,000-5,000 tokens input
- AI analysis + remediation: ~1,000-2,000 tokens output

| Scenario                      | Model      | Estimated Cost |
|-------------------------------|------------|----------------|
| 50 findings, single stack     | Sonnet 4.6 | ~$0.50-$1.50   |
| 50 findings, single stack     | Haiku 4.5  | ~$0.15-$0.40   |
| 200 findings, all stacks      | Sonnet 4.6 | ~$2.00-$6.00   |
| 200 findings, all stacks      | Opus 4.6   | ~$10.00-$30.00 |
| Default (no `--ai` flag)      | N/A        | $0.00          |

By default, commands run without AI (zero cost). Use `--ai` to enable AI analysis.

### Other Pricing Tiers

| Tier             | Description                                              | Discount   |
|------------------|----------------------------------------------------------|------------|
| **On-Demand**    | Pay per token, no commitment. Default for Atmos.         | Baseline   |
| **Batch**        | Async processing, up to 24h turnaround.                  | ~50% off   |
| **Reserved**     | 1-month or 6-month commitment for guaranteed throughput. | Up to 50%  |
| **Cross-Region** | Route requests globally for higher availability.         | Same price |

For most Atmos security analysis workloads, on-demand pricing with prompt caching is the most
cost-effective option. Reserved throughput is only needed for organizations running continuous
compliance scans at high frequency.

---

## Regional Availability

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
