# Config Profiles Example

This example demonstrates Atmos config profiles - a powerful feature for managing environment-specific configurations without duplicating settings across your infrastructure.

## What are Config Profiles?

Config profiles allow you to:
- Switch between different configurations based on context (development, CI/CD, production)
- Override settings without modifying base configuration
- Share common configurations across team members
- Maintain clean separation between personal and team settings

## Directory Structure

```
examples/config-profiles/
├── README.md
├── atmos.yaml                    # Base configuration (shared settings)
└── profiles/                     # Profile definitions
    ├── developer/                # Developer workstation profile
    │   ├── settings.yaml         # Terminal and UI settings
    │   └── auth.yaml            # Local AWS SSO configuration
    ├── ci/                       # CI/CD pipeline profile
    │   ├── settings.yaml         # CI-friendly settings (no color, etc.)
    │   └── auth.yaml            # GitHub OIDC authentication
    └── production/               # Production deployment profile
        ├── settings.yaml         # Production-safe settings
        └── auth.yaml            # Production AWS credentials

```

## Usage

### Single Profile

Activate a profile using the `--profile` flag:

```bash
# Use developer profile for local development
atmos terraform plan vpc -s dev --profile developer

# Use CI profile in GitHub Actions
atmos terraform apply vpc -s prod --profile ci
```

### Multiple Profiles (Layered Configuration)

You can layer multiple profiles - **rightmost wins**:

```bash
# Base settings + developer overrides
atmos terraform plan vpc -s dev --profile base --profile developer

# Shared team settings + personal overrides
atmos terraform plan vpc -s dev --profile team --profile personal
```

### Environment Variable

Set a profile globally for your session:

```bash
export ATMOS_PROFILE=developer
atmos terraform plan vpc -s dev    # Automatically uses developer profile
```

### Comma-Separated Profiles

```bash
atmos terraform plan vpc -s dev --profile base,developer,personal
```

## Profile Precedence

Profiles are discovered and loaded from multiple locations (highest to lowest precedence):

1. **Configurable** (`profiles.base_path` in atmos.yaml)
2. **Project-hidden** (`.atmos/profiles/` in project)
3. **XDG user** (`~/.config/atmos/profiles/` or `$XDG_CONFIG_HOME/atmos/profiles/`)
4. **Project** (`profiles/` in project)

Configuration merge order:
1. Base `atmos.yaml`
2. `.atmos.d/` directory configs
3. Profiles (left-to-right: `--profile base --profile developer` applies base first, then developer)
4. CLI flags and environment variables

## Example Scenarios

### Scenario 1: Developer Workstation

```bash
# Create personal developer profile in XDG location
mkdir -p ~/.config/atmos/profiles/developer

# Configure AWS SSO for local development
cat > ~/.config/atmos/profiles/developer/auth.yaml <<EOF
auth:
  providers:
    aws-sso-dev:
      kind: aws/sso
      region: us-east-2
      start_url: https://my-company.awsapps.com/start

  identities:
    dev-access:
      via:
        provider: aws-sso-dev
      principal:
        account_id: "999888777666"
        permission_set: DeveloperAccess
      default: true
EOF

# Use it
atmos terraform plan vpc -s dev --profile developer
```

### Scenario 2: CI/CD Pipeline

```yaml
# .github/workflows/deploy.yml
name: Deploy Infrastructure
on: [push]

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      id-token: write  # Required for OIDC
      contents: read
    steps:
      - uses: actions/checkout@v4

      - name: Deploy with CI profile
        env:
          ATMOS_PROFILE: ci
        run: |
          atmos terraform apply vpc -s prod --auto-approve
```

### Scenario 3: Production Deployment

```bash
# Production profile with strict settings
ATMOS_PROFILE=production atmos terraform apply --dry-run vpc -s prod
```

### Scenario 4: Team + Personal Configuration

```bash
# profiles/team/settings.yaml - shared by all developers
# profiles/personal/settings.yaml - your personal overrides

# Apply team settings, then your personal overrides
atmos terraform plan vpc -s dev --profile team --profile personal
```

## Profile Configuration Reference

### settings.yaml

Configure terminal, logging, and behavior settings:

```yaml
# profiles/developer/settings.yaml
settings:
  terminal:
    color: true
    max_width: 120
    syntax_highlighting:
      enabled: true
  logs:
    level: Debug
```

### auth.yaml

Configure authentication providers and identities:

```yaml
# profiles/ci/auth.yaml
auth:
  providers:
    github-oidc:
      kind: github/oidc
      region: us-east-1

  identities:
    ci-deployer:
      kind: aws/assume-role
      via:
        provider: github-oidc
      principal:
        assume_role: "arn:aws:iam::123456789012:role/GitHubActionsDeployRole"
        role_session_name: '{{ env "GITHUB_RUN_ID" }}'
```

### Custom Configuration

Profiles can override any configuration in `atmos.yaml`:

```yaml
# profiles/custom/overrides.yaml
components:
  terraform:
    base_path: ./custom-components

stacks:
  base_path: ./custom-stacks

integrations:
  github:
    gitops:
      artifact_storage:
        region: eu-west-1  # Override default region
```

## Managing Profiles

### List Available Profiles

```bash
# Show all available profiles
atmos profile list

# Show details of a specific profile
atmos profile show developer
```

### Create New Profile

```bash
# Create in project (committed to git - shared with team)
mkdir -p profiles/team
echo "settings:" > profiles/team/settings.yaml

# Create in user directory (personal - not committed)
mkdir -p ~/.config/atmos/profiles/personal
echo "settings:" > ~/.config/atmos/profiles/personal/settings.yaml
```

### Hide Profiles from Git

Use `.atmos/profiles/` for project-specific profiles that shouldn't be committed:

```bash
mkdir -p .atmos/profiles/local-dev
# Add to .gitignore
echo ".atmos/" >> .gitignore
```

## Best Practices

1. **Project profiles** (`profiles/`) - Team-shared configurations (commit to git)
2. **Hidden profiles** (`.atmos/profiles/`) - Project-specific, temporary configs (add to .gitignore)
3. **User profiles** (`~/.config/atmos/profiles/`) - Personal preferences (never committed)
4. **CI profiles** - Non-interactive, deterministic configurations for pipelines
5. **Layer profiles** - Use `--profile base --profile specific` for composition

## Troubleshooting

### Profile Not Found

```bash
$ atmos terraform plan vpc -s dev --profile nonexistent
Error: profile not found: 'nonexistent' (searched: [...])
Available profiles: developer, ci, production
Run 'atmos profile list' to see all available profiles
```

**Solution**: Check profile name spelling or create the profile.

### Multiple Profiles with Same Name

If a profile exists in multiple locations, the highest precedence location wins:

```
1. Configurable (profiles.base_path)
2. Project-hidden (.atmos/profiles/)
3. XDG user (~/.config/atmos/profiles/)
4. Project (profiles/)
```

### Configuration Not Applying

Profiles are merged in order. Later profiles override earlier ones:

```bash
# base sets color: false, developer sets color: true
atmos --profile base --profile developer ...  # color: true (developer wins)
```

## See Also

- [Atmos Profiles PRD](../../docs/prd/atmos-profiles.md) - Complete design documentation
- [CLI Configuration](https://atmos.tools/cli/configuration) - Base configuration reference
- [Authentication](https://atmos.tools/cli/commands/auth) - Auth configuration guide
