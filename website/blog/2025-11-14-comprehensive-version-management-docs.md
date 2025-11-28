---
slug: comprehensive-version-management-documentation
title: "New Comprehensive Version Management Documentation"
authors: [osterman]
tags: [atmos, documentation, version-management, versioning, deployment-strategies]
---

When you deploy infrastructure across multiple environments—dev, staging, production—you need a way to manage which version of each component runs where. Maybe your VPC module in dev is testing new CIDR ranges, while production stays on the stable version until you're confident the changes work.

That's **version management**: deciding how different versions of your infrastructure components flow through your environments.

The obvious answer—pin every version in every environment—turns out to optimize for the wrong thing. Strict pinning creates divergence by default: environments drift apart unless you constantly update pins. It weakens feedback loops because lower environments stay on old versions, hiding cross-environment impacts. And at scale, you face PR storms from automated dependency updates.

So what's the right approach? It depends. We've documented these strategies as **design patterns**—proven approaches that optimize for different goals. Some prioritize convergence and fast feedback; others prioritize control and reproducibility. The best choice depends on your organization's culture, team size, and how you already think about software delivery.

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

**[Continuous Version Deployment](/design-patterns/version-management/continuous-version-deployment)** - The recommended trunk-based approach where all environments converge to the same version through automated progressive rollout. As LaunchDarkly puts it, "Decoupling deploy from release increases speed and stability when delivering software." Atmos achieves this through CI/CD gates that control when environments receive changes.

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

**The best strategy is one that follows how your team already thinks about software delivery.** As the Thoughtworks Technology Radar notes, "More-frequent deployments reduce the risk associated with change, while business stakeholders retain control over when features are released."

If your team has established Git Flow practices, extend them to infrastructure—keeping the mental model consistent matters. If you embrace trunk-based development with strong automation, Continuous Version Deployment is your simplest path forward.

Ready to explore? Check out the new [Version Management documentation](/design-patterns/version-management) today!

## Share Your Experience

Have you found another versioning strategy that works well for your organization? We'd love to hear about it! Share your approach in our [GitHub Discussions](https://github.com/cloudposse/atmos/discussions) or [open an issue](https://github.com/cloudposse/atmos/issues) to help us expand this documentation with real-world patterns from the community.
