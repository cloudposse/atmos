---
slug: native-devcontainer-support
title: "Native Dev Container Support: Solving \"Works on My Machine\" Once and For All"
authors: [atmos]
tags: [feature, atmos, devcontainers, geodesic, developer-experience, docker, podman]
date: 2025-10-22
---

Running Atmos and managing cloud infrastructure inevitably means depending on dozens of tools—Terraform, kubectl, Helmfile, AWS CLI, and many more. But here's the problem every platform team faces: **"It works on my machine."**

Different versions. Missing dependencies. Subtle configuration differences. Onboarding a new team member becomes a day-long exercise in installing and configuring tools. Something that worked perfectly on your laptop fails in CI. You spend more time managing your toolchain than actually using it.

Today, we're solving this problem once and for all with **native Development Container support in Atmos**.

<!--truncate-->

## The DevOps Toolbox Pattern

The concept of containerized development environments—what we call "DevOps toolboxes"—isn't new. Companies like CoreOS pioneered the toolbox pattern years ago, recognizing that developers need consistent, reproducible environments without installing dozens of tools locally.

This pattern has been proven in DevOps long before the Development Containers specification existed. The idea is simple but powerful: **package all your tools into a container, and developers just need Docker and a shell**.

### Development Containers: The Modern Standard

The software development world caught on, and the [Development Containers specification](https://containers.dev/) emerged as an industry standard. Today, every major IDE supports devcontainers:

- **VS Code** with the Dev Containers extension
- **JetBrains IDEs** (IntelliJ IDEA, PyCharm, WebStorm, etc.)
- **Cloud development environments**: GitHub Codespaces, Gitpod, DevPod, Coder, CodeSandbox

The specification provides a simple, declarative JSON format (`devcontainer.json`) that describes your development environment. It's become the lingua franca for reproducible dev environments, especially popular in web development and software engineering.

**But here's the irony**: While devcontainers are incredibly useful for DevOps workflows, they're primarily supported by IDEs. To use them from the command line—where DevOps teams actually work—you need to install yet another tool (the official devcontainer CLI).

### Why Native Support in Atmos?

We asked ourselves: if devcontainers are just JSON configuration describing which container to run, why not support them natively in Atmos?

After all, launching a container, mounting volumes, forwarding ports, and executing commands is straightforward. And Atmos can bring superpowers that go beyond basic devcontainer support.

**The result? Install Atmos, and you're done.** No Docker Desktop devcontainer CLI, no separate tools. From that point on, everything can happen from a Docker image that gets pulled automatically.

## Introducing `atmos devcontainer shell`

The heart of Atmos devcontainer support is one command: **`atmos devcontainer shell`**

```bash
# Launch an interactive shell in your devcontainer
atmos devcontainer shell geodesic

# That's it. You're in a fully-equipped DevOps environment.
```

Inside the container, you have everything:
- Terraform with all major providers
- kubectl and Kubernetes tools
- Helmfile and Helm
- AWS, Azure, and GCP CLIs
- Atmos itself
- All your workspace files mounted and ready

No installation. No version conflicts. No "works on my machine." Just a consistent, containerized environment that works everywhere.

### Interactive Selection

Don't remember the devcontainer name? No problem. Atmos prompts you interactively:

```bash
$ atmos devcontainer shell

? Select a devcontainer:
❯ geodesic
  terraform
  python-dev
```

Just like `atmos auth login`, Atmos makes the experience smooth and intuitive.

### Shell Autocomplete

Tab completion works for all devcontainer names:

```bash
atmos devcontainer shell geo<TAB>
# Autocompletes to: atmos devcontainer shell geodesic
```

### Multiple Instances

Need multiple environments? Launch the same devcontainer configuration with different instance names:

```bash
# Development instance
atmos devcontainer shell geodesic --instance dev

# Production instance
atmos devcontainer shell geodesic --instance prod

# Each team member can have their own
atmos devcontainer shell geodesic --instance alice
atmos devcontainer shell geodesic --instance bob
```

Each instance is an independent container with its own state, perfect for running multiple environments or isolating work.

## Configuration in atmos.yaml

Define devcontainers alongside your stack and component configuration:

```yaml
# atmos.yaml
components:
  devcontainer:
    geodesic:
      spec:
        name: "Geodesic DevOps Toolbox"
        image: "cloudposse/geodesic:latest"
        workspaceFolder: "/localhost"
        workspaceMount: "type=bind,source=${PWD},target=/localhost"
        forwardPorts:
          - 8080
        containerEnv:
          ATMOS_BASE_PATH: "/localhost"
        remoteUser: "root"

    terraform:
      spec:
        name: "Terraform Development"
        image: "hashicorp/terraform:1.10"
        workspaceFolder: "/workspace"
        forwardPorts:
          - 3000
        mounts:
          - "type=bind,source=${HOME}/.aws,target=/root/.aws,readonly"
```

### Use Existing devcontainer.json Files

Already have `.devcontainer/devcontainer.json` files? Atmos can use them directly with the `!include` function:

```yaml
# atmos.yaml
components:
  devcontainer:
    geodesic:
      spec: !include .devcontainer/devcontainer.json
```

Or include and override specific fields:

```yaml
# atmos.yaml
components:
  devcontainer:
    geodesic:
      spec:
        - !include .devcontainer/devcontainer.json
        - containerEnv:
            ATMOS_BASE_PATH: "/localhost"
            CUSTOM_VAR: "value"
```

This is the **real Atmos `!include` function**—the same powerful YAML processing you use everywhere else in Atmos. It supports deep merging, overrides, and all the template functions you know.

## Geodesic: A Production-Ready Devcontainer

While Atmos supports any devcontainer configuration, **Geodesic is a proven DevOps toolbox** that's been battle-tested for almost 10 years.

[Geodesic](https://github.com/cloudposse/geodesic) is Cloud Posse's implementation of the DevOps toolbox pattern—a comprehensive development container that includes:

- **Atmos** (of course!)
- **Terraform** with all major providers
- **kubectl** and Kubernetes tools (helm, helmfile, k9s, etc.)
- **Cloud CLIs**: AWS CLI, Azure CLI, Google Cloud SDK
- **Data processing**: jq, yq, gomplate
- **Development essentials**: git, make, vim, and more
- **Custom scripts and tooling**

Geodesic images are:
- **Multi-platform**: linux/amd64 and linux/arm64
- **Debian-based**: Familiar package management
- **Customizable**: Use as a base image for your own toolbox
- **Production-tested**: Nearly a decade of real-world usage
- **Open source**: Over 1,000 stars on GitHub

**Geodesic is a devcontainer implementation.** It predates the Development Containers spec, but it's the exact same concept: a containerized environment with all your tools pre-installed and pre-configured.

## Getting Started in 2 Minutes

Here's how fast you can go from zero to productive:

```bash
# 1. Install Atmos (one binary)
brew install atmos

# 2. Navigate to your infrastructure repo
cd my-infrastructure

# 3. Launch your devcontainer
atmos devcontainer shell geodesic

# You're in. Start working.
$ atmos terraform plan vpc -s prod
$ kubectl get pods
$ helm list
```

**That's the ingenious part**: All you need to install is Atmos. Everything else—Terraform, cloud CLIs, Kubernetes tools—gets pulled from the container image automatically.

Your host machine stays clean. Your environment stays consistent. Your team uses identical tool versions.

### Quick Start with Examples

Check out the live examples in the Atmos repository to get started immediately:

```bash
# Clone Atmos repo (or just browse examples on GitHub)
git clone https://github.com/cloudposse/atmos.git
cd atmos/examples/devcontainer

# The example includes a complete configuration
cat atmos.yaml
# Shows geodesic devcontainer with !include usage

# Launch it
atmos devcontainer shell geodesic
```

The `examples/devcontainer` folder contains:
- Complete `atmos.yaml` with devcontainer configuration
- Example `devcontainer.json` file
- Shell aliases for convenience
- Ready-to-use Geodesic setup

**Use this as a starting point** for your own configuration. Copy it, customize it, make it yours.

## Shell Aliases for One-Word Access

Make it even easier with shell aliases in your `atmos.yaml`:

```yaml
# atmos.yaml
cli:
  aliases:
    shell: "devcontainer shell geodesic"
```

Now you can just type:

```bash
atmos shell
# Immediately launches your devcontainer
```

This mirrors the classic Geodesic pattern where you'd type `./geodesic.sh` to launch your environment. Now it's even simpler: `atmos shell`.

## Additional Lifecycle Commands

While `shell` is the primary command you'll use, Atmos provides full lifecycle management for advanced scenarios:

```bash
# Start a container (create if needed, then start and attach)
atmos devcontainer start geodesic --attach

# Attach to an already-running container
atmos devcontainer attach geodesic

# Stop without removing
atmos devcontainer stop geodesic

# View logs
atmos devcontainer logs geodesic

# Remove container
atmos devcontainer remove geodesic

# Rebuild image and recreate
atmos devcontainer rebuild geodesic
```

These commands give you fine-grained control when you need it, but **`shell` is what you need 99% of the time**.

## Atmos Superpowers

Beyond standard devcontainer support, Atmos brings unique capabilities:

### 1. Zero Additional Dependencies
Install Atmos, and you're done. No devcontainer CLI, no separate tools to manage.

### 2. Named Containers with Multiple Instances
Unlike traditional devcontainer tools, Atmos supports named devcontainer configurations and multiple instances per configuration.

### 3. Interactive Selection and Autocomplete
Atmos prompts you to select from available devcontainers and provides full tab completion.

### 4. Rich Terminal UI
Built with the [Charm ecosystem](https://charm.sh/), Atmos provides beautiful progress indicators and status messages while keeping structured output pipeline-friendly.

### 5. Docker and Podman Support
Works with both Docker and Podman, with automatic runtime detection. No vendor lock-in.

```yaml
# Per-devcontainer runtime selection
components:
  devcontainer:
    geodesic:
      settings:
        runtime: docker  # or podman, or omit for auto-detect
```

### 6. Identity Injection
Atmos supports injecting authenticated identities directly into devcontainers with the `--identity` flag:

```bash
# Launch with AWS identity
atmos devcontainer shell geodesic --identity aws-prod

# Launch with GitHub identity
atmos devcontainer shell geodesic --identity github-main

# Works with ANY provider - Azure, GCP, custom providers
atmos devcontainer shell geodesic --identity azure-prod
```

Inside the container, cloud provider SDKs automatically use the authenticated identity. The implementation is provider-agnostic - each provider's credentials are injected via environment variables without devcontainer code knowing provider-specific details.

### 7. XDG Base Directory Support
Atmos automatically configures XDG Base Directory environment variables inside containers, ensuring Atmos and other tools use the correct paths for config, cache, and data files.

### 8. Run Atmos from Atmos
The inception pattern—run Atmos inside a devcontainer that already has Atmos installed. Your host machine only needs the Atmos binary; everything else lives in the container.

## Use Cases for Development Containers

Development containers are incredibly valuable across different domains:

### Software Development & Web Development
- Consistent Node.js, Python, Ruby, or Go environments
- Database tools and clients pre-installed
- IDE integration for seamless development

### DevOps & Infrastructure
- **This is where the pattern originated** with toolboxes like CoreOS Toolbox
- Consistent Terraform, kubectl, and cloud CLI versions
- No conflicts between different project requirements
- Onboarding new team members in minutes instead of hours

### Data Engineering
- Jupyter notebooks with pre-installed libraries
- Data processing tools (Spark, Airflow, etc.)
- Database clients and connectors

Development containers are **equally valuable—if not more valuable—for DevOps** than traditional software development. Infrastructure teams juggle more tools, more versions, and more environmental complexity than most application developers.

## Comparison with Traditional Approaches

### Before: Manual Environment Setup
```bash
# Install Terraform
brew install terraform
# Wait, wrong version for this project...
tfenv install 1.10.0
tfenv use 1.10.0

# Install AWS CLI
pip install awscli
# Conflicts with other Python packages...

# Install kubectl
brew install kubectl
# Different version than CI uses...

# Install Helmfile
brew install helmfile

# Repeat for every tool...
# Repeat for every team member...
# Repeat when versions change...
# Repeat when you switch projects...
```

### After: Atmos Devcontainer
```bash
atmos devcontainer shell geodesic
# Everything installed, versioned, ready to use
```

### Comparison with Official devcontainer CLI

| Feature | Official CLI | Atmos Native |
|---------|-------------|--------------|
| Installation | Separate tool | Built-in |
| Primary command | `devcontainer up` | `atmos devcontainer shell` |
| Multiple instances | No | Yes |
| Interactive selection | No | Yes |
| Shell autocomplete | Limited | Full support |
| Named configs | File-based only | Named in atmos.yaml |
| Identity injection | Manual | Coming soon |
| Rich TUI | Basic | Charm ecosystem |
| Runtime choice | Docker only | Docker or Podman |
| `!include` support | No | Yes (native Atmos) |
| Atmos integration | External | Native |

## Practical Subset of the Spec

Atmos implements a **practical subset** of the [Development Containers specification](https://containers.dev/implementors/spec/), focusing on the features that matter most for DevOps workflows:

### ✅ Supported
- Container image and Dockerfile builds
- Volume mounts and workspace configuration
- Port forwarding (critical for development)
- Environment variables
- Container runtime arguments
- Build arguments
- Remote user configuration

### ❌ Intentionally Unsupported
- `features` - Use Dockerfile instead for explicit dependencies
- Lifecycle scripts (`postCreateCommand`, etc.) - Use Dockerfile `ENTRYPOINT`/`CMD`
- Editor customizations - Use official IDE extensions
- Host requirements - Keep it simple

This approach keeps the implementation lean, maintainable, and focused on solving the actual problem: **reproducible development environments for infrastructure teams**.

## Real-World Workflows

### Onboarding a New Team Member

**Old way:**
1. Install Homebrew
2. Install Docker
3. Install Terraform (with tfenv or version manager)
4. Install kubectl
5. Install AWS CLI
6. Configure AWS credentials
7. Install Helm
8. Install Helmfile
9. Install jq, yq, and other tools
10. Debug version conflicts
11. Maybe productive by end of day?

**New way:**
```bash
brew install atmos
cd team-infrastructure
atmos devcontainer shell
# Productive in 2 minutes
```

### Working on Multiple Projects

**Old way:**
- Project A uses one set of tool versions
- Project B uses different tool versions
- Use version managers (tfenv, etc.) to switch constantly
- Hope you remember to switch before running commands

**New way:**
```yaml
# project-a/atmos.yaml
components:
  devcontainer:
    toolbox:
      spec:
        image: "cloudposse/geodesic:4.3.0"  # Pinned toolbox version

# project-b/atmos.yaml
components:
  devcontainer:
    toolbox:
      spec:
        image: "cloudposse/geodesic:4.4.0"  # Different toolbox version
```

Each project gets the right tool versions automatically.

## What's Next?

This is just the beginning. We're planning:

- **Pre-built environments**: Official Atmos devcontainer images for common workflows
- **Enhanced IDE integration**: Better detection and coordination with VS Code and JetBrains
- **Template library**: Common devcontainer configurations for different infrastructure patterns
- **Identity autocomplete**: Tab completion for available identities

But the foundation is solid, battle-tested, and ready for production use today.

## Get Started Now

### 1. Upgrade Atmos

```bash
brew upgrade atmos
# or download from GitHub releases
```

### 2. Check Out the Examples

```bash
# Browse or clone the examples
https://github.com/cloudposse/atmos/tree/main/examples/devcontainer

# Or try it locally
git clone https://github.com/cloudposse/atmos.git
cd atmos/examples/devcontainer
atmos devcontainer shell geodesic
```

### 3. Add to Your Project

```yaml
# atmos.yaml
components:
  devcontainer:
    geodesic:
      spec:
        image: "cloudposse/geodesic:latest"
        workspaceFolder: "/localhost"
        workspaceMount: "type=bind,source=${PWD},target=/localhost"

cli:
  aliases:
    shell: "devcontainer shell geodesic"
```

### 4. Launch Your Environment

```bash
atmos shell
# Or: atmos devcontainer shell geodesic
```

## Conclusion

The DevOps toolbox pattern has been proven for years. Development containers brought the pattern into the modern age with an industry-standard specification. Now, **Atmos brings native devcontainer support with DevOps superpowers**.

The result? **Install Atmos, run one command, and everything just works.**

No more "works on my machine." No more installation marathons. No more version conflicts. Just consistent, reproducible, containerized development environments that work everywhere—on your laptop, in CI, on your team member's machine.

**Use Geodesic to get started quickly**, or create your own devcontainer, or use Geodesic as a base to build on. Check out the `examples/devcontainer` folder for a live example you can use immediately.

It's a pretty ingenious system, if we do say so ourselves.

## Resources

- [Devcontainer Command Documentation](/cli/commands/devcontainer)
- [Geodesic GitHub Repository](https://github.com/cloudposse/geodesic)
- [Development Containers Specification](https://containers.dev/)
- [Atmos Examples - Devcontainer](https://github.com/cloudposse/atmos/tree/main/examples/devcontainer)
- [Atmos GitHub Repository](https://github.com/cloudposse/atmos)

---

*Have feedback or questions? Join our [Slack community](https://slack.cloudposse.com/) or [open an issue on GitHub](https://github.com/cloudposse/atmos/issues).*
