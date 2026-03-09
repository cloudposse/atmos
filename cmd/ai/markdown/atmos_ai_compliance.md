Generate compliance posture reports against specific frameworks.

Retrieves compliance status from AWS Security Hub enabled standards, maps failing controls to
Atmos components, and generates reports with remediation guidance.

## Examples

```shell
# CIS AWS Foundations Benchmark report
atmos ai compliance --framework cis-aws --stack prod-us-east-1

# PCI DSS compliance status
atmos ai compliance --framework pci-dss

# All frameworks for a stack
atmos ai compliance --stack prod-us-east-1

# Output as JSON
atmos ai compliance --framework cis-aws --format json
```
