---
slug: auth-and-utility-commands-no-longer-require-stacks
title: Auth and Utility Commands No Longer Require Stack Configurations
authors: [atmos]
tags: [atmos, enhancement, dx, auth]
date: 2025-10-26
---

Atmos auth, documentation, and workflow management commands now work independently of stack configurations, making it easier to use Atmos in CI/CD pipelines and alongside "native" Terraform workflows.

<!--truncate-->

## What Changed

Six Atmos commands that don't operate on stacks have been updated to no longer require stack configurations:

**Auth Commands:**
- `atmos auth env` - Export cloud credentials as environment variables
- `atmos auth exec` - Execute commands with authenticated credentials
- `atmos auth shell` - Launch an authenticated shell session

**Utility Commands:**
- `atmos list workflows` - List available workflows
- `atmos list vendor` - List vendor configurations
- `atmos docs <component>` - Display component documentation

## Why This Matters

Previously, these commands would fail with an error if you didn't have `stacks.base_path` and `stacks.included_paths` configured in your `atmos.yaml`:

```text
Error: failed to initialize atmos config
stack base path must be provided in 'stacks.base_path' config or ATMOS_STACKS_BASE_PATH' ENV variable
```

This created an unnecessary barrier for teams who wanted to:

- Use Atmos auth for credential management without adopting full stack-based configuration
- Run authentication commands in CI/CD pipelines
- Browse component documentation without setting up stacks
- Manage workflows independently of stack operations

With these changes, Atmos now works with "native" Terraform, regardless of whether you use Atmos to manage stack configuration or not (but let's face it, [Nobody Runs Native Terraform](https://cloudposse.com/blog/nobody-runs-native-terraform/)).

You can now use Atmos features incrementally:

### Just Authentication
Use Atmos for cloud credential management without any stack configuration:

```yaml
# atmos.yaml - minimal config for auth only
base_path: .

auth:
  providers:
    aws-prod:
      kind: aws-sso
      type: aws
      region: us-east-1
      sso_start_url: https://mycompany.awsapps.com/start
      sso_region: us-east-1
      sso_account_id: "123456789012"
      sso_role_name: AdministratorAccess

  identities:
    prod-admin:
      provider: aws-prod
      default: true
```

Then use it with your existing Terraform:

```bash
# Get authenticated credentials
atmos auth exec -- terraform plan

# Or export credentials for your scripts
eval $(atmos auth env)
```

### Just Vendor Management
Use Atmos to vendor and manage component dependencies without adopting stack-based configuration:

```bash
# List all vendored components
atmos list vendor

# Pull component updates
atmos vendor pull
```

### Just Documentation
Browse component README files without any stack configuration:

```bash
# View component documentation
atmos docs vpc
atmos docs eks-cluster
```

### Incremental Adoption
Start with authentication and vendor management, then gradually adopt stack-based configuration as your needs evolve. Each Atmos feature can be used independently.

## What Hasn't Changed

Commands that actually work with stacks still require stack configuration:

- `atmos list stacks`
- `atmos list components`
- `atmos describe component`
- `atmos terraform plan/apply`

This ensures that stack-dependent operations have the context they need while allowing utility commands to work independently.

## Related Links

- [PR #1717: Relax stack config requirement for commands that don't operate on stacks](https://github.com/cloudposse/atmos/pull/1717)
- [Nobody Runs Native Terraform](https://cloudposse.com/blog/nobody-runs-native-terraform/)
- [Authentication Documentation](/cli/commands/auth/usage)
- [Vendor Configuration](/cli/commands/vendor/usage)
