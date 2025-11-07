# Flag Handling Documentation

**⚠️ NOTICE: This directory contains historical design documents that have been consolidated.**

**For the current, authoritative specification, see: [`../unified-flag-parsing-refactoring.md`](../unified-flag-parsing-refactoring.md)**

## Overview

This directory contains design documents for various aspects of Atmos's flag parsing system. These documents represent the evolution of thinking that led to the final unified approach.

**Current Status**: All these documents have been consolidated into a single comprehensive PRD: [`unified-flag-parsing-refactoring.md`](../unified-flag-parsing-refactoring.md)

## Historical Documents

The following documents provide detailed context on specific aspects of the design but should be read in conjunction with the master PRD:

### [unified-flag-parsing.md](unified-flag-parsing.md)
**Status**: Superseded by `unified-flag-parsing-refactoring.md`

Original comprehensive design for unified flag parsing. The concepts here have been integrated into the master PRD with additional details on compatibility flages and migration strategy.

### [global-flags-pattern.md](global-flags-pattern.md)
**Status**: Integrated into master PRD (Section: "Strongly-Typed Options Structs")

Documents the GlobalFlags embedding pattern that eliminates duplication of 13+ global flags across commands.

### [global-flags-examples.md](global-flags-examples.md)
**Status**: Integrated into master PRD (Section: "Strongly-Typed Options Structs")

Real-world examples of GlobalFlags usage with `--logs-level` and `--identity` flags.

### [default-values-pattern.md](default-values-pattern.md)
**Status**: Integrated into master PRD (Section: "Strongly-Typed Options Structs")

Four-layer default value system and precedence order.

### [strongly-typed-builder-pattern.md](strongly-typed-builder-pattern.md)
**Status**: Integrated into master PRD (Section: "Strongly-Typed Options Structs")

Builder pattern for strongly-typed options structs (TerraformOptions, etc.).

### [type-safe-positional-arguments.md](type-safe-positional-arguments.md)
**Status**: Integrated into master PRD (Section: "Positional Args Builders")

Type-safe extraction of positional arguments with builders.

### [command-registry-colocation.md](command-registry-colocation.md), [flagparser-integration.md](flagparser-integration.md)
**Status**: Reference documents for implementation details

## Quick Reference

For the complete, up-to-date specification covering:
- Compatibility alias translation
- Unified parser implementation
- Strongly-typed options structs
- Global flags embedding
- NoOptDefVal pattern
- Breaking changes & migration
- Blog post guidance
- Implementation status

**→ See [`../unified-flag-parsing-refactoring.md`](../unified-flag-parsing-refactoring.md)**

## Implementation Status

See the master PRD for current implementation status. As of this writing:

- ✅ Phase 1: Core Infrastructure (COMPLETE)
  - CompatibilityFlagsTranslator (51 tests)
  - UnifiedParser (25 tests)
  - TerraformOptions struct

- 🚧 Phase 2: Terraform Integration (IN PROGRESS)

- 📋 Phase 3: Packer & Helmfile (PLANNED)

- 🧹 Phase 4: Cleanup (PLANNED)
