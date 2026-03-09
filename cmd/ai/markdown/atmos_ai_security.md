Analyze AWS security findings and map them to Atmos components and stacks.

Connects to AWS Security Hub, Config, Inspector, GuardDuty, and other security services via Atmos Auth,
maps findings to the Terraform/Atmos components that manage the affected resources, and generates
remediation reports with concrete code changes.

## Examples

```shell
# Analyze findings for a specific stack
atmos ai security --stack prod-us-east-1

# Filter by severity
atmos ai security --stack prod-us-east-1 --severity critical,high

# Filter by source service
atmos ai security --stack prod-us-east-1 --source security-hub

# Output as JSON for CI/CD integration
atmos ai security --stack prod-us-east-1 --format json

# Raw findings without AI analysis
atmos ai security --stack prod-us-east-1 --no-ai

# Output as CSV for compliance reporting
atmos ai security --format csv > findings.csv
```
