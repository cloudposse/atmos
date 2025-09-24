#!/usr/bin/env python3
import re

file_path = '/Users/erik/Dev/cloudposse/tools/atmos/.conductor/custom-planfile/website/docs/core-concepts/terraform/planfiles.mdx'

with open(file_path, 'r') as f:
    content = f.read()

# Remove CI/CD Integration section entirely
ci_cd_pattern = r'## CI/CD Integration.*?(?=## |$)'
content = re.sub(ci_cd_pattern, '', content, flags=re.DOTALL)

# Find and enhance the Alternative section about JSON/YAML planfiles
old_alternative_section = r'## Alternative: JSON/YAML Planfiles.*?This performs a semantic comparison of the plan data structures, not a naive text diff\.\n'

new_alternative_section = """## Working with Planfiles in Practice

### Binary Planfiles (Native Terraform)

When using supported backends, Atmos automatically generates binary planfiles:

```bash
# Atmos generates: dev-vpc.planfile
atmos terraform plan vpc -s dev

# Apply the generated planfile
atmos terraform apply vpc -s dev --from-plan

# Or specify a custom planfile name
atmos terraform plan vpc -s dev -out=my-custom.tfplan
atmos terraform apply vpc -s dev --planfile my-custom.tfplan
```

### JSON/YAML Planfiles with `atmos terraform generate planfile`

For enhanced security scanning, policy validation, or when binary planfiles aren't suitable, use Atmos's JSON/YAML planfile generation:

```bash
# Generate JSON planfile for security scanning
atmos terraform generate planfile vpc -s dev --file-template plan-{{.stack}}-{{.component}}.json

# Generate YAML planfile for human review
atmos terraform generate planfile vpc -s dev --file-template plan-{{.stack}}-{{.component}}.yaml
```

#### Integration with Security and Compliance Tools

JSON planfiles enable integration with security scanning and policy tools:

```bash
# Generate JSON planfile for security scanning
atmos terraform generate planfile vpc -s prod --file-template prod-vpc.json

# Scan with Wiz.io or similar tools
wiz-cli iac scan --format terraform-plan prod-vpc.json

# Check with Open Policy Agent (OPA)
opa eval -d policies/ -i prod-vpc.json "data.terraform.deny[msg]"

# Validate with Checkov
checkov --framework terraform_plan -f prod-vpc.json

# Analyze with Terrascan
terrascan scan -i tfplan -f prod-vpc.json
```

Benefits of JSON/YAML planfiles:
- **Security Scanning**: Integrate with tools like Wiz.io, Snyk, Checkov
- **Policy as Code**: Validate with OPA, Sentinel, or custom policies
- **Human Review**: YAML format is easy to read and review
- **Version Control**: Can be safely committed (no binary data)
- **Backend Independent**: Works with any backend, including those that don't support binary planfiles
- **Audit Trail**: Creates reviewable record of planned changes

### Comparing Planfiles with `atmos terraform plan-diff`

Compare any two planfiles to understand differences:

```bash
# Compare two JSON planfiles
atmos terraform plan-diff --file plan1.json --file2 plan2.json

# Compare different formats
atmos terraform plan-diff --file plan1.yaml --file2 plan2.json --format yaml

# Compare binary planfile with JSON (after converting)
atmos terraform generate planfile vpc -s dev --file-template current.json
atmos terraform plan-diff --file baseline.json --file2 current.json
```

This performs a semantic comparison of the plan data structures, not a naive text diff.
"""

content = re.sub(old_alternative_section, new_alternative_section, content, flags=re.DOTALL)

# Remove the "Use Planfiles in Production" section if it exists
production_pattern = r'### \d+\. Use Planfiles in Production.*?(?=### \d+\.|## |$)'
content = re.sub(production_pattern, '', content, flags=re.DOTALL)

# Update the Best Practices section to be more focused
old_practices = r'## Best Practices.*?(?=## |$)'
new_practices = """## Best Practices

### When to Use Planfiles

**Use planfiles when:**
- You need guaranteed consistency between plan and apply
- Multiple team members review changes before applying
- Compliance requires an audit trail of approved changes
- You want to prevent drift between planning and execution
- Running in automated pipelines where plans need approval

**Skip planfiles when:**
- Working in development environments with rapid iteration
- Using backends that don't support them (Remote, Terraform Cloud)
- Backend credentials would be embedded (HTTP backend)
- Real-time infrastructure state is critical

### Security Considerations

1. **Never commit binary planfiles to version control** - They may contain sensitive data
2. **Use environment variables for backend authentication** - Avoid `-backend-config` with sensitive values
3. **Consider JSON planfiles for sensitive environments** - Use `atmos terraform generate planfile` for reviewable, scannable output
4. **Set appropriate file permissions** - Planfiles should be readable only by authorized users
5. **Clean up old planfiles** - Remove after applying or when no longer needed

### Choosing the Right Approach

| Scenario | Recommended Approach | Command |
|----------|---------------------|---------|
| Production deployment with approval | Binary planfile | `atmos terraform plan vpc -s prod` |
| Security scanning required | JSON planfile | `atmos terraform generate planfile vpc -s prod --file-template plan.json` |
| Using HTTP backend | Skip planfiles | `atmos terraform plan vpc -s prod --skip-planfile` |
| Rapid development iteration | Skip planfiles | `atmos terraform deploy vpc -s dev` |
| Compliance audit required | JSON planfile + version control | `atmos terraform generate planfile` with Git |

"""

content = re.sub(old_practices, new_practices, content, flags=re.DOTALL)

# Remove any remaining GitLab-specific sections
content = re.sub(r'For GitLab-managed.*?```\n', '', content, flags=re.DOTALL)

# Clean up any double blank lines
content = re.sub(r'\n{3,}', '\n\n', content)

with open(file_path, 'w') as f:
    f.write(content)

print("✓ Cleaned up planfiles.mdx")
print("  - Removed unvalidated CI/CD examples")
print("  - Enhanced JSON/YAML planfile section with security tools")
print("  - Focused on why, when, and how to use planfiles")
print("  - Added practical command examples")
