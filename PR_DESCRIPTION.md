# feat: Implement unified flag parsing system with strongly-typed Options pattern

## Summary

This PR implements a comprehensive unified flag parsing system that replaces ad-hoc flag access patterns across all Atmos commands with a strongly-typed Options pattern. This architecture improves type safety, precedence handling, testability, and maintainability.

## Motivation

The previous flag parsing approach had several issues:
- **Inconsistent precedence**: Commands used `cmd.Flags().GetString()` which bypassed Viper's CLI → ENV → config → defaults precedence
- **Type safety**: Direct flag map access with string keys was error-prone
- **Testability**: Hard to test flag parsing in isolation from command execution
- **Maintainability**: Flag handling logic duplicated across commands

## Changes

### Core Infrastructure

#### PassThroughFlagParser (pkg/flags/passthrough.go)
- Handles commands with `--` separator (terraform, helmfile, packer, custom commands)
- Separates Atmos flags from tool-specific pass-through arguments
- Respects Viper precedence for all flags
- Returns `ParsedConfig` with separated arguments

#### StandardParser (pkg/flags/standard_parser.go)
- Handles standard commands without pass-through arguments
- Builder pattern for composable flag registration
- Returns strongly-typed `*StandardOptions` struct
- Full Viper integration for precedence

### Strongly-Typed Options Patterns

Created specialized Options types for commands with unique flags:

- **TerraformOptions** - terraform-specific flags (stack, identity, dry-run + pass-through)
- **HelmfileOptions** - helmfile-specific flags (stack, identity, dry-run + pass-through)
- **PackerOptions** - packer-specific flags (stack, identity, dry-run + pass-through)
- **AuthOptions** - auth command flags (identity selector, login, provider, all, etc.)
- **AuthExecOptions** - auth exec flags (identity + pass-through for executable)
- **AuthShellOptions** - auth shell flags (identity, shell type + pass-through)
- **DescribeStacksOptions** - describe stacks specialized flags (sections, include-empty-stacks, etc.)
- **EditorConfigOptions** - editorconfig validation flags (check, exclude)
- **StandardOptions** - common flags shared across describe, list, validate commands

### Commands Migrated (30+)

**Auth commands** (8):
- `auth console`, `auth env`, `auth exec`, `auth list`, `auth logout`, `auth shell`, `auth validate`, `auth whoami`

**Describe commands** (5):
- `describe component`, `describe config`, `describe dependents`, `describe stacks`, `describe workflows`

**List commands** (5):
- `list components`, `list stacks`, `list values`, `list vendor`, `list workflows`

**Validate commands** (4):
- `validate component`, `validate editorconfig`, `validate schema`, `validate stacks`

**Infrastructure tools** (3):
- `terraform`, `helmfile`, `packer` (all subcommands)

**Other** (3):
- `vendor diff`, `vendor pull`, `workflow`

**Custom commands**:
- Dynamic custom commands defined in atmos.yaml now use unified parser

### Integration with Recent Main Changes

This PR includes a merge from `main` and integrates:
- Identity flag support for describe commands (#1742)
- AuthManager propagation through template functions
- UI output improvements for validation commands (#1741)

All describe commands now support `--identity` flag while maintaining the new strongly-typed pattern.

### Type-Safe Helper Methods

Added helper methods to avoid unsafe map access:
- `ParsedConfig.GetIdentity()` - Type-safe identity flag access
- `ParsedConfig.GetStack()` - Type-safe stack flag access
- `opts.GetPositionalArgs()` - Access positional arguments from any Options type

### Testing

- Added comprehensive test coverage for new parsers
- Updated command tests to use new Options patterns
- Added TestKit for proper RootCmd isolation in tests
- Fixed test compatibility after merge from main

### Documentation

- Added PRD documentation in `docs/prd/flag-parser/`
- Documented builder patterns and usage examples
- Architecture decision records for Options pattern approach

## Breaking Changes

None - this is an internal refactoring that maintains all existing CLI interfaces.

## Migration Notes

For future command additions:
1. Use `StandardOptionsBuilder` for standard commands
2. Create custom Options for specialized flags
3. Use `PassThroughFlagParser` for commands with `--` separator
4. Always bind flags to Viper for proper precedence

## Testing Instructions

```bash
# Build succeeds
go build .

# Run command tests
go test ./cmd -v

# Test flag precedence
export ATMOS_STACK=from-env
atmos describe stacks  # Should use from-env

atmos describe stacks --stack=from-cli  # Should use from-cli (CLI overrides ENV)
```

## Metrics

- **Files changed**: 303
- **Lines added**: 27,662
- **Lines removed**: 2,727
- **Net change**: +24,935 lines
- **Commands migrated**: 30+
- **Test coverage**: Maintained >80% for cmd package

## Related Issues

Closes #[issue-number] (if applicable)

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
