# Atmos AWS Security & Compliance — Product Requirements Document

**Status:** Shipped (experimental)
**Version:** 1.0
**Last Updated:** 2026-04-03

---

## Problem

Reviewing AWS security findings requires navigating multiple AWS console pages, cross-referencing
resources with Terraform code, and manually figuring out which configuration caused the issue. This is
slow, error-prone, and requires deep AWS + Terraform expertise.

## Solution

Two CLI commands that fetch findings from AWS Security Hub, map them to Atmos components and stacks,
and generate structured remediation reports — all from a single command.

```shell
# Security findings mapped to components
atmos aws security analyze --stack prod-us-east-1

# AI-powered remediation (reads source code, generates specific fixes)
atmos aws security analyze --stack prod-us-east-1 --ai

# Compliance posture scoring
atmos aws compliance report --framework cis-aws
```

**Key differentiator:** Atmos owns the component-to-stack relationship, so it traces a finding on an
AWS resource back to the exact Terraform code and stack configuration that created it.

**AI is optional.** Commands work purely with AWS APIs. The `--ai` flag adds root cause analysis,
remediation guidance, and deploy commands using any Atmos AI provider (Anthropic, OpenAI, Gemini,
Azure OpenAI, Bedrock, Ollama, Grok).

**Cloud-specific namespace.** Commands live under `atmos aws` to enable future `atmos azure security`
and `atmos gcp security`.

---

## Architecture

```text
┌─────────────────────────────────────────────────────────────────────┐
│                 atmos aws security analyze <stack>                  │
├─────────────────────────────────────────────────────────────────────┤
│  Atmos Auth → AWS Security Hub → Component Mapper → AI → Report   │
│                                                                     │
│  Finding → Resource ARN → Resource Tags → Atmos Stack →            │
│  Atmos Component → Terraform Source → Root Cause → Remediation     │
└─────────────────────────────────────────────────────────────────────┘
```

### Data Flow

1. **Authenticate** — Atmos Auth obtains AWS credentials (SSO, assume-role) via `identity` config
2. **Fetch Findings** — Query Security Hub for active findings (severity, source, framework filters)
3. **Map to Components** — Trace each finding's resource back to an Atmos component/stack via tags or heuristics
4. **AI Analysis** (opt-in) — Send finding + component source + stack config to AI for root cause analysis
5. **Generate Report** — Render as Markdown (terminal), JSON (CI/CD), YAML, or CSV

### Data Schema

```text
Finding → ComponentMapping → Remediation (AI-populated) → Report → ReportRenderer
```

- **Finding:** ID, title, severity, source, resource ARN, resource type, tags, account, region
- **ComponentMapping:** stack, component, component_path, confidence (exact/high/low/none), method
- **Remediation:** root_cause, steps[], code_changes[], stack_changes, deploy_command, risk_level, references[]
- **Report:** generated_at, total_findings, severity_counts, mapped/unmapped counts, findings[]

Without `--ai`, Remediation is nil. With `--ai`, the embedded skill prompt ensures all providers
fill the same fields in the same format.

---

## Finding-to-Code Mapping

The system uses 5 mapping strategies in priority order, stopping at the first confident match:

| Priority | Method | Confidence | How It Works |
|----------|--------|------------|--------------|
| 1 | `finding-tag` | exact | `atmos_stack` + `atmos_component` tags embedded in the Security Hub finding |
| 2 | `tag-api` | exact | Same tags from the Resource Groups Tagging API (same-account only) |
| 3 | `context-tags` | high | Cloud Posse context tags (`Namespace`, `Tenant`, `Environment`, `Stage`, `Name`) reconstruct naming prefix → extract component name |
| 4 | `account-map` | low | Account-level findings → account name from `aws.security.account_map` config |
| 5 | `ecr-repo` | low | ECR findings → component from repository name, stack from account map |
| 6 | `naming-convention` | low | Last hyphen segment of resource name (unreliable for multi-word components) |
| 7 | `resource-type` | low | AWS resource type → component name heuristic |

**Tag configuration** — the tag keys are configurable in `atmos.yaml`:

```yaml
aws:
  security:
    tag_mapping:
      stack_tag: "atmos:stack"
      component_tag: "atmos:component"
```

**Post-mapping filtering** — `--stack` and `--component` filter AFTER mapping (Security Hub has
no concept of Atmos stacks). Stack matching supports prefix (`plat-use2-prod` matches
`plat-use2-prod-vpc`). Unmapped findings are excluded when filters are active.

---

## AI Analysis

When `--ai` is passed, each mapped finding is sent to the configured AI provider for analysis.

**API providers (multi-turn):** Uses `SendMessageWithSystemPromptAndTools` with the Atmos tool
registry. The AI can call `atmos_describe_component`, `read_component_file`, `read_stack_file`
to gather context before generating remediation. Up to 10 tool iterations.

**CLI providers (single-prompt):** Falls back to enriched single-prompt mode with pre-fetched
component source (`main.tf`) and stack config.

**Deduplication:** Findings with the same title + component + stack are analyzed once; remediation
is shared across duplicates.

**Retry:** Transient errors (529/429/500/502/503) retry with exponential backoff (3 attempts,
2s initial delay, 15s max, 30% jitter) via `pkg/retry`.

**Timeout:** 300s default when `--ai` is used (configurable via `ai.timeout_seconds`).

**Skill prompt:** Embedded `go:embed skill_prompt.md` instructs AI to return structured output
matching the `Remediation` schema fields (root cause, steps, code changes, stack changes,
deploy command, risk level, references).

---

## CLI Commands

### `atmos aws security analyze`

```shell
atmos aws security analyze                                          # All findings
atmos aws security analyze --stack prod-us-east-1                   # Filter by stack
atmos aws security analyze --stack prod-us-east-1 --component vpc   # Filter by component
atmos aws security analyze --severity critical,high                 # Filter by severity
atmos aws security analyze --source guardduty                       # Filter by source
atmos aws security analyze --ai                                     # AI-powered remediation
atmos aws security analyze --format json --file findings.json       # Save as JSON
atmos aws security analyze --identity security-admin --region us-west-2  # Override auth
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--stack` | string | (all) | Target stack |
| `--component` | string | (all) | Target component |
| `--severity` | string | `critical,high` | Severity filter |
| `--source` | string | `all` | Source: security-hub, config, inspector, guardduty, all |
| `--format` | string | `markdown` | Output: markdown, json, yaml, csv |
| `--file` | string | (stdout) | Write to file |
| `--max-findings` | int | `500` | Maximum findings |
| `--ai` | bool | `false` | Enable AI analysis |
| `--no-group` | bool | `false` | Disable duplicate grouping |
| `--region` | string | (config) | AWS region override |
| `--identity` | string | (config) | Atmos Auth identity override |

### `atmos aws compliance report`

```shell
atmos aws compliance report                                         # Default framework (cis-aws)
atmos aws compliance report --framework cis-aws                     # CIS benchmark
atmos aws compliance report --framework pci-dss --format json       # PCI DSS as JSON
atmos aws compliance report --controls CIS.1.14,CIS.2.1             # Specific controls
```

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--stack` | string | (all) | Target stack |
| `--framework` | string | (all) | Framework: cis-aws, pci-dss, soc2, hipaa, nist |
| `--format` | string | `markdown` | Output: markdown, json, yaml, csv |
| `--file` | string | (stdout) | Write to file |
| `--controls` | string | (all) | Specific control IDs to check |
| `--identity` | string | (config) | Atmos Auth identity override |

---

## Configuration

```yaml
# atmos.yaml
aws:
  security:
    enabled: true
    identity: "security-readonly"    # Atmos Auth identity → Security Hub account
    region: "us-east-2"              # Security Hub aggregation region
    default_severity: [CRITICAL, HIGH]
    max_findings: 500
    tag_mapping:
      stack_tag: "atmos_stack"
      component_tag: "atmos_component"
    account_map:                     # Account ID → name for account-level findings
      "123456789012": "core-security"
      "234567890123": "plat-use2-dev"
    frameworks: [cis-aws, pci-dss]

# AI (optional, for --ai flag)
ai:
  enabled: true
  default_provider: "anthropic"
  providers:
    anthropic:
      model: "claude-sonnet-4-6"
      api_key: !env "ANTHROPIC_API_KEY"
  tools:
    enabled: true
```

### Authentication

All AWS API calls use Atmos Auth. The `identity` field targets the delegated admin account
where Security Hub aggregates findings from all member accounts. The `region` field targets
the Security Hub aggregation region.

### Required AWS Permissions

```json
{
  "Statement": [
    {
      "Sid": "SecurityFindings",
      "Effect": "Allow",
      "Action": [
        "securityhub:GetFindings",
        "securityhub:GetEnabledStandards",
        "securityhub:ListSecurityControlDefinitions",
        "sts:GetCallerIdentity"
      ],
      "Resource": "*"
    },
    {
      "Sid": "ResourceTagLookup",
      "Effect": "Allow",
      "Action": ["tag:GetResources"],
      "Resource": "*"
    }
  ]
}
```

---

## Implementation

### File Structure

| File | Purpose |
|------|---------|
| `cmd/aws/security.go` | Security analyze command, flags, filtering, AI integration |
| `cmd/aws/compliance.go` | Compliance report command, control filtering |
| `cmd/aws/credentials.go` | Shared AWS credential validation, Atmos Auth resolution |
| `pkg/aws/security/types.go` | Finding, ComponentMapping, Remediation, Report structs |
| `pkg/aws/security/finding_fetcher.go` | Security Hub API queries, pagination, compliance scoring |
| `pkg/aws/security/component_mapper.go` | 7-strategy mapping pipeline (tag → context → heuristic) |
| `pkg/aws/security/analyzer.go` | AI analysis: dedup, retry, multi-turn tools, skill prompt |
| `pkg/aws/security/report_renderer.go` | Markdown, JSON, YAML, CSV rendering |
| `pkg/aws/security/aws_clients.go` | AWS SDK interfaces (SecurityHubAPI, TaggingAPI) |
| `pkg/aws/security/cache.go` | Findings and compliance cache |
| `pkg/aws/security/skill_prompt.md` | Embedded AI system prompt for structured remediation |
| `pkg/schema/aws_security.go` | Schema: AWSSecuritySettings, TagMapping, AccountMap |
| `agent-skills/skills/atmos-aws-security/SKILL.md` | Agent skill for MCP/AI tools |

### AI Tools

Registered in `pkg/ai/tools/atmos/` for both CLI commands and MCP clients:

| Tool | Purpose |
|------|---------|
| `atmos_list_findings` | List security findings with filters |
| `atmos_describe_finding` | Full finding details with mapping |
| `atmos_analyze_finding` | AI analysis for a specific finding |
| `atmos_compliance_report` | Compliance posture report |

### Error Handling

Static sentinel errors in `errors/errors.go`:

| Error | When |
|-------|------|
| `ErrAISecurityNotEnabled` | `aws.security.enabled` is false |
| `ErrAISecurityFetchFailed` | AWS API errors |
| `ErrAISecurityMappingFailed` | Component mapping fails |
| `ErrAISecurityAnalysisFailed` | AI provider errors |
| `ErrAWSCredentialsNotValid` | STS GetCallerIdentity fails |
| `ErrAISecurityInvalidSeverity` | Unknown severity value |
| `ErrAISecurityInvalidSource` | Unknown source value |
| `ErrAISecurityInvalidFramework` | Unknown framework value |
| `ErrAISecurityInvalidFormat` | Unknown output format |

---

## Testing

### Coverage

| Test File | Tests | Coverage |
|-----------|-------|----------|
| `pkg/aws/security/finding_fetcher_test.go` | 17+ | ~92% |
| `pkg/aws/security/component_mapper_test.go` | 11 | ~90% |
| `pkg/aws/security/report_renderer_test.go` | 18 | ~95% |
| `pkg/aws/security/analyzer_test.go` | 25+ | ~90% |
| `pkg/aws/security/cache_test.go` | 9 | ~90% |
| `cmd/aws/security_test.go` | 50+ | 100%* |
| `cmd/aws/compliance_test.go` | 25+ | 100%* |

\* All testable functions. RunE handlers and `validateAWSCredentials` require real AWS.

**Overall:** `pkg/aws/security/` at 84.5%, `cmd/aws/` at 32.5%.

### Approach

- Unit tests with mocks for all AWS API interactions (no real AWS calls in CI)
- Table-driven tests for input validation
- Manual mock implementations for AI client (`mockAIClient`, `countingMockClient`)
- Interface-driven design: `SecurityHubAPI`, `TaggingAPI`, `FindingFetcher`, `ComponentMapper`, `FindingAnalyzer`, `ReportRenderer`

---

## Production Testing Results

Tested against a multi-account AWS organization (11 accounts, Security Hub delegated admin).

### Mapping Accuracy (500 findings)

| Method | Count | Confidence |
|--------|-------|------------|
| `ecr-repo` | 395 | low |
| `context-tags` | 41 | high |
| `finding-tag` | 28 | exact |
| `account-map` | 21 | low |
| `resource-type` | 1 | low |
| **Total mapped** | **486 (97.2%)** | |
| Unmapped | 14 (2.8%) | |

### Stack/Component Filtering

- `--stack plat-use2-prod` → 13 findings (all HIGH, 100% mapped, 10 components)
- `--stack plat-use2-dev` → 17 findings (all mapped, 11 components)
- `--stack plat-use2-dev --component rds/example` → 4 findings (exact match)
- No filter → 500 findings across 18 stacks

### Compliance Report

- `atmos aws compliance report --framework cis-aws` → 40/42 controls passing (95%)
- 2 failing: AWS Config not enabled (CRITICAL), S3 public access block (MEDIUM)
- Total controls counted via `ListSecurityControlDefinitions` API

### AI Analysis (`--ai`)

- `--stack plat-use2-dev --component rds/example --ai` → 4 findings on same security group
- AI read `catalog/rds/defaults.yaml` via tools, identified `allowed_cidr_blocks` root cause
- Generated: 6 remediation steps, stack YAML changes, Terraform validation guards, deploy command
- Detected anomaly: port 22 on RDS SG flagged as likely console drift
- Risk assessment: Medium (port 5432), Low (port 22/SSH)
- Global `--ai` summary synthesized all 4 findings into prioritized action plan

---

## Known Limitations

1. **Cross-account tag lookup** — The Tagging API only works in the same account. Finding-embedded
   tags (`Resources[].Tags`) are the primary source.

2. **Naming convention is the weakest mapper** — Only used as last resort (confidence: low).

3. **AI timeout on large context** — Multi-turn tool analysis with retries can take >120s
   per finding. Default increased to 300s. Configurable via `ai.timeout_seconds`.

4. **Compliance framework filter** — Uses PREFIX matching with type prefix (`ruleset/` or
   `standards/`). Some frameworks may have variant prefixes not yet mapped.

---

## Remaining Work

- **Component name validation** — Cross-reference heuristic names against `atmos list components`
- **Terraform state search** — Scan state files for resource ARN mapping (reuse `!terraform.state`)
- **AI-assisted inference** — Send unmapped findings to AI for component inference
- **Integration tests** — End-to-end tests with real AWS API calls (test account needed)

---

## Design Decisions

### Why Direct AWS SDK (not MCP Server)

The `awslabs.well-architected-security-mcp-server` fetches the same raw findings via the same APIs.
We chose direct SDK calls because:

- Full control over filtering (severity, source, framework, max findings)
- No external dependencies (no `uvx`, no MCP subprocess)
- Finding-to-code mapping requires Atmos-internal data (stacks, components) that no MCP server has
- MCP servers complement this for ad-hoc conversational queries (`atmos ai ask "show me critical findings"`)

### Why Post-Mapping Filtering

Security Hub has no concept of Atmos stacks. Stack/component filtering happens AFTER findings are
mapped to components via tags/heuristics. This is the only reliable approach because the mapping
method (tags vs naming convention) determines which stack a finding belongs to.

---

## Security Considerations

- **AI is opt-in** — No data sent to AI providers without `--ai` flag
- **Read-only** — Commands never modify infrastructure
- **Data residency** — Choose provider matching requirements (Bedrock keeps data in-account, Ollama runs locally)
- **Atmos Auth** — All AWS access via Atmos Auth; no hardcoded keys
- **Credential validation** — Early STS GetCallerIdentity check before pipeline starts

---

## AWS Security Services Reference

Security Hub aggregates findings from: AWS Config, GuardDuty, Inspector, Macie, IAM Access Analyzer.
All follow a multi-account delegated admin pattern with the security account as admin.

| Service | Component | Finding Types |
|---------|-----------|---------------|
| Security Hub | `aws-security-hub` | Aggregated ASFF findings, compliance controls |
| AWS Config | `aws-config` | Resource compliance evaluations |
| GuardDuty | `aws-guardduty` | Threat detection (ML-based) |
| Inspector v2 | `aws-inspector2` | CVE vulnerabilities, network reachability |
| Access Analyzer | `aws-access-analyzer` | External/unused access |
| Macie | `aws-macie` | S3 sensitive data, policy findings |

### Prerequisite Components

| Component | Required |
|-----------|----------|
| `cloudtrail` + `cloudtrail-bucket` | Yes |
| `aws-config` + `aws-config-bucket` | Yes |
| `aws-security-hub` | Yes |
| `aws-guardduty` | Yes |
| `aws-inspector2` | Recommended |
| `aws-access-analyzer` | Recommended |
| `aws-macie` | Optional |

---

## Documentation

- `website/docs/cli/commands/aws/security/analyze.mdx`
- `website/docs/cli/commands/aws/compliance/report.mdx`
- `website/docs/cli/configuration/aws/security.mdx`
- `examples/aws-security-compliance/`
- `website/blog/2026-04-03-aws-security-compliance.mdx`
