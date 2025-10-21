---
slug: introducing-atmos-auth-shell
title: "Introducing atmos auth shell: Secure, Scoped Shell Sessions for Cloud Identities"
authors: [atmos]
tags: [feature, atmos-auth, security, workflows]
---

We're excited to announce a new command in Atmos that makes working with cloud identities more secure and convenient: `atmos auth shell`. This powerful new feature launches an isolated shell session with your cloud credentials pre-configured, providing clear session boundaries and preventing credential leakage.

<!--truncate-->

## The Problem: Secure Multi-Identity Workflows

When managing cloud infrastructure, you frequently need to work with different **identities**, **roles**, or **accounts** throughout your day. This creates several security and usability challenges:

1. **Credential Persistence**: Environment variables from previous sessions can accidentally persist, leading to commands running against the wrong identity
2. **Context Confusion**: It's easy to lose track of which identity is currently active in your shell
3. **Credential Leakage**: Long-lived credentials in your environment can be exposed to child processes or accidentally shared
4. **Multi-Identity Management**: Difficult to work with multiple identities simultaneously without credential conflicts

This workflow becomes risky when switching between identities:

```shell
# Traditional workflow
eval $(atmos auth env --identity prod-admin --format bash)
aws s3 ls
kubectl get pods
# ... many more commands ...
# Later: which identity am I using now? 🤔
```

Every time you switch identities or your session expires, you risk:
- Running commands against the wrong environment
- Leaving stale credentials in your shell
- Accidentally exposing credentials to other processes

## The Solution: `atmos auth shell`

The `atmos auth shell` command solves this by creating **isolated shell sessions** scoped to a specific identity. Think of it like [aws-vault exec](https://github.com/99designs/aws-vault) but for all your cloud identities managed by Atmos.

This approach provides:

- **Session Scoping**: Credentials exist only within the shell session
- **Clear Boundaries**: Exiting the shell removes all credentials—no persistence
- **Visual Context**: Shell prompt indicates which identity is active
- **Multi-Identity Support**: Run multiple shells with different identities simultaneously in separate terminals
- **Security by Design**: Credentials don't leak to parent shell or persist after exit
- **Conventional Pattern**: Follows established patterns from tools like `aws-vault exec`

Simply run:

```shell
atmos auth shell --identity prod-admin
```

You now have a dedicated shell environment with prod-admin credentials. When you `exit`, the credentials are gone.

## How It Works

When you run `atmos auth shell`, Atmos:

1. **Authenticates** with your configured identity (AWS SSO, SAML, static credentials, etc.).
2. **Sets up environment variables** (like `AWS_ACCESS_KEY_ID`, `AWS_PROFILE`, etc.).
3. **Launches a subshell** with those credentials active.
4. **Shows context** in your prompt so you always know which identity you're using.
5. **Cleans up** when you exit—no credentials persist in your parent shell.

## Usage Examples

### Basic Usage

Launch a shell with a specific identity:

```shell
atmos auth shell --identity prod-admin
# You're now in a subshell with prod-admin credentials
$ aws s3 ls
bucket-prod-1
bucket-prod-2

$ terraform plan
# Uses prod-admin credentials automatically

$ exit
# Credentials are gone, back to your normal shell
```

### Multiple Identities Simultaneously

Open different terminals for different identities:

```shell
# Terminal 1: Production admin
atmos auth shell --identity prod-admin
$ aws ecs list-clusters  # Works against prod

# Terminal 2: Staging developer (different window)
atmos auth shell --identity staging-dev
$ kubectl get pods  # Works against staging

# Terminal 3: Dev reader (another window)
atmos auth shell --identity dev-reader
$ aws s3 ls  # Read-only access to dev
```

Each shell is completely isolated with its own credential context. No risk of confusion or credential conflicts.

### Custom Shell

Use your preferred shell:

```shell
# Use zsh instead of default shell
atmos auth shell --identity prod-admin --shell /bin/zsh

# Or set via environment variable
export ATMOS_AUTH_SHELL=/bin/fish
atmos auth shell --identity prod-admin
```

## When to Use `atmos auth shell` vs `atmos auth env`

Atmos provides two complementary ways to work with credentials:

### Use `atmos auth shell` when:

✅ Working **interactively** with cloud resources
✅ Switching between **multiple identities** frequently
✅ You want **clear session boundaries** and credential isolation
✅ Need to run **multiple identity contexts simultaneously** in different terminals
✅ You want **visual confirmation** of which identity is active

**Best for**: Day-to-day interactive cloud operations, exploratory work, debugging

### Use `atmos auth env` when:

✅ You want to **export credentials** to your current shell
✅ Integrating with **CI/CD pipelines** or scripts
✅ You need **fine-grained control** over credential lifetime
✅ Building **automation** that requires specific environment variables
✅ Working in environments where **subshells** aren't practical

**Best for**: Scripts, automation, CI/CD, integration with other tools

## Security Benefits

The `atmos auth shell` approach provides several security advantages:

1. **No Credential Persistence**: When you exit the shell, credentials are completely removed—they don't linger in your environment

2. **Isolation**: Credentials are scoped only to the subshell and its child processes, not your entire terminal session

3. **Clear Context**: You always know which identity you're using because it's scoped to that shell session

4. **Reduced Accident Risk**: Can't accidentally run commands against the wrong identity because each shell has explicit, isolated credentials

5. **Multi-Identity Safety**: Running multiple shells with different identities eliminates the risk of credential conflicts or using stale environment variables

## Comparison to AWS Vault

If you're familiar with [aws-vault](https://github.com/99designs/aws-vault), `atmos auth shell` provides similar functionality but for all cloud providers and identity types managed by Atmos:

| Feature | aws-vault exec | atmos auth shell |
|---------|---------------|------------------|
| Scoped shell sessions | ✅ | ✅ |
| Credential isolation | ✅ | ✅ |
| Multi-profile support | ✅ AWS only | ✅ Multi-cloud |
| Clear session boundaries | ✅ | ✅ |
| SSO integration | ✅ AWS SSO | ✅ AWS SSO, SAML, GitHub, etc. |
| Custom shells | ✅ | ✅ |

## Real-World Workflows

### Scenario 1: Development Workflow

```shell
# Morning: Start with dev environment
atmos auth shell --identity dev-admin
$ terraform apply -auto-approve  # Safe in dev
$ aws s3 sync ./build s3://dev-bucket/
$ exit

# Afternoon: Deploy to staging
atmos auth shell --identity staging-admin
$ terraform plan  # Review changes for staging
$ terraform apply
$ kubectl rollout status deployment/app
$ exit

# Evening: Quick prod check (read-only)
atmos auth shell --identity prod-reader
$ aws cloudwatch get-metric-statistics ...
$ kubectl get pods -n production
$ exit
```

Each session is isolated and scoped. No risk of accidentally running `terraform apply` against prod with dev credentials still active.

### Scenario 2: Multi-Account Management

```shell
# Terminal 1: Main account operations
atmos auth shell --identity main-account-admin
$ aws organizations list-accounts
$ aws iam list-users

# Terminal 2: Separate workload account (simultaneously)
atmos auth shell --identity workload-account-admin
$ aws ecs update-service ...
$ aws rds describe-db-instances

# Terminal 3: Security account (audit/compliance)
atmos auth shell --identity security-account-reader
$ aws cloudtrail lookup-events ...
$ aws config describe-compliance-by-config-rule
```

All three sessions run simultaneously with different credentials, no conflicts.

### Scenario 3: Pair Programming

When pair programming or doing live demos, `atmos auth shell` makes it clear which identity is active:

```shell
# Clearly scoped session for demo
atmos auth shell --identity demo-account-readonly

# Everyone can see which account you're in
$ aws s3 ls  # Safe, read-only demo account
$ kubectl get pods  # Also read-only

# When done, exit and credentials are gone
$ exit
```

## Get Started

The `atmos auth shell` command is available in Atmos v1.X.X and later.

Try it today:

```shell
# List your configured identities
atmos auth list

# Launch a shell with an identity
atmos auth shell --identity <identity-name>

# Work with your cloud resources
aws s3 ls
terraform plan
kubectl get pods

# Exit when done
exit
```

For more information, see:
- [Authentication commands](/cli/commands/auth/usage)
- [atmos auth login documentation](/cli/commands/auth/login)
- [atmos auth env documentation](/cli/commands/auth/env)

## Feedback Welcome

We'd love to hear how you're using `atmos auth shell` in your workflows! Share your experience:

- [Join our Slack community](https://slack.cloudposse.com/)
- [Open a discussion on GitHub](https://github.com/cloudposse/atmos/discussions)
- [Report issues or suggest improvements](https://github.com/cloudposse/atmos/issues)
