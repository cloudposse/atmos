Analyze AWS security findings and map them to Atmos components and stacks.

Connects to AWS Security Hub, Config, Inspector, GuardDuty, and other security services via Atmos Auth,
maps findings to the Terraform/Atmos components that manage the affected resources, and generates
remediation reports with concrete code changes.

## Examples

```shell
# Analyze findings for a specific stack
atmos aws security analyze --stack prod-us-east-1

# Filter by severity
atmos aws security analyze --stack prod-us-east-1 --severity critical,high

# Filter by source service
atmos aws security analyze --stack prod-us-east-1 --source security-hub

# Output as JSON for CI/CD integration
atmos aws security analyze --stack prod-us-east-1 --format json

# Enable AI-powered analysis
atmos aws security analyze --stack prod-us-east-1 --ai

# Output as CSV for compliance reporting
atmos aws security analyze --format csv > findings.csv

# Save report to a file
atmos aws security analyze --stack prod-us-east-1 --file security-report.md

# Save JSON report to a file
atmos aws security analyze --stack prod-us-east-1 --format json --file findings.json
```
