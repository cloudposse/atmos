---
name: atmos-aws-compliance
description: "AWS compliance commands in Atmos: atmos aws compliance report, Security Hub standards, CIS AWS, PCI DSS, SOC2, HIPAA, NIST, report formats, AI summaries"
metadata:
  copyright: Copyright Cloud Posse, LLC 2026
  version: "1.0.0"
---

# Atmos AWS Compliance

Use this skill for compliance posture reporting through AWS Security Hub. It owns
`atmos aws compliance report`.

## Command Model

`atmos aws compliance report` retrieves enabled Security Hub standard controls, maps failing
controls to Atmos stacks/components where possible, and writes reports for humans or automation.

```shell
atmos aws compliance report --framework cis-aws --stack prod-us-east-1
atmos aws compliance report --framework pci-dss --format json --file compliance.json
atmos aws compliance report --controls CIS.1.1,CIS.1.2 --format markdown
atmos aws compliance report --ai
```

Supported report formats are `markdown`, `json`, `yaml`, and `csv`.

## Configuration

Configure defaults in `atmos.yaml` under `aws.security`. Route identity setup to `atmos-auth`.

```yaml
aws:
  security:
    enabled: true
    identity: security-readonly
    region: us-east-2
    frameworks:
      - cis-aws
      - pci-dss
```

Use `--identity` to override the configured identity for a run.

## Frameworks

| Framework | Use |
|-----------|-----|
| `cis-aws` | CIS AWS Foundations Benchmark |
| `pci-dss` | Payment Card Industry Data Security Standard |
| `soc2` | SOC 2 trust service criteria |
| `hipaa` | HIPAA controls for protected health information |
| `nist` | NIST 800-53 controls |

## Agent Guidance

- Prefer `--framework` for targeted checks. Omit it only when the user explicitly wants all enabled
  frameworks.
- Use `--stack` when the report should map compliance status to a specific Atmos stack.
- Use `--format json` or `--format yaml` for automation and CI gates; use `markdown` for human
  reports.
- Use `--file` for durable artifacts. Parent directories are created by the command.
- Use `--ai` only when the user asks for AI-generated summary or remediation guidance.
- For detailed per-finding remediation output, route to `atmos-aws-security`.
- Do not invent compliance mappings. If component mapping is missing or low-confidence, say so and
  use Atmos introspection before proposing component changes.

## Routing

| Need | Skill |
|------|-------|
| Detailed security finding analysis and remediation format | `atmos-aws-security` |
| AWS identity/provider setup, SSO, SAML, OIDC, assume role/root | `atmos-auth` |
| AI provider setup for `--ai` summaries | `atmos-ai` |
| Stack/component lookup before remediation | `atmos-introspection`, `atmos-components` |
