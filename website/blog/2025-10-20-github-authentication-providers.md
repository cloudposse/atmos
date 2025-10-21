---
slug: github-authentication-providers
title: "Unified GitHub Authentication: From Local Development to CI/CD"
authors: [atmos]
tags: [feature, github, authentication, ci-cd]
---

Managing GitHub access tokens across local development, CI/CD pipelines, and team environments has always been fragmented. Today, we're introducing comprehensive GitHub authentication in Atmos that works everywhere - from your laptop to GitHub Actions, using the same configuration.

<!--truncate-->

## The GitHub Token Challenge

Platform teams working with infrastructure as code face a persistent challenge: **GitHub authentication is everywhere, yet every tool handles it differently**. You need GitHub tokens for:

- **Terraform** downloading private modules from GitHub
- **Helmfile** accessing private chart repositories
- **CI/CD pipelines** interacting with the GitHub API
- **Local development** testing infrastructure changes
- **Git operations** cloning private repositories

Each context typically requires a different approach:

- Personal Access Tokens (PATs) hardcoded in environment variables
- GitHub Actions `GITHUB_TOKEN` with limited permissions
- OAuth apps with complex setup
- GitHub Apps requiring JWT signing and installation tokens

The result? **Token sprawl**. Credentials scattered across `.env` files, CI/CD secrets, shell profiles, and wiki pages. When someone leaves the team or a token expires, good luck finding everywhere it's used.

## A Better Way: Unified GitHub Authentication

We've added three GitHub authentication providers to Atmos that work seamlessly across all environments:

### 1. GitHub User Authentication (OAuth Device Flow)

Perfect for local development, the `github/user` provider uses OAuth Device Flow - the same approach used by the official GitHub CLI (`gh`). We were inspired by tools like [`ghtkn`](https://github.com/suzuki-shunsuke/ghtkn) which demonstrated how elegant Device Flow can be for CLI applications.

```yaml
auth:
  providers:
    github-user:
      kind: github/user
      # Optional - defaults to GitHub CLI's OAuth App
      scopes:
        - repo
        - read:org
```

**What makes this special:**

- **Zero configuration** - Uses the same OAuth App as GitHub CLI by default
- **Two authentication flows** - Device Flow (default) for remote sessions, Web Application Flow for local development
- **OS keychain integration** - Tokens securely stored in macOS Keychain, Windows Credential Manager, or Linux Secret Service
- **Short-lived tokens** - 8-hour lifetime by default, automatically refreshed
- **Fine-grained scopes** - Request only the permissions you need

The Device Flow works beautifully for remote/SSH sessions where you can't open a browser locally. For local development, the Web Application Flow opens your browser automatically and completes authentication in seconds.

### 2. GitHub App Authentication

For production and CI/CD environments, GitHub Apps provide the most secure and granular access control. The `github/app` provider handles the complexity of JWT signing and installation token management:

```yaml
auth:
  providers:
    infra-bot:
      kind: github/app
      spec:
        app_id: "123456"
        installation_id: "789012"
        private_key_path: "/path/to/key.pem"
```

**Why GitHub Apps over PATs:**

- **Repository-scoped permissions** - Grant access to specific repositories only, not your entire organization
- **Granular permissions** - Request only the capabilities needed (e.g., "read contents" vs. "admin everything")
- **No user account required** - Apps aren't tied to individual users, solving the "bot account" problem
- **Audit trail** - All API calls clearly attributed to the app, not a personal account
- **Automatic token rotation** - Installation tokens expire after 1 hour and are refreshed automatically

GitHub Apps are particularly powerful for teams managing private Terraform modules. Instead of sharing a PAT that grants access to everything, create a GitHub App with read-only access to specific module repositories. When a team member leaves, their access is automatically revoked without affecting the app.

### 3. GitHub OIDC (Actions Integration)

For GitHub Actions workflows, the `github/oidc` provider uses GitHub's native OIDC token:

```yaml
auth:
  providers:
    github-actions:
      kind: github/oidc
```

**The elegance of OIDC:**

- **No stored secrets** - Tokens are issued by GitHub Actions at runtime
- **Claims-based validation** - Tokens include repository, workflow, and actor information
- **Automatic rotation** - New token for each workflow run
- **Zero configuration** - Just add `permissions: id-token: write` to your workflow

This is the same OIDC mechanism you'd use to authenticate to AWS or other cloud providers from GitHub Actions, but now it works with GitHub itself.

## One Configuration, Every Environment

Here's where it gets powerful. Define your GitHub authentication once:

```yaml
# atmos.yaml
auth:
  providers:
    # Local development
    github-user:
      kind: github/user
      scopes: [repo, read:org]

    # CI/CD
    github-actions:
      kind: github/oidc

    # Production automation
    terraform-bot:
      kind: github/app
      spec:
        app_id: "123456"
        installation_id: "789012"
        private_key_env: "GITHUB_APP_PRIVATE_KEY"

  identities:
    # Local development identity
    dev:
      kind: github/token
      via:
        provider: github-user

    # CI/CD identity
    ci:
      kind: github/token
      via:
        provider: github-actions

    # Production identity
    prod:
      kind: github/token
      via:
        provider: terraform-bot
```

Now use the same commands everywhere:

```bash
# Local development
atmos auth login --identity dev
eval $(atmos auth env --identity dev)
terraform init  # Downloads private modules using GITHUB_TOKEN

# In GitHub Actions
- run: atmos auth login --identity ci
- run: eval $(atmos auth env --identity ci)
- run: terraform init

# Production automation
- run: atmos auth login --identity prod
- run: terraform apply
```

## Git Credential Helper Integration

Atmos includes a Git credential helper, so authenticated sessions automatically work with Git operations:

```bash
# Configure once
git config --global credential.helper "atmos auth git-credential"

# Now Git operations just work
git clone git@github.com:my-org/private-repo.git
# Uses your Atmos-managed GitHub token automatically
```

This is particularly useful in CI/CD where you need to clone private repositories but don't want to manage deploy keys or personal access tokens.

## Real-World Example: Terraform Private Modules

Before Atmos auth, accessing private Terraform modules in CI/CD typically looked like this:

```yaml
# Old approach - fragile and insecure
- name: Setup GitHub token
  run: |
    git config --global url."https://x-access-token:${{ secrets.GH_PAT }}@github.com/".insteadOf "https://github.com/"
```

**Problems:**
- PAT must have access to ALL repositories (overly broad permissions)
- Token shared across the entire organization
- When someone leaves, you create a new PAT and update every workflow
- No audit trail showing which workflow used which token

With Atmos:

```yaml
# New approach - secure and maintainable
- name: Authenticate to GitHub
  run: atmos auth login --identity ci

- name: Download modules and apply
  run: |
    eval $(atmos auth env --identity ci)
    terraform init
    terraform apply
```

**Benefits:**
- OIDC token scoped to the specific workflow run
- No long-lived secrets to rotate
- Clear audit trail in GitHub's security logs
- Same configuration works locally and in CI/CD

## Fine-Grained Permissions with GitHub Apps

One of the most compelling features of GitHub Apps is repository-scoped permissions. Let's say you have:

- 50 repositories in your organization
- 5 private Terraform module repositories
- 100 team members

**With a PAT:**
- Create a "bot" user account
- Grant it access to all 5 module repositories
- Share the PAT across the team
- Hope nobody accidentally commits it to a public repo
- When the bot user leaves (someone manages these accounts... right?), create a new PAT

**With a GitHub App:**
- Create an app with read-only access to the 5 module repositories
- Install it once in your organization
- Every team member can use it without sharing credentials
- App access automatically revoked when someone leaves the org
- Clear audit logs showing exactly when and where modules were accessed

This is especially powerful for organizations using Atmos across multiple teams. Each team can have its own GitHub App with access to only their modules, while the configuration lives in the shared `atmos.yaml`.

## Acknowledgments

Building authentication that works well is hard. We're grateful to the teams behind:

- **[GitHub CLI (`gh`)](https://github.com/cli/cli)** - Demonstrated the power of OAuth Device Flow for CLI tools and proved that native GitHub authentication could be elegant. We use the same OAuth App by default, providing instant compatibility.

- **[`ghtkn`](https://github.com/suzuki-shunsuke/ghtkn)** - Showed us how clean GitHub token management could be with OS keychain integration. Suzuki-Shunsuke's work on developer experience inspired our focus on making authentication invisible when it's working correctly.

- **[aws-vault](https://github.com/99designs/aws-vault)** and **[saml2aws](https://github.com/Versent/saml2aws)** - Pioneers in CLI credential management that taught us the patterns we've applied to GitHub authentication.

These tools solved real problems and proved that CLI authentication doesn't have to be painful. We've learned from all of them.

## Getting Started

### Local Development

```bash
# Authenticate once
atmos auth login --identity dev

# Token automatically available to all tools
terraform init
helm repo add my-charts https://my-org.github.io/charts
git clone git@github.com:my-org/private-repo.git
```

### GitHub Actions

```yaml
name: Deploy Infrastructure
on: push

permissions:
  id-token: write  # Required for OIDC
  contents: read

jobs:
  deploy:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - name: Setup Atmos
        uses: cloudposse/github-action-setup-atmos@v2

      - name: Authenticate
        run: atmos auth login --identity ci

      - name: Deploy
        run: |
          eval $(atmos auth env --identity ci)
          atmos terraform apply infra -s prod
```

### GitHub App for Private Modules

```yaml
# atmos.yaml
auth:
  providers:
    module-reader:
      kind: github/app
      spec:
        app_id: "${GITHUB_APP_ID}"
        installation_id: "${GITHUB_INSTALLATION_ID}"
        private_key_env: "GITHUB_APP_PRIVATE_KEY"

  identities:
    modules:
      kind: github/token
      via:
        provider: module-reader
```

Store your GitHub App credentials as GitHub Actions secrets, and every workflow automatically has access to private modules without sharing PATs.

## What's Next

This is just the beginning. We're exploring:

- **Additional providers** - GitLab, Bitbucket, and other VCS platforms
- **Token lifecycle policies** - Automatic rotation and expiration rules
- **Enhanced audit logging** - Track exactly when and where tokens are used
- **Credential sharing boundaries** - Fine-grained control over which teams can use which providers

## Try It Today

GitHub authentication is available now in Atmos. The `github/user` and `github/oidc` providers work immediately with zero configuration. GitHub Apps require creating an app in your organization settings, but the improved security and auditability are worth the five-minute setup.

We'd love to hear how you're using it. Have questions about GitHub Apps? Need help configuring OIDC in your workflows? Join us in [discussions](https://github.com/cloudposse/atmos/discussions) or open an issue.

---

**Resources:**
- [GitHub User Authentication Documentation](/cli/commands/auth/providers/github-user)
- [GitHub App Authentication Documentation](/cli/commands/auth/providers/github-app)
- [GitHub OIDC Authentication Documentation](/cli/commands/auth/providers/github-oidc)
- [Git Credential Helper Setup](/cli/commands/auth/git-credential)
