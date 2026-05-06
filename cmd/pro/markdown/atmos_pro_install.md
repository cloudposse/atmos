Install Atmos Pro workflows and configuration into your project.

If no `atmos.yaml` exists, a minimal one is created as an anchor for
the `.atmos.d/` drop-in configuration files.

This command scaffolds the following files:

**GitHub Actions Workflows** (in `.github/workflows/`):
- `atmos-pro-terraform-plan.yaml` - Runs terraform plan on pull requests
- `atmos-pro-terraform-apply.yaml` - Applies changes on merge
- `atmos-pro-affected-stacks.yaml` - Determines affected stacks on PRs
- `atmos-pro-list-instances.yaml` - Lists instances for Atmos Pro sync

**Auth Profiles** (in `profiles/`):
- `github-plan/atmos.yaml` - Read-only OIDC authentication for terraform plan
- `github-apply/atmos.yaml` - Full OIDC authentication for terraform apply

**Stack Configuration** (in your configured stacks path):
- `mixins/atmos-pro.yaml` - Atmos Pro workflow trigger settings

**Root Configuration** (if missing):
- `atmos.yaml` - Minimal Atmos configuration anchor

**Drop-in Configuration** (in `.atmos.d/`):
- `ci.yaml` - Enables native CI/CD support
- `atmos-pro.yaml` - Atmos Pro API settings
