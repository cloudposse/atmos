---
slug: toolchain-management
title: "Native Toolchain Management with Aqua Registry Integration"
authors: [atmos]
tags: [feature, toolchain, aqua]
date: 2025-10-23
---

Atmos now includes native toolchain management that seamlessly integrates with the Aqua registry ecosystem—giving you access to hundreds of pre-configured CLI tools without the overhead of external tool managers.

<!--truncate-->

## What's New

Atmos now includes **built-in toolchain management** commands that let you install, manage, and version control CLI tools directly within your infrastructure projects. This feature integrates natively with the [Aqua registry](https://aquaproj.github.io/), leveraging its extensive ecosystem of package definitions while providing deep integration with Atmos workflows and components.

### Why This Matters

Managing tool versions across infrastructure teams has always been a challenge. Different developers use different versions of terraform, kubectl, helm, and dozens of other CLIs—leading to "works on my machine" problems and deployment inconsistencies.

Traditional solutions require:
- Installing separate tool managers (asdf, aqua, tfenv, etc.)
- Maintaining separate configuration files
- Context switching between your infrastructure tool and your tool manager
- No integration with your infrastructure automation workflows

**Now you can manage all your CLI tools directly from Atmos**—with zero external dependencies and seamless integration with your infrastructure workflows.

## How It Works

### Installation

Install tools with simple commands:

```bash
# Install specific versions using aliases
atmos toolchain install terraform@1.9.8
atmos toolchain install opentofu@1.10.3
atmos toolchain install kubectl@1.28.0

# Install using canonical registry paths
atmos toolchain install hashicorp/terraform@1.9.8
atmos toolchain install opentofu/opentofu@1.10.3

# Install all tools from .tool-versions file
atmos toolchain install
```

### Version Management

Manage tool versions with `.tool-versions` files (asdf-compatible):

```
terraform 1.9.8
opentofu 1.10.3
kubectl 1.28.0
helm 3.13.0
```

```bash
# Add a tool to .tool-versions
atmos toolchain add terraform@1.9.8

# Remove a tool from .tool-versions
atmos toolchain remove terraform

# Set default version when multiple are installed
atmos toolchain set terraform 1.9.8

# List installed tools with status
atmos toolchain list
```

### Execution

Run tools directly through Atmos:

```bash
# Execute a specific version
atmos toolchain exec terraform@1.9.8 -- plan

# Use the version from .tool-versions
atmos toolchain exec terraform -- plan

# Get tool paths for integration
atmos toolchain path
atmos toolchain which terraform
```

## Aqua Registry Integration

The power of Atmos toolchain comes from its integration with the [Aqua registry](https://github.com/aquaproj/aqua-registry)—a community-maintained collection of over 1,000 CLI tool definitions.

### Benefits of Aqua Registry

- **Extensive Coverage**: Pre-configured definitions for terraform, kubectl, helm, aws-cli, and hundreds more
- **Community Maintained**: Regular updates and new tools added by the community
- **Proven Format**: Battle-tested YAML format used by thousands of teams
- **No Vendor Lock-in**: Compatible with asdf `.tool-versions` files

### Why We Reimplemented the Registry Parser

While we leverage the Aqua registry ecosystem, we implemented our own parser rather than depending on Aqua's Go modules:

**Aqua's SDK Isn't Stable**
The Aqua maintainers have explicitly stated their Go modules are for internal use only and not stable for external dependencies. This makes any integration fragile and subject to breaking changes without notice.

**Native Atmos Integration**
By implementing our own parser, we provide seamless integration with:
- Atmos configuration and workflows
- Component-level tool dependencies
- Stack-aware tool management
- Performance optimizations for infrastructure automation

**Focused Feature Set**
We support the core Aqua registry features needed for infrastructure tooling:
- GitHub releases and HTTP downloads
- Template interpolation for asset URLs
- Archive extraction (.tar.gz, .zip, .gz)
- Version resolution and constraints
- Local tool aliases

### What This Means for You

You get the best of both worlds:
- **Access to Aqua's ecosystem** - Hundreds of pre-configured tools with reliable metadata
- **Native Atmos integration** - Deep integration with your infrastructure workflows
- **Zero external dependencies** - No need to install aqua, asdf, or other tool managers
- **Full control** - We evolve the feature based on infrastructure automation needs

## Configuration

### Tool Aliases

Define simple aliases in `tools.yaml`:

```yaml
aliases:
  terraform: hashicorp/terraform
  opentofu: opentofu/opentofu
  kubectl: kubernetes-sigs/kubectl
  helm: helm/helm
  tflint: terraform-linters/tflint
```

Then use simple names in your `.tool-versions` file:

```
terraform 1.9.8
kubectl 1.28.0
helm 3.13.0
```

### GitHub Token Support

For higher rate limits and private repositories:

```bash
# Set token via environment variable
export ATMOS_GITHUB_TOKEN=ghp_your_token_here

# Or use GITHUB_TOKEN
export GITHUB_TOKEN=ghp_your_token_here
```

### Global Configuration

Configure toolchain behavior in `atmos.yaml`:

```yaml
toolchain:
  file_path: ".tool-versions"      # Tool versions file location
  tools_dir: ".tools"               # Where to install tools
  tools_config_file: "tools.yaml"  # Tool aliases and configuration
```

## Complete Command Reference

```bash
# Installation
atmos toolchain install [tool@version]     # Install specific version
atmos toolchain install                     # Install all from .tool-versions

# Version Management
atmos toolchain add tool@version           # Add to .tool-versions
atmos toolchain remove tool                # Remove from .tool-versions
atmos toolchain set tool version           # Set default version
atmos toolchain list                       # List installed tools
atmos toolchain versions                   # Show versions from file

# Execution
atmos toolchain exec tool -- args          # Execute tool
atmos toolchain path                       # Print PATH entries
atmos toolchain which tool                 # Show tool binary path

# Information
atmos toolchain info tool                  # Show tool configuration
atmos toolchain aliases                    # List configured aliases

# Cleanup
atmos toolchain uninstall tool@version     # Uninstall specific version
atmos toolchain uninstall                  # Uninstall all from .tool-versions
atmos toolchain clean                      # Remove all tools and cache
```

## Future Enhancements

We're planning additional features based on community feedback:
- Component-level tool dependency declarations
- Stack-aware tool version resolution
- Workflow integration for automatic tool installation
- Support for additional Aqua registry features
- Custom registry support for private tools

## Get Involved

The toolchain feature is actively developed and we welcome feedback:
- **Try it out** - Start using `atmos toolchain` commands in your projects
- **Report issues** - Let us know if you encounter any problems
- **Request tools** - If a tool isn't working, open an issue
- **Share feedback** - Tell us what features would be most valuable

Explore the [Aqua registry](https://github.com/aquaproj/aqua-registry) to see all available tools, and check out the [toolchain documentation](/cli/commands/toolchain/usage) for detailed usage examples.
