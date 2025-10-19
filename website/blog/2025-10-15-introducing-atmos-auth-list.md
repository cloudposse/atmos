---
slug: introducing-atmos-auth-list
title: "Introducing atmos auth list: Visualize Your Authentication Configuration"
authors: [atmos]
tags: [atmos, authentication, providers, identities, cli]
---

We're excited to announce a powerful new command for managing authentication in Atmos: `atmos auth list`. This command provides comprehensive visibility into your authentication configuration, making it easier than ever to understand and manage complex authentication chains across multiple cloud providers and identities.

<!--truncate-->

## Why atmos auth list?

As cloud infrastructure grows more complex, so does authentication management. Modern teams often work with:

- **Multiple cloud providers** (AWS, Azure, GCP, Okta)
- **Complex role assumption chains** (SSO → base role → admin role → specific account)
- **Multiple identities per environment** (dev, staging, production)
- **Team-specific access patterns** (developer, operator, security auditor)

Without proper tooling, it becomes difficult to answer simple questions like:
- "What authentication providers do we have configured?"
- "Which identities can I use to access production?"
- "How does this identity authenticate? Through which provider?"
- "What's the complete authentication chain for this admin role?"

`atmos auth list` solves these challenges by providing clear, actionable visibility into your entire authentication configuration.

## Key Features

### 🎨 Multiple Output Formats

**Table Format (Default)**
Perfect for quick overviews with formatted tables showing key attributes:

```shell
atmos auth list
```

**Tree Format**
Visualize hierarchical relationships and authentication chains:

```shell
atmos auth list --format tree
```

**JSON/YAML Export**
Integrate with scripts and automation tools:

```shell
atmos auth list --format json | jq '.identities'
atmos auth list --format yaml > auth-config.yml
```

### 🔍 Smart Filtering

Filter by providers or identities to focus on what matters:

```shell
# Show only AWS SSO providers
atmos auth list --providers=aws-sso

# View specific identities
atmos auth list --identities=admin,developer

# Show all providers (no identities)
atmos auth list --providers
```

### 🔗 Authentication Chain Visualization

Understand complex authentication flows at a glance. Chains show the complete path from provider to target identity:

```
aws-sso → base-role → admin-role → prod-account
```

This makes it immediately clear:
- Which provider authenticates you initially
- What roles you assume along the way
- The final identity you end up with

### 🎯 Real-World Examples

**Quick Overview**
```shell
$ atmos auth list

PROVIDERS
NAME      KIND       REGION      START URL                                DEFAULT
aws-sso   aws-sso    us-east-1   https://example.awsapps.com/start       ✓
okta      okta                   https://example.okta.com

IDENTITIES
NAME       KIND              VIA PROVIDER  VIA IDENTITY  DEFAULT  ALIAS
admin      aws/assume-role   aws-sso                     ✓        prod-admin
developer  aws/assume-role   aws-sso                              dev
ops        aws/assume-role   aws-sso       admin                  ops-admin
```

**Detailed Tree View**
```shell
$ atmos auth list --format tree --identities

Identities
├─ admin (aws/assume-role) [DEFAULT] [ALIAS: prod-admin]
│  ├─ Via Provider: aws-sso
│  ├─ Chain: aws-sso → admin
│  └─ Principal
│     └─ arn: arn:aws:iam::123456789012:role/AdminRole
├─ developer (aws/assume-role) [ALIAS: dev]
│  ├─ Via Provider: aws-sso
│  ├─ Chain: aws-sso → developer
│  └─ Principal
│     └─ arn: arn:aws:iam::123456789012:role/DeveloperRole
└─ ops (aws/assume-role) [ALIAS: ops-admin]
   ├─ Via Identity: admin
   ├─ Chain: aws-sso → admin → ops
   └─ Principal
      └─ arn: arn:aws:iam::987654321098:role/OpsRole
```

**Automation Integration**
```shell
# Export to JSON for CI/CD validation
atmos auth list --format json | jq -r '.providers | keys[]'

# Generate documentation
atmos auth list --format yaml > docs/auth-config.yml

# Check if specific provider exists
atmos auth list --providers=aws-sso --format json | jq -e '.providers["aws-sso"]'
```

## Understanding Authentication Chains

One of the most powerful features is authentication chain visualization. Chains show how identities authenticate through providers or other identities:

- **Simple chain**: `aws-sso → admin`  
  Direct authentication through AWS SSO

- **Multi-step chain**: `aws-sso → base-role → admin-role`  
  Authenticate via SSO, assume base role, then assume admin role

- **Complex chain**: `okta → aws-dev → prod-account → admin`  
  Authenticate through Okta, assume AWS dev role, switch to prod account, become admin

These chains can be arbitrarily long, supporting even the most complex enterprise authentication scenarios.

## Integration with Existing Commands

`atmos auth list` complements the existing authentication commands:

- **`atmos auth whoami`** - See your current authentication status
- **`atmos auth login`** - Authenticate with a provider
- **`atmos auth list`** - **NEW!** View all available providers and identities
- **`atmos auth validate`** - Validate authentication configuration
- **`atmos auth env`** - Export credentials as environment variables

Together, these commands provide a complete authentication workflow from discovery to usage.

## Technical Highlights

For those interested in the implementation:

- **Built with charmbracelet**: Uses the excellent [Bubble Tea](https://github.com/charmbracelet/bubbletea) ecosystem for beautiful terminal output
- **Recursive chain resolution**: Handles arbitrarily deep authentication chains with circular dependency detection
- **Theme integration**: Respects Atmos color scheme for consistent CLI experience
- **Comprehensive testing**: 70%+ test coverage with unit and integration tests
- **Performance instrumentation**: Built-in performance tracking like all Atmos commands

## Get Started

`atmos auth list` is available in Atmos `v1.x.x` and later. To get started:

1. **Upgrade Atmos** to the latest version
2. **List your configuration**: Run `atmos auth list`
3. **Explore the formats**: Try `--format tree`, `json`, and `yaml`
4. **Filter as needed**: Use `--providers` and `--identities` to focus

For full documentation, see the [atmos auth list command reference](/cli/commands/auth/list).

## What's Next?

This is just the beginning. We're continuing to improve authentication management in Atmos with:

- Enhanced visualization options
- Interactive TUI mode for browsing configurations
- Integration with secret managers
- Authentication policy validation
- Audit logging for authentication events

We'd love to hear your feedback! Let us know what you think on [GitHub](https://github.com/cloudposse/atmos) or join our [community Slack](https://slack.cloudposse.com/).

---

*Happy authenticating! 🔐*
