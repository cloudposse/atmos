# Example: Toolchain

Configure tool registries and use toolchain-managed tools in custom commands and workflows.

Learn more about [Toolchain Configuration](https://atmos.tools/cli/configuration/toolchain).

## What You'll See

- [Inline registry](https://atmos.tools/cli/configuration/toolchain/registries) defining custom tool downloads alongside the official Aqua registry as fallback
- [Aliases](https://atmos.tools/cli/configuration/toolchain/aliases) mapping short names (`jq`) to full specs (`jqlang/jq`)
- [Custom commands](https://atmos.tools/cli/configuration/commands) using four dependency patterns: implicit, pinned, constrained, and multi-tool
- [Workflows](https://atmos.tools/workflows) demonstrating the same four patterns

## Try It

```shell
cd examples/toolchain

# Install tools from .tool-versions
atmos toolchain install

# Run custom command demos
atmos demo which         # Implicit deps from .tool-versions
atmos demo pinned        # Exact version pinning
atmos demo constrained   # SemVer constraints
atmos demo convert       # Multi-tool pipeline

# Run workflow demos
atmos workflow which -f toolchain-demo
atmos workflow convert -f toolchain-demo
```

## Key Files

| File | Purpose |
|------|---------|
| `atmos.yaml` | Inline registry + Aqua fallback, aliases, custom commands |
| `.tool-versions` | Project tool defaults (jq 1.7.1, yq 4.45.1) |
| `workflows/toolchain-demo.yaml` | Workflow versions of the same 4 patterns |
