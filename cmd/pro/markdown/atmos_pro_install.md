Install Atmos Pro workflows and configuration into your project.

This command scaffolds the following files:

**GitHub Actions Workflows** (in `.github/workflows/`):
- `atmos-pro-terraform-plan.yaml` - Runs terraform plan on pull requests
- `atmos-pro-terraform-apply.yaml` - Applies changes on merge
- `atmos-pro-terraform-drift-detection.yaml` - Scheduled drift detection
- `atmos-pro-terraform-drift-remediation.yaml` - Drift remediation via issue labels

**Auth Profile** (in `profiles/github/`):
- `atmos.yaml` - GitHub Actions OIDC authentication configuration

**Stack Configuration** (in `stacks/`):
- `mixins/atmos-pro.yaml` - Atmos Pro workflow trigger settings
- `deploy/_defaults.yaml` - Updated with Atmos Pro import and settings
