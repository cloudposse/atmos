# PRD: Unified Parser with Compatibility Aliases

**⚠️ NOTICE: This document has been superseded by [`unified-flag-parsing-refactoring.md`](unified-flag-parsing-refactoring.md)**

This was an initial design document that has been consolidated into the comprehensive master PRD. See the master PRD for the complete, up-to-date specification.

---

## Status
**Superseded** - See `unified-flag-parsing-refactoring.md` for current specification

## Problem Statement

Current flag parsing architecture has critical issues:

1. **`DisableFlagParsing = true`** - Bypasses Cobra's validation and parsing
2. **Manual flag parsing** - Custom `parseFlag()` reinvents pflag/Cobra functionality
3. **Global flags duplication** - Every parser (Terraform, Packer, Helmfile) manually extracts global flags
4. **Two separate parsers** - StandardParser vs PassThroughFlagParser with duplicated logic
5. **Legacy terraform flags** - Single-dash multi-char flags (`-var`, `-var-file`) conflict with Cobra's POSIX parsing

## Root Cause

Terraform uses single-dash multi-character flags (e.g., `-var`, `-var-file`, `-destroy`) which conflict with Cobra's POSIX-style flag parsing where `-abc` means three separate short flags `-a -b -c`.

To avoid this conflict, we disabled Cobra's parser entirely (`DisableFlagParsing = true`) and implemented custom parsing, which led to all the other problems.

## Solution: Unified Parser with Compatibility Aliases

### Core Strategy

1. **One unified parser** - Handles both standard commands and commands with separated args
2. **Compatibility alias translation** - Pre-process args to normalize legacy syntax
3. **Enable Cobra validation** - Remove `DisableFlagParsing`, let Cobra do its job
4. **Categorize flags properly**:
   - **Atmos-managed flags** - Registered with Cobra, validated, documented
   - **Pass-through compatibility aliases** - Moved to separated args automatically

### Compatibility Alias Translation

**Compatibility aliases** are arg rewriting rules applied before Cobra parses:

```
Input:   -s dev -var foo=bar -var-file prod.tfvars -- -target=aws_instance.app
         ↓
Translate: -s → --stack (Atmos flag)
           -var → --var (Atmos-managed terraform flag)
           -var-file → append to separated args (pass-through)
         ↓
After:   --stack dev --var foo=bar -- -var-file prod.tfvars -target=aws_instance.app
         ↓
Cobra:   Parses --stack, --var normally
         ↓
Result:  AtmosFlags: {stack: "dev", var: ["foo=bar"]}
         SeparatedArgs: ["-var-file", "prod.tfvars", "-target=aws_instance.app"]
```

## Architecture

### Unified Parser

```go
// pkg/flags/parser.go

type Parser struct {
    registry              *FlagRegistry
    cmd                   *cobra.Command
    viper                 *viper.Viper
    compatibilityTranslator *CompatibilityAliasTranslator

    // Configuration
    supportsSeparatedArgs bool   // Whether this command supports -- separator
    positionalArgsCount   int    // Number of positional args to extract
}

// Parse processes command-line arguments.
func (p *Parser) Parse(ctx context.Context, args []string) (*ParsedConfig, error) {
    // STEP 1: Split at -- separator (if supported)
    beforeDash, afterDash := p.splitAtDoubleDash(args)

    // STEP 2: Apply compatibility alias translation
    // Separates: Atmos args vs args that should go to separated
    atmosArgs, compatibilitySeparatedArgs := p.compatibilityTranslator.Translate(beforeDash)

    // STEP 3: Let Cobra parse Atmos args
    // DisableFlagParsing = FALSE - Cobra does its job!
    flags, positionalArgs, err := p.parseWithCobra(atmosArgs)

    // STEP 4: Combine separated args
    separatedArgs := append(compatibilitySeparatedArgs, afterDash...)

    return &ParsedConfig{
        Flags:          flags,
        PositionalArgs: positionalArgs,
        SeparatedArgs:  separatedArgs,
    }, nil
}
```

### Compatibility Alias Translator

```go
// pkg/flags/compatibility_translator.go

type CompatibilityAliasTranslator struct {
    // Maps compatibility alias → behavior
    aliasMap map[string]CompatibilityAlias
}

type CompatibilityAlias struct {
    Behavior CompatibilityBehavior
    Target   string  // Target flag name (for MapToAtmosFlag) or empty
}

type CompatibilityBehavior int

const (
    MapToAtmosFlag    CompatibilityBehavior = iota  // -s → --stack
    MoveToSeparated                                  // -var-file → separated args
)

// Translate processes args and separates Atmos args from separated args.
func (t *CompatibilityAliasTranslator) Translate(args []string) (atmosArgs []string, separatedArgs []string) {
    // For each arg:
    // 1. Check if it's a compatibility alias
    // 2. If MapToAtmosFlag: convert and add to atmosArgs
    // 3. If MoveToSeparated: add to separatedArgs
    // 4. Otherwise: add to atmosArgs (let Cobra handle it)
}
```

## Terraform Flag Categorization

### Category 1: Atmos-Managed Terraform Flags

Flags that Atmos processes/uses (register as Cobra flags):

| Flag | Compatibility Alias | Why Atmos Manages |
|------|-------------------|-------------------|
| `--var` | `-var` | Atmos parses with `getCliVars()` and merges into component vars |
| `--out` | `-out` | Atmos checks for this and auto-adds planfile if missing |
| `--auto-approve` | `-auto-approve` | Atmos checks for this and auto-adds if `apply_auto_approve: true` |

**Implementation:**
```go
func WithTerraformVar() Option {
    return func(cfg *parserConfig) {
        cfg.registry.Register(&StringSliceFlag{
            Name:                 "var",
            Shorthand:            "",
            Default:              []string{},
            Description:          "Set a variable in the Terraform configuration",
            CompatibilityAliases: []string{"-var"},
        })
    }
}
```

### Category 2: Pass-Through Compatibility Aliases

Flags that Atmos does NOT process (move to separated args):

| Compatibility Alias | Behavior |
|-------------------|----------|
| `-var-file` | Move to separated args |
| `-target` | Move to separated args |
| `-replace` | Move to separated args |
| `-destroy` | Move to separated args |
| `-refresh-only` | Move to separated args |
| `-refresh` | Move to separated args |
| `-lock` | Move to separated args |
| `-lock-timeout` | Move to separated args |
| `-parallelism` | Move to separated args |
| `-input` | Move to separated args |
| `-compact-warnings` | Move to separated args |
| `-detailed-exitcode` | Move to separated args |
| `-generate-config-out` | Move to separated args |
| `-state` | Move to separated args |

**Implementation:**
```go
func WithTerraformPassThroughCompatibility() Option {
    return func(cfg *parserConfig) {
        cfg.passThroughAliases = append(cfg.passThroughAliases,
            PassThroughAlias{Source: "-var-file", MoveToSeparated: true},
            PassThroughAlias{Source: "-target", MoveToSeparated: true},
            // ... all pass-through flags
        )
    }
}
```

## Implementation Plan

### Phase 1: Core Infrastructure (Test-Driven)

**Goal:** Build unified parser with compatibility alias support, fully tested.

#### Step 1.1: Compatibility Alias Translator Tests

Create comprehensive test matrix covering all edge cases:

```go
// pkg/flags/compatibility_translator_test.go

func TestCompatibilityAliasTranslator(t *testing.T) {
    tests := []struct {
        name          string
        input         []string
        expected      translationResult
        description   string
    }{
        // Category: Atmos shorthand flags
        {
            name:  "atmos shorthand -s → --stack",
            input: []string{"-s", "dev"},
            expected: translationResult{
                atmosArgs:     []string{"--stack", "dev"},
                separatedArgs: []string{},
            },
        },
        {
            name:  "atmos shorthand -s=dev → --stack=dev",
            input: []string{"-s=dev"},
            expected: translationResult{
                atmosArgs:     []string{"--stack=dev"},
                separatedArgs: []string{},
            },
        },

        // Category: Terraform managed flags (Atmos processes)
        {
            name:  "terraform -var → --var (Atmos managed)",
            input: []string{"-var", "foo=bar"},
            expected: translationResult{
                atmosArgs:     []string{"--var", "foo=bar"},
                separatedArgs: []string{},
            },
        },
        {
            name:  "terraform -var=foo=bar → --var=foo=bar",
            input: []string{"-var=foo=bar"},
            expected: translationResult{
                atmosArgs:     []string{"--var=foo=bar"},
                separatedArgs: []string{},
            },
        },
        {
            name:  "multiple -var flags",
            input: []string{"-var", "foo=bar", "-var", "baz=qux"},
            expected: translationResult{
                atmosArgs:     []string{"--var", "foo=bar", "--var", "baz=qux"},
                separatedArgs: []string{},
            },
        },

        // Category: Terraform pass-through flags
        {
            name:  "terraform -var-file → separated args",
            input: []string{"-var-file", "prod.tfvars"},
            expected: translationResult{
                atmosArgs:     []string{},
                separatedArgs: []string{"-var-file", "prod.tfvars"},
            },
        },
        {
            name:  "terraform -var-file=prod.tfvars → separated args",
            input: []string{"-var-file=prod.tfvars"},
            expected: translationResult{
                atmosArgs:     []string{},
                separatedArgs: []string{"-var-file=prod.tfvars"},
            },
        },
        {
            name:  "terraform -target → separated args",
            input: []string{"-target=aws_instance.app"},
            expected: translationResult{
                atmosArgs:     []string{},
                separatedArgs: []string{"-target=aws_instance.app"},
            },
        },

        // Category: Mixed scenarios
        {
            name:  "mixed: atmos + terraform managed + pass-through",
            input: []string{"-s", "dev", "-var", "foo=bar", "-var-file", "prod.tfvars"},
            expected: translationResult{
                atmosArgs:     []string{"--stack", "dev", "--var", "foo=bar"},
                separatedArgs: []string{"-var-file", "prod.tfvars"},
            },
        },
        {
            name:  "mixed with equals syntax",
            input: []string{"-s=dev", "-var=foo=bar", "-var-file=prod.tfvars"},
            expected: translationResult{
                atmosArgs:     []string{"--stack=dev", "--var=foo=bar"},
                separatedArgs: []string{"-var-file=prod.tfvars"},
            },
        },

        // Category: Unknown single-dash flags (let Cobra handle)
        {
            name:  "unknown -x flag (pass to Atmos for Cobra to validate)",
            input: []string{"-x"},
            expected: translationResult{
                atmosArgs:     []string{"-x"},  // Let Cobra error on unknown flag
                separatedArgs: []string{},
            },
        },

        // Category: Double-dash flags (already modern)
        {
            name:  "already modern --stack flag",
            input: []string{"--stack", "dev"},
            expected: translationResult{
                atmosArgs:     []string{"--stack", "dev"},
                separatedArgs: []string{},
            },
        },
        {
            name:  "already modern --var flag",
            input: []string{"--var", "foo=bar"},
            expected: translationResult{
                atmosArgs:     []string{"--var", "foo=bar"},
                separatedArgs: []string{},
            },
        },

        // Category: Positional args
        {
            name:  "positional args not prefixed with dash",
            input: []string{"plan", "vpc"},
            expected: translationResult{
                atmosArgs:     []string{"plan", "vpc"},
                separatedArgs: []string{},
            },
        },

        // Category: Complex real-world scenarios
        {
            name:  "realistic terraform plan command",
            input: []string{
                "plan", "vpc",
                "-s", "dev",
                "-var", "region=us-east-1",
                "-var", "env=prod",
                "-var-file", "common.tfvars",
                "-target", "aws_instance.app",
            },
            expected: translationResult{
                atmosArgs: []string{
                    "plan", "vpc",
                    "--stack", "dev",
                    "--var", "region=us-east-1",
                    "--var", "env=prod",
                },
                separatedArgs: []string{
                    "-var-file", "common.tfvars",
                    "-target", "aws_instance.app",
                },
            },
        },

        // Category: Edge cases
        {
            name:  "flag value that looks like flag",
            input: []string{"-var", "flag=-target"},
            expected: translationResult{
                atmosArgs:     []string{"--var", "flag=-target"},
                separatedArgs: []string{},
            },
        },
        {
            name:  "empty args",
            input: []string{},
            expected: translationResult{
                atmosArgs:     []string{},
                separatedArgs: []string{},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            translator := NewCompatibilityAliasTranslator(buildTestRegistry())
            atmosArgs, separatedArgs := translator.Translate(tt.input)

            assert.Equal(t, tt.expected.atmosArgs, atmosArgs)
            assert.Equal(t, tt.expected.separatedArgs, separatedArgs)
        })
    }
}
```

#### Step 1.2: Unified Parser Tests

Test the full parsing flow with compatibility aliases + Cobra validation:

```go
// pkg/flags/parser_test.go

func TestParser_WithCompatibilityAliases(t *testing.T) {
    tests := []struct {
        name               string
        args               []string
        expectedFlags      map[string]interface{}
        expectedPositional []string
        expectedSeparated  []string
        expectError        bool
    }{
        {
            name: "legacy terraform syntax",
            args: []string{"plan", "vpc", "-s", "dev", "-var", "foo=bar", "-var-file", "prod.tfvars"},
            expectedFlags: map[string]interface{}{
                "stack": "dev",
                "var":   []string{"foo=bar"},
            },
            expectedPositional: []string{"plan", "vpc"},
            expectedSeparated:  []string{"-var-file", "prod.tfvars"},
        },
        {
            name: "modern syntax with explicit separator",
            args: []string{"plan", "vpc", "--stack=dev", "--", "-var", "foo=bar"},
            expectedFlags: map[string]interface{}{
                "stack": "dev",
            },
            expectedPositional: []string{"plan", "vpc"},
            expectedSeparated:  []string{"-var", "foo=bar"},
        },
        {
            name: "mixed legacy and modern",
            args: []string{"plan", "vpc", "-s", "dev", "--dry-run", "-var", "x=y", "--", "-target=aws_instance.app"},
            expectedFlags: map[string]interface{}{
                "stack":   "dev",
                "dry-run": true,
                "var":     []string{"x=y"},
            },
            expectedPositional: []string{"plan", "vpc"},
            expectedSeparated:  []string{"-target=aws_instance.app"},
        },
        {
            name:        "unknown flag triggers Cobra validation error",
            args:        []string{"plan", "vpc", "--unknown-flag"},
            expectError: true,
        },
        {
            name: "multiple --var flags",
            args: []string{"plan", "vpc", "--var", "a=1", "--var", "b=2", "--var", "c=3"},
            expectedFlags: map[string]interface{}{
                "var": []string{"a=1", "b=2", "c=3"},
            },
            expectedPositional: []string{"plan", "vpc"},
            expectedSeparated:  []string{},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            parser := NewParser(
                WithSeparatedArgs(),
                WithCommonFlags(),
                WithTerraformVar(),
                WithTerraformPassThroughCompatibility(),
            )

            result, err := parser.Parse(context.Background(), tt.args)

            if tt.expectError {
                assert.Error(t, err)
                return
            }

            require.NoError(t, err)
            assert.Equal(t, tt.expectedFlags, result.Flags)
            assert.Equal(t, tt.expectedPositional, result.PositionalArgs)
            assert.Equal(t, tt.expectedSeparated, result.SeparatedArgs)
        })
    }
}
```

### Phase 2: Terraform Integration

Once core infrastructure is tested and working:

1. Update `TerraformParser` to use unified parser
2. Remove `DisableFlagParsing = true`
3. Update `internal/exec/terraform.go` to use new flags
4. Run existing terraform integration tests
5. Add new tests for compatibility alias scenarios

### Phase 3: Packer & Helmfile Integration

Apply same pattern to Packer and Helmfile.

### Phase 4: Remove Old Code

- Delete `PassThroughFlagParser` (replaced by unified parser)
- Delete custom `parseFlag()` logic
- Clean up global flag duplication

## Success Criteria

- ✅ All compatibility alias translation tests pass
- ✅ Unified parser tests pass with Cobra validation enabled
- ✅ Existing terraform/packer/helmfile tests pass
- ✅ No `DisableFlagParsing = true` in codebase
- ✅ Global flags registered once, inherited everywhere
- ✅ `--help` shows terraform flags properly
- ✅ Unknown flags trigger Cobra validation errors
- ✅ Legacy `-var` syntax works
- ✅ Modern `--var` syntax works
- ✅ `--` separator works

## Benefits

1. **Simplified architecture** - One parser, not two
2. **Cobra validation** - Proper error messages for unknown flags
3. **Documented flags** - `--help` shows all supported flags
4. **Type-safe** - Cobra validates flag types
5. **Backwards compatible** - Legacy syntax still works
6. **Future-proof** - New terraform flags work via `--`
7. **No global flag duplication** - Centralized once on RootCmd
8. **Maintainable** - Less custom code, more Cobra/pflag

## Migration Path

**Phase 1: Add compatibility aliases** (Backwards compatible)
- No breaking changes
- Legacy syntax works
- Modern syntax works

**Phase 2: Deprecation warnings** (Optional future)
- Detect legacy syntax
- Warn users to use modern syntax
- Document migration

**Phase 3: Remove compatibility aliases** (Breaking change, far future)
- Require modern syntax only
- Remove translation layer

## Breaking Changes & Blog Post Guidance

### Potential Breaking Changes

While compatibility aliases maintain backwards compatibility for most common patterns, there are edge cases that may break:

1. **NoOptDefVal Pattern Change**
   - **Old behavior**: `--identity prod` might have worked (undefined/inconsistent)
   - **New behavior**: `--identity=prod` required, or `--identity=` for interactive
   - **Reason**: Cobra's NoOptDefVal treats space-separated values as positional args
   - **Impact**: Low - this pattern was already inconsistent

2. **Unknown Terraform Flags**
   - **Old behavior**: Any terraform flag like `-weird-flag` passed through silently
   - **New behavior**: Unknown single-dash flags trigger Cobra error (unless after `--`)
   - **Recommended**: Use `--` separator for unknown/future terraform flags
   - **Impact**: Medium - users with custom/plugin terraform flags may be affected

3. **Flag Validation**
   - **Old behavior**: No validation on registered flags (DisableFlagParsing)
   - **New behavior**: Cobra validates flag types, required values, etc.
   - **Impact**: Low - catches user errors earlier

### Blog Post Content Guidance

**Title**: "Unified Flag Parsing: Better Validation, Clearer Syntax"

**Key Messages**:

1. **Prefer `--` separator for pass-through arguments**
   - Clear separation between Atmos and tool flags
   - Guaranteed to work with ANY terraform/packer/helmfile flag
   - Future-proof: new tool flags work automatically
   - Example: `atmos terraform plan vpc --stack=dev -- -var-file=prod.tfvars -target=aws_instance.app`

2. **Compatibility aliases maintain backwards compatibility**
   - Common patterns like `-var`, `-var-file`, `-target` still work
   - Translation happens automatically
   - No action required for most users

3. **Breaking changes are minimal but possible**
   - Recommend testing in non-production first
   - If you hit issues, use `--` separator as workaround
   - Report issues so we can add more compatibility aliases

4. **Benefits outweigh risks**
   - Proper Cobra validation catches typos/errors early
   - Better help documentation (`--help` shows supported flags)
   - Consistent behavior across terraform/packer/helmfile
   - Simpler codebase = faster feature development

**Migration Guide**:

```markdown
## Migration Checklist

### Before Upgrading

1. Review your CI/CD scripts for terraform/packer/helmfile commands
2. Look for patterns like `--identity prod` → change to `--identity=prod`
3. If using custom terraform plugins with unknown flags, add `--` separator

### Recommended Pattern (Future-Proof)

Always use `--` to separate Atmos flags from tool flags:

```bash
# Clear and explicit
atmos terraform plan vpc \
  --stack=prod \
  --identity=admin \
  -- \
  -var-file=common.tfvars \
  -var-file=prod.tfvars \
  -target=aws_instance.app
```

### Legacy Pattern (Still Works)

Common terraform flags work without `--`:

```bash
# Compatibility mode (still works)
atmos terraform plan vpc -s prod -var region=us-east-1 -var-file prod.tfvars
```

### If You Hit Issues

1. Add `--` separator before terraform/packer/helmfile flags
2. Use `=` syntax for all flags: `--stack=dev` not `--stack dev`
3. Report issue on GitHub with example command

### Testing

```bash
# Test your commands in dry-run mode first
atmos terraform plan vpc --stack=dev --dry-run -- -var-file=prod.tfvars

# Verify help shows expected flags
atmos terraform plan --help
```
```

**Risk Assessment Table**:

| Pattern | Works? | Notes |
|---------|--------|-------|
| `atmos terraform plan vpc -s dev` | ✅ Yes | Compatibility alias |
| `atmos terraform plan vpc --stack=dev` | ✅ Yes | Modern syntax |
| `atmos terraform plan vpc -var x=1` | ✅ Yes | Atmos-managed flag |
| `atmos terraform plan vpc -var-file prod.tfvars` | ✅ Yes | Pass-through compatibility |
| `atmos terraform plan vpc -- -weird-plugin-flag` | ✅ Yes | Explicit separator |
| `atmos terraform plan vpc -weird-plugin-flag` | ⚠️ Maybe | Only if registered as compatibility alias |
| `--identity prod` | ❌ No | Use `--identity=prod` |

**Tone**: Positive but honest about tradeoffs. Emphasize that `--` separator is the "escape hatch" that guarantees compatibility.
