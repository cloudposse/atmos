# Strongly-Typed Interpreters

This directory contains documentation for the strongly-typed command interpreter system in Atmos.

## Overview

The strongly-typed interpreter pattern provides type-safe access to command flags, replacing weakly-typed map-based access with strongly-typed structs.

## Documents

### [global-flags-pattern.md](global-flags-pattern.md)
**Architecture and Design Pattern**

Details the core architecture using Go struct embedding to provide DRY (Don't Repeat Yourself) global flags handling. Shows how all 13+ persistent flags (logs-level, no-color, identity, pager, profiler-*, heatmap-*, etc.) are defined once and embedded in all command interpreters.

**Key Topics:**
- GlobalFlags struct design
- IdentitySelector and PagerSelector types
- CommandInterpreter interface
- Struct embedding pattern
- Benefits and alternatives

### [global-flags-examples.md](global-flags-examples.md)
**Real-World Examples**

Demonstrates the pattern with concrete examples using two real flags:
- `--logs-level` (simple global flag)
- `--identity` (complex flag with NoOptDefVal semantics)

**Key Topics:**
- Before/after code comparisons
- Manual precedence vs. automatic
- IdentitySelector usage scenarios
- Testing examples
- Code reduction metrics

### [default-values-pattern.md](default-values-pattern.md)
**Default Value System**

Explains the four-layer default value system and how defaults flow through:
1. Cobra flag registration
2. FlagRegistry storage
3. Viper precedence resolution
4. GlobalFlags struct

**Key Topics:**
- Default value types (empty string, non-empty, bool, int, special types)
- Precedence order: CLI > ENV > config > default > zero value
- GlobalFlagsRegistry() with pre-configured defaults
- Testing default values

## Usage

See the main PRD at [`docs/prd/unified-flag-parsing.md`](../unified-flag-parsing.md) for the complete specification and implementation plan.

## Quick Example

```go
// Define strongly-typed interpreter
type TerraformInterpreter struct {
    GlobalFlags  // Embedded - provides all 13+ global flags

    // Command-specific flags
    Stack   string
    DryRun  bool
}

// Use with type safety
interpreter, _ := parser.Parse(ctx, args)

// ✅ Strongly typed access
log.SetLevel(interpreter.LogsLevel)    // From GlobalFlags
if interpreter.NoColor {                // From GlobalFlags
    color.Disable()
}

info.Stack = interpreter.Stack          // From TerraformInterpreter
info.DryRun = interpreter.DryRun        // From TerraformInterpreter

// ✅ Clear identity handling
if interpreter.Identity.IsInteractiveSelector() {
    selectIdentityInteractively()
} else if !interpreter.Identity.IsEmpty() {
    useIdentity(interpreter.Identity.Value())
}
```

## Implementation Status

- ✅ Infrastructure complete (100% test coverage)
- ✅ Documentation complete
- 🚧 Adoption in progress (Terraform, Helmfile, Packer, etc.)
