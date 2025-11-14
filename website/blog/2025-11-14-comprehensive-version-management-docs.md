---
slug: comprehensive-version-management-documentation
title: "New Comprehensive Version Management Documentation"
authors: [osterman]
tags: [atmos, documentation, version-management, versioning, deployment-strategies]
---

We've published comprehensive new documentation on version management patterns for Atmos, providing clear guidance on managing infrastructure component versions across your environments. Whether you're running a small startup or a large enterprise, understanding version management patterns is crucial for safe, reliable infrastructure deployments.

<!--truncate-->

## What's New

The new documentation provides a complete framework for understanding and implementing version management in Atmos, including:

### 🎯 Clear Strategy Recommendations

**[Continuous Version Deployment](/design-patterns/version-management/continuous-version-deployment)** is our recommended default pattern. This trunk-based approach:

- **Promotes convergence** across all environments through progressive rollout
- **Simplifies operations** with no complex version tracking or branch management
- **Enables easy previews** to see change impacts across dependent environments
- **Supports rapid iteration** with confident, frequent deployments

### 📚 Comprehensive Pattern Documentation

All version management patterns are fully documented under a unified [Version Management](/design-patterns/version-management) section:

#### Deployment Strategies

**[Continuous Version Deployment](/design-patterns/version-management/continuous-version-deployment)** - The recommended trunk-based approach where all environments converge to the same version through automated progressive rollout. Perfect for teams embracing modern DevOps practices with strong CI/CD automation.

**[Git Flow: Branches as Channels](/design-patterns/version-management/git-flow-branches-as-channels)** - Long-lived branches map to release channels for teams that need prolonged divergence or already practice Git Flow workflows. Use when you need version control to represent current state versus desired state.

#### Folder Organization Approaches

Within Continuous Version Deployment, choose how to organize component folders:

**[Folder-Based Versioning](/design-patterns/version-management/folder-based-versioning)** - Simple, explicit folder structures (`vpc/`, `eks/`, `rds/`). What you see is what you get.

**[Release Tracks/Channels](/design-patterns/version-management/release-tracks-channels)** - Named release channels (`alpha/vpc`, `beta/vpc`, `prod/vpc`) where environments subscribe to moving tracks.

**[Strict Version Pinning](/design-patterns/version-management/strict-version-pinning)** - Explicit SemVer versions (`vpc/1.2.3`, `vpc/2.0.0`) for vendored components and shared libraries.

#### Complementary Techniques

**[Vendoring Component Versions](/design-patterns/version-management/vendoring-components)** - Automate copying component versions from external sources with manifest tracking. Works with any deployment strategy.

### 💡 When to Use Each Pattern

The documentation includes clear guidance on choosing the right pattern for your organization:

**Use Continuous Version Deployment when:**
- You embrace trunk-based development
- All environments should eventually converge to the same version
- You want preview capabilities across all environments
- You have strong CI/CD automation

**Use Git Flow when:**
- Your organization already practices Git Flow branch management
- You need prolonged divergence between environments
- You're comfortable with cherry-picking and merge strategies
- You want version control to represent current vs. desired state

### 🔍 Industry Context

The documentation frames Atmos's approach within industry best practices:

> Decoupling deploy from release increases speed and stability when delivering software.
>
> — LaunchDarkly

> More-frequent deployments reduce the risk associated with change, while business stakeholders retain control over when features are released to end users.
>
> — Thoughtworks Technology Radar

Atmos achieves decoupling through **progressive deployment automation** where CI/CD gates control when environments receive changes, and comprehensive plan previews enable informed release decisions.

### 🛠️ Practical Improvements

Throughout the documentation, you'll find:

- **Code examples first** - Developers absorb patterns faster through examples
- **Complete workspace_key_prefix coverage** - Critical for Terraform state management across version changes
- **Go template documentation** - Understand `{{.Component}}`, `{{.Version}}`, and other template variables
- **Stack-level base_path alternative** - DRY alternative to repeating metadata.component paths
- **Anti-patterns section** - Learn what to avoid (vendoring to same path, inconsistent conventions, etc.)

### 📊 Comparison Tables

Quick-reference tables help you understand trade-offs:

| Strategy | Development Model | Convergence | Automation | Best For |
|----------|------------------|-------------|------------|----------|
| **Continuous Version Deployment** | Trunk-based | Very High | Required | Most teams - simple, automated convergence |
| **Git Flow** | Branch-based | Medium | Optional | Legacy systems with established branch workflows |

## Getting Started

Start with the [Version Management overview](/design-patterns/version-management) to understand all available patterns, then dive into [Continuous Version Deployment](/design-patterns/version-management/continuous-version-deployment) for our recommended default approach.

The documentation includes:
- Real-world examples with complete configurations
- Step-by-step implementation guides
- Troubleshooting sections for common issues
- Migration paths between patterns

## Key Takeaway

**The best approach is what your engineering organization already follows.** If your team has established Git Flow practices, extending them to infrastructure management keeps the mental model consistent. If you're building modern cloud-native infrastructure with strong automation, Continuous Version Deployment provides the simplest path forward.

Ready to explore? Check out the new [Version Management documentation](/design-patterns/version-management) today!
