---
slug: introducing-command-registry-pattern
title: 'Introducing the Command Registry Pattern: Toward Pluggable Commands'
sidebar_label: Command Registry Pattern
authors:
  - osterman
tags:
  - core
date: 2025-10-16T00:00:00.000Z
release: v1.195.0
---

We're excited to announce the first step in a major architectural evolution for Atmos: the **Command Registry Pattern**. This foundational change will eventually enable **pluggable commands**, allowing the community to extend Atmos with custom command packages without modifying the core codebase.

<!-- truncate -->

## Why This Matters

Today, all Atmos commands live in a single monolithic `cmd/` directory. While this works well for built-in commands, it creates friction for:

- **Plugin developers** who want to add new commands without forking Atmos
- **Command maintainers** who need clear boundaries between command implementations
- **Organizations** that want to distribute custom command packages internally

The Command Registry Pattern solves these challenges by treating commands as **self-contained packages** that register themselves with Atmos at startup.

## What's Changing

### Before: Monolithic Command Structure

```
cmd/
├── terraform.go        # All commands in one directory
├── describe.go
├── list.go
└── about.go
```

### After: Package-Per-Command Architecture

```
cmd/
├── terraform/          # Each command is a package
│   └── terraform.go
├── describe/
│   └── describe.go
├── about/              # First migrated command
│   └── about.go
└── internal/           # Registry infrastructure
    ├── command.go      # CommandProvider interface
    └── registry.go     # Thread-safe registry
```

## How It Works

Commands implement a simple interface and register themselves during package initialization:

```go
// cmd/about/about.go
package about

import (
    "github.com/spf13/cobra"
    "github.com/cloudposse/atmos/cmd/internal"
)

func init() {
    // Self-registration via init()
    internal.Register(&AboutCommandProvider{})
}

type AboutCommandProvider struct{}

func (a *AboutCommandProvider) GetCommand() *cobra.Command {
    return aboutCmd
}

func (a *AboutCommandProvider) GetName() string {
    return "about"
}

func (a *AboutCommandProvider) GetGroup() string {
    return "Other Commands"
}
```

The registry pattern provides:
- ✅ **Self-registering commands** - No manual wiring required
- ✅ **Type-safe interfaces** - Compile-time guarantees
- ✅ **Thread-safe operation** - Concurrent registration support
- ✅ **Custom command compatibility** - Works seamlessly with existing `atmos.yaml` custom commands

## Impact on Users

This is a **100% backward-compatible change** with zero impact on Atmos users. All existing functionality remains identical:

- Custom commands in `atmos.yaml` work exactly as before
- Command behavior is unchanged
- No configuration updates required
- All existing workflows continue to work

For context: The registry pattern actually enhances custom command capabilities by allowing them to extend built-in commands with subcommands, but this is an existing feature that continues to work—nothing new from a user perspective.

## The Road Ahead

This PR lays the **foundation** for pluggable commands. Here's what's coming next:

### Phase 2: Migrate Core Commands
Subsequent PRs will refactor existing commands into the new package structure:
- `atmos terraform` → `cmd/terraform/`
- `atmos describe` → `cmd/describe/`
- `atmos list` → `cmd/list/`
- `atmos validate` → `cmd/validate/`

Each command family will move into its own **package** (Go's term for a self-contained code module).

### Phase 3: External Plugin Support
Once all commands use the registry pattern, we'll enable:
- **Plugin discovery** - Load commands from external Go modules
- **Plugin packaging** - Distribute commands as standalone binaries
- **Plugin marketplace** - Share and discover community commands

## For Atmos Contributors

This change is **internal to Atmos development** and has no impact on Atmos users. End users won't notice any difference in behavior—this is purely an architectural improvement for maintainability and future extensibility.

**If you're an Atmos contributor** interested in migrating commands or building plugins:
- **[PRD: Command Registry Pattern](https://github.com/cloudposse/atmos/blob/main/docs/prd/command-registry-pattern.md)** - Complete architecture documentation
- **[Developer Guide: Developing Atmos Commands](https://github.com/cloudposse/atmos/blob/main/docs/developing-atmos-commands.md)** - Step-by-step implementation guide

The `about` command has been migrated as a proof-of-concept in this PR, demonstrating the pattern works in production.

## Get Involved

We're building this in the open and welcome contributions from the community:
- 💬 **Discuss** - Share your thoughts in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- 🐛 **Report Issues** - Found a bug? [Open an issue](https://github.com/cloudposse/atmos/issues)
- 🚀 **Contribute** - Help migrate commands in future PRs

This is the foundation for a more modular, extensible Atmos architecture.

---

**Want to learn more?** Check out the full [Command Registry Pattern PRD](https://github.com/cloudposse/atmos/blob/main/docs/prd/command-registry-pattern.md) for technical details.
