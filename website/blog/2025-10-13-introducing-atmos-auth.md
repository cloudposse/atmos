---
slug: introducing-atmos-auth
title: "Introducing Atmos Auth: Native Cloud Authentication for Platform Teams"
authors: [aknysh]
tags: [feature, cloud-architecture]
date: 2025-10-13
---

We're introducing `atmos auth` - native cloud authentication built directly into Atmos. After years of solving the same authentication problems repeatedly across different tools and teams, we've built a solution that works whether you adopt the entire Atmos framework or just need better credential management.

<!--truncate-->

## The Problem We're Solving

Platform teams face a persistent authentication challenge: **there's no unified, configuration-as-code approach to managing cloud credentials**. Teams typically resort to:

- **Standalone tools** like [Leapp](https://www.leapp.cloud/) - the closest alternative we've found, but requires a separate GUI application
- **Manual credential management** - copying and pasting temporary credentials, managing profiles across team members
- **Wiki-based documentation** - maintaining wiki pages with authentication instructions that quickly become outdated
- **Multiple point solutions** - aws-vault for AWS, different tools for Azure, GCP, etc.

This creates several pain points:

1. **No shared configuration** - Each team member configures authentication independently, leading to inconsistencies and support burden
2. **Context switching** - Jumping between credential managers, browsers, and CLI tools breaks workflow
3. **Configuration drift** - When access requirements change, teams must update wikis, Slack messages, and individual setups
4. **Framework lock-in** - Authentication solutions are often tied to specific tools or workflows

Over the years, we've been heavily inspired by tools like [aws-vault](https://github.com/99designs/aws-vault), [aws2saml](https://github.com/Versent/saml2aws), and other utilities we cut our teeth on. These tools solved specific problems well, but we kept reimplementing authentication for each new project, cloud provider, or workflow.

## Why We Built This

Authentication is **integral to every platform team's ability to deliver infrastructure**. When your team spends time debugging credential issues, updating wiki pages, or helping teammates configure access, that's time not spent delivering value.

We were tired of solving the same problem over and over. So we built native authentication into Atmos, following these principles:

- **Configuration as code** - Authentication config lives in `atmos.yaml` alongside your infrastructure
- **Shared by default** - Commit once, everyone on the team uses the same configuration
- **Cloud-agnostic** - Works with AWS IAM Identity Center, SAML providers, and extensible to other providers
- **Standalone or integrated** - Use it even if you don't adopt the whole Atmos framework

## What Atmos Auth Provides

### Native AWS IAM Identity Center Support

```yaml
auth:
  providers:
    company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://company.awsapps.com/start/

  identities:
    prod-admin:
      kind: aws/permission-set
      via:
        provider: company-sso
      principal:
        name: "AdministratorAccess"
        account:
          name: "production"
```

### Simple Authentication Flow

```bash
# Authenticate once
atmos auth login

# Verify who you are
atmos auth whoami

# Use with Terraform, Helmfile, or any tool
atmos terraform plan vpc -s prod
```

### Component-Level Authentication

Different components can use different identities:

```yaml
components:
  terraform:
    vpc:
      settings:
        auth:
          identity: network-admin

    database:
      settings:
        auth:
          identity: data-admin
```

### Credential Security

- **Browser-based SSO flow** - Leverages your existing IAM Identity Center authentication
- **Temporary credentials** - Short-lived credentials that expire automatically
- **OS keyring integration** - Optionally stores refresh tokens in macOS Keychain, Linux Secret Service, or Windows Credential Manager
- **No static credentials** - Never stores long-term access keys

## Getting Started

### 1. Configure Authentication in `atmos.yaml`

```yaml
auth:
  providers:
    my-company-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://mycompany.awsapps.com/start/

  identities:
    my-identity:
      default: true
      kind: aws/permission-set
      via:
        provider: my-company-sso
      principal:
        name: "PowerUserAccess"
        account:
          name: "development"
```

### 2. Authenticate

```bash
atmos auth login
```

This opens your browser for IAM Identity Center authentication, then stores temporary credentials locally.

### 3. Use with Your Infrastructure

```bash
# Terraform
atmos terraform plan <component> -s <stack>

# Helmfile
atmos helmfile deploy <component> -s <stack>

# Export credentials for other tools
eval $(atmos auth env)
```

## Use It Standalone

You don't need to adopt Atmos's stack management, workflows, or component architecture to use `atmos auth`. Install Atmos, configure `atmos.yaml` with just the `auth` section, and use it for credential management:

```bash
# Just for authentication
atmos auth login
eval $(atmos auth env)

# Now use any AWS tool
aws s3 ls
aws ecs list-clusters
kubectl get pods
```

## What's Next

We're continuing to expand authentication capabilities:

- Additional provider types (Azure AD, GCP, generic SAML)
- Enhanced session management
- Credential caching optimizations
- IDE integrations

## Documentation and Support

- [Authentication User Guide](/cli/commands/auth/usage)
- [Command Reference](/cli/commands/auth/login)
- [Migrating from Leapp](/cli/commands/auth/tutorials/migrating-from-leapp)
- [Configuring Geodesic](/cli/commands/auth/tutorials/configuring-geodesic)

## Get Involved

Authentication is critical infrastructure for platform teams. If you have feedback, feature requests, or want to contribute:

- Open an issue on [GitHub](https://github.com/cloudposse/atmos/issues)
- Share your use cases in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- Contribute provider implementations or enhancements

---

**Ready to try it?** Install Atmos v1.194.1 or later and configure authentication in your `atmos.yaml`. You can use it standalone for credential management or as part of your complete Atmos infrastructure workflow.
