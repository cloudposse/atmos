---
slug: introducing-command-registry-pattern
title: "Introducing the Command Registry Pattern: Toward Pluggable Commands"
authors: [atmos]
tags: [architecture, extensibility, plugins]
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

## Backward Compatibility

This is a **100% backward-compatible change**. Custom commands defined in `atmos.yaml` continue to work exactly as before. In fact, they can now:

1. **Replace built-in commands** - Override default behavior
2. **Extend built-in commands** - Add subcommands to registry commands
3. **Add new commands** - Create entirely new functionality

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

## Try It Today

The Command Registry Pattern is available in the latest Atmos release. The `about` command has been migrated as a proof-of-concept, demonstrating the pattern works in production.

Developers interested in migrating additional commands can reference:
- **[PRD: Command Registry Pattern](https://github.com/cloudposse/atmos/blob/main/docs/prd/command-registry-pattern.md)** - Complete architecture documentation
- **[Developer Guide: Developing Atmos Commands](https://github.com/cloudposse/atmos/blob/main/docs/developing-atmos-commands.md)** - Step-by-step implementation guide

## Get Involved

We're building this in the open and welcome community feedback:
- 💬 **Discuss** - Share your thoughts in [GitHub Discussions](https://github.com/cloudposse/atmos/discussions)
- 🐛 **Report Issues** - Found a bug? [Open an issue](https://github.com/cloudposse/atmos/issues)
- 🚀 **Contribute** - Help migrate commands in future PRs

This is just the beginning. We're excited to see what the community builds with pluggable commands!

---

**Want to learn more?** Check out the full [Command Registry Pattern PRD](https://github.com/cloudposse/atmos/blob/main/docs/prd/command-registry-pattern.md) for technical details.
