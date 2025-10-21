---
slug: introducing-atmos-auth-logout
title: "Introducing atmos auth logout: Making Cloud Credential Cleanup Easy"
authors: [atmos]
tags: [feature, authentication, security]
---

Most cloud practitioners never log out of their cloud provider identities. Not because they don't want to, but because the tooling doesn't make it easy. Credentials persist in `~/.aws/credentials`, `~/.azure/`, and `~/.config/gcloud/` long after they should have been removed, creating security risks and credential sprawl.

Atmos Auth now provides an explicit, straightforward way to clean up your cloud credentials with `atmos auth logout`.

<!--truncate-->

## The Problem: Credential Cleanup is Hard

When you authenticate with cloud providers through their CLI tools, credentials get scattered across your filesystem:

- **AWS**: `~/.aws/credentials`, `~/.aws/config`, session tokens
- **Azure**: `~/.azure/` directory with multiple authentication artifacts
- **Google Cloud**: `~/.config/gcloud/` with various credential files

Most tools don't provide a simple logout command. You're left to:
- Manually delete credential files (which ones exactly?)
- Navigate through provider-specific web consoles to revoke tokens
- Hope that session expiration handles cleanup for you

This leads to **credential sprawl**: old, forgotten credentials littering your system, many still valid and exploitable.

## The Solution: Explicit Logout Commands

Atmos Auth makes credential cleanup explicit and easy with the `atmos auth logout` command:

```shell
# Log out of a specific identity
atmos auth logout my-identity

# Log out of all identities for a provider
atmos auth logout --provider aws

# Log out of everything
atmos auth logout --all
```

Behind the scenes, Atmos:
1. **Revokes active sessions** with the cloud provider
2. **Removes credential files** from your filesystem
3. **Cleans up configuration** for that identity
4. **Confirms the cleanup** with clear feedback

## How It Works

### Log Out of a Specific Identity

```shell
$ atmos auth logout production-admin
✓ Successfully logged out of identity 'production-admin'
```

This removes:
- Credentials from `~/.atmos/credentials.yaml`
- Provider-specific credential files
- Active session tokens
- Configuration entries

### Log Out of All Identities for a Provider

```shell
$ atmos auth logout --provider aws
✓ Successfully logged out of 3 AWS identities
```

Useful when switching between AWS accounts or rotating credentials across multiple profiles.

### Log Out of Everything

```shell
$ atmos auth logout --all
✓ Successfully logged out of 5 identities across 2 providers
```

Perfect for:
- End-of-day security practice
- Switching between client environments
- Preparing for credential rotation
- Decommissioning a workstation

## When to Use Logout

**Daily security practice:**
```shell
# At the end of your workday
atmos auth logout --all
```

**Before credential rotation:**
```shell
# Clean up before rotating AWS credentials
atmos auth logout --provider aws
atmos auth login production-admin
```

**Switching contexts:**
```shell
# Moving from client A to client B
atmos auth logout client-a-admin
atmos auth login client-b-admin
```

**Troubleshooting authentication:**
```shell
# Clear stale credentials
atmos auth logout my-identity
atmos auth login my-identity
```

## Security Benefits

### 1. **Reduced Credential Exposure**
Active credentials exist only when you need them. No persistent tokens waiting to be compromised.

### 2. **Clear Audit Trail**
Explicit logout events create audit records, making it clear when credentials were actively removed (not just expired).

### 3. **Session Hygiene**
Regular logout prevents credential sprawl and ensures you're always using fresh, valid credentials.

### 4. **Defense in Depth**
Even if an attacker gains filesystem access, logged-out credentials can't be used.

## Implementation Details

For Atmos contributors, `atmos auth logout` is implemented through:
- **Provider-agnostic interface** in `pkg/auth/identities/`
- **Cloud-specific cleanup** in `pkg/auth/cloud/` (AWS, Azure, GCP)
- **File management** in `pkg/auth/cloud/{provider}/files.go`
- **Session revocation** through provider SDKs

See the [full implementation](https://github.com/cloudposse/atmos/pull/1656) for details.

## Get Started

```shell
# List your current identities
atmos auth list

# Log out of a specific identity
atmos auth logout <identity-name>

# See all logout options
atmos auth logout --help
```

## Get Involved

- [Documentation](/cli/commands/auth/logout)
- [Feature Request Discussion](https://github.com/cloudposse/atmos/discussions)
- [Report Issues](https://github.com/cloudposse/atmos/issues)

Make credential cleanup a regular part of your cloud security practice with `atmos auth logout`.
