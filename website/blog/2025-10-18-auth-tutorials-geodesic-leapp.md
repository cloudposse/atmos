---
slug: auth-tutorials-geodesic-leapp
title: "New Guides for Atmos Auth: Leapp Migration and Geodesic Integration"
authors: [Benbentwo]
tags: [feature, cloud-architecture]
date: 2025-10-18
---

We've published two comprehensive guides to help you adopt and integrate `atmos auth` into your workflows: migrating from Leapp and configuring Geodesic for seamless authentication.

<!--truncate-->

## What's New

The `atmos auth` command (introduced in v1.194.1) provides native AWS IAM Identity Center authentication directly in Atmos, eliminating the need for external credential management tools. To help teams adopt this feature, we've created two detailed tutorials:

### 1. [Migrating from Leapp](/cli/commands/auth/tutorials/migrating-from-leapp)

If your team uses Leapp for credential management, this guide walks you through the migration process step-by-step:

- **Understanding the mapping** between Leapp concepts (providers, sessions, identities) and `atmos auth` configuration
- **Quick migration examples** showing side-by-side comparisons
- **Field-by-field reference** for converting Leapp sessions to Atmos identities
- **Troubleshooting common issues** during migration

The guide includes practical examples using real Leapp session configurations, making it easy to translate your existing setup.

### 2. [Configuring Geodesic with Atmos Auth](/cli/commands/auth/tutorials/configuring-geodesic)

For teams using [Geodesic](https://github.com/cloudposse/geodesic) as their DevOps toolbox, this guide explains how to integrate `atmos auth`:

- **Host-based authentication flow** - How authentication works on your laptop before starting Geodesic
- **Dockerfile configuration** with required environment variables
- **Makefile setup** for automatic authentication before shell start
- **Source profile configuration** for assume-role utilities
- **Complete working examples** showing all components together

The guide covers the authentication workflow, explaining that authentication happens on your host machine (not inside the container) and details keychain integration behavior with containers.

## Key Benefits of Atmos Auth

Using `atmos auth` provides several advantages over external credential managers:

- **Configuration as code** - Authentication config lives in `atmos.yaml` alongside your infrastructure
- **Component-level auth** - Different components can use different AWS identities
- **Workflow integration** - No separate credential management app to run
- **Cross-platform** - Works consistently on Linux, macOS, and Windows
- **Team consistency** - Everyone uses the same authentication approach

## Getting Started

1. **Read the guides**:
   - [Migrating from Leapp](/cli/commands/auth/tutorials/migrating-from-leapp)
   - [Configuring Geodesic](/cli/commands/auth/tutorials/configuring-geodesic)

2. **Review the main documentation**:
   - [Authentication User Guide](/cli/commands/auth/usage)
   - [Command Reference](/cli/commands/auth/login)

3. **Try it out**:
   ```bash
   # Configure providers and identities in atmos.yaml
   # Then authenticate
   atmos auth login

   # Verify authentication
   atmos auth whoami

   # Use with Terraform
   atmos terraform plan <component> -s <stack>
   ```

## Feedback Welcome

These guides are designed to be practical and actionable. If you encounter issues, find gaps in the documentation, or have suggestions for improvement:

- Open an issue on [GitHub](https://github.com/cloudposse/atmos/issues)
- Share your experience in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- Contribute improvements via pull request

---

**Ready to migrate?** Start with the [Leapp migration guide](/cli/commands/auth/tutorials/migrating-from-leapp) or jump straight to [Geodesic configuration](/cli/commands/auth/tutorials/configuring-geodesic) if you're already using `atmos auth`.
