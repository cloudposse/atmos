Generate compliance posture reports against specific frameworks.

Retrieves compliance status from AWS Security Hub enabled standards, maps failing controls to
Atmos components, and generates reports with remediation guidance.

## Examples

```shell
# CIS AWS Foundations Benchmark report
atmos aws compliance report --framework cis-aws --stack prod-us-east-1

# PCI DSS compliance status
atmos aws compliance report --framework pci-dss

# All frameworks for a stack
atmos aws compliance report --stack prod-us-east-1

# Output as JSON
atmos aws compliance report --framework cis-aws --format json

# Save report to a file
atmos aws compliance report --framework cis-aws --stack prod-us-east-1 --file compliance-report.md

# Save JSON report to a file
atmos aws compliance report --framework pci-dss --format json --file pci-report.json
```
