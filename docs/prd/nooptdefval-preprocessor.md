# NoOptDefVal Space-to-Equals Preprocessor

## Problem Statement

Cobra/pflag's `NoOptDefVal` feature has a documented limitation: it only works with equals syntax (`--flag=value`), not space-separated syntax (`--flag value`). This is acknowledged in:
- pflag issue #134 (closed as "COMPLETED" - treating ambiguity as design constraint)
- pflag issue #321 (closed as "not planned")
- cobra issue #1962 (maintainer confirmed: "only the = form can be used")

### The Ambiguity

When a flag has `NoOptDefVal`, it can be used in two ways:
- `--flag` (no value) → uses NoOptDefVal
- `--flag=value` → uses explicit value

But `--flag value` creates ambiguity:
- Is `value` the flag's value?
- Or is `value` a positional argument?

Cobra's maintainers decided equals syntax is required to disambiguate.

### Impact on Atmos

Atmos uses `NoOptDefVal` for two flags:
1. **`--identity`** - Interactive selection when no value: `--identity` → selector, `--identity=prod` → use prod
2. **`--pager`** - Enable pager: `--pager` → enable with default, `--pager=less` → use specific pager

Users expect both syntaxes to work:
- `--identity=prod` ✅ (works)
- `--identity prod` ❌ (doesn't work - gets NoOptDefVal, "prod" becomes positional arg)

We need to maintain backward compatibility with existing usage.

## Solution: Declarative Preprocessor

### Design Principles

1. **Declarative** - Flags with `NoOptDefVal` automatically get preprocessing, no manual opt-in per command
2. **Registry-based** - Preprocessor uses flag metadata from FlagRegistry
3. **Early in pipeline** - Runs before Cobra parsing, after `--` separator splitting
4. **Transparent** - Commands don't need to know about this, it's handled in the parser

### Implementation Approach

#### 1. Add preprocessor method to FlagRegistry

```go
// PreprocessNoOptDefValArgs rewrites space-separated flag syntax to equals syntax
// for flags that have NoOptDefVal set.
//
// This maintains backward compatibility with user expectations while working within
// Cobra's documented behavior that NoOptDefVal requires equals syntax.
//
// Example:
//   Input:  ["--identity", "prod", "plan"]
//   Output: ["--identity=prod", "plan"]
//
// Only processes flags registered in this registry with non-empty NoOptDefVal.
func (r *FlagRegistry) PreprocessNoOptDefValArgs(args []string) []string
```

Algorithm:
1. Build set of flag names (long + shorthand) that have NoOptDefVal
2. Iterate through args
3. When we find a flag in the set:
   - Check if next arg exists and doesn't start with `-`
   - If so, combine them: `flag` + `=` + `nextArg`
   - Skip the next arg (already consumed)
4. Return rewritten args

#### 2. Integrate into AtmosFlagParser.Parse()

Add new step after splitting at `--` separator:

```go
// Step 2.5: Preprocess NoOptDefVal flags (identity, pager)
// Rewrite --flag value → --flag=value for flags with NoOptDefVal.
// This maintains backward compatibility while working within Cobra's documented
// limitation that NoOptDefVal requires equals syntax.
argsBeforeSep = p.registry.PreprocessNoOptDefValArgs(argsBeforeSep)
```

This runs BEFORE:
- Cobra shorthand normalization (Step 3)
- Compatibility alias translation (Step 4)
- Cobra parsing (Step 5)

#### 3. Wire up registry to parser

AtmosFlagParser needs access to the registry:

```go
type AtmosFlagParser struct {
    cmd        *cobra.Command
    viper      *viper.Viper
    translator *CompatibilityFlagsTranslator
    registry   *FlagRegistry  // NEW: for preprocessing
}
```

Parser constructors need to accept registry parameter.

### Edge Cases to Handle

1. **Flag with equals already** - `--identity=prod` should pass through unchanged
2. **Flag at end of args** - `--identity` with no next arg should pass through (will use NoOptDefVal)
3. **Flag followed by another flag** - `--identity --dry-run` should pass through (identity uses NoOptDefVal)
4. **Shorthand forms** - `-i prod` should become `-i=prod`
5. **Multiple NoOptDefVal flags** - Both identity and pager should be processed
6. **Args after separator** - Don't preprocess args after `--` (they're pass-through)

### Files to Modify

1. **`pkg/flags/registry.go`**
   - Add `PreprocessNoOptDefValArgs()` method

2. **`pkg/flags/flag_parser.go`**
   - Add `registry` field to `AtmosFlagParser`
   - Update `NewAtmosFlagParser()` constructor
   - Add preprocessing step in `Parse()` method

3. **All parser constructors** that create `AtmosFlagParser`
   - Pass registry to constructor
   - `pkg/flags/auth/exec/parser.go`
   - `pkg/flags/auth/shell/parser.go`
   - `pkg/flags/terraform/*.go`
   - `pkg/flags/helmfile/*.go`
   - `pkg/flags/packer/*.go`

### Testing Strategy

1. **Unit tests for PreprocessNoOptDefValArgs()**
   - Test each edge case listed above
   - Verify only registered NoOptDefVal flags are preprocessed
   - Verify flags without NoOptDefVal pass through unchanged

2. **Update parser integration tests**
   - Both `--identity value` and `--identity=value` should work
   - Keep existing equals-syntax tests (still the documented way)
   - Add space-syntax tests (now works via preprocessing)

3. **Update command tests**
   - Verify commands work with both syntaxes
   - No changes needed to command code itself

### Documentation Updates

1. **Code comments** - Document that this is a compatibility shim for Cobra's limitation
2. **User docs** - Both syntaxes work, but equals syntax is canonical
3. **PRD** - This document captures the design decision

## Benefits

1. **Backward compatibility** - Users can use the syntax they expect
2. **Declarative** - No per-command code, just flag metadata
3. **Transparent** - Works at parser level, commands unchanged
4. **Maintainable** - Centralized in one place, easy to extend to new flags
5. **Well-documented** - Clear why we're doing this (Cobra limitation)

## Alternatives Considered

### 1. Remove NoOptDefVal
- Would break interactive selection feature
- Less ergonomic user experience
- Would need different approach for optional flag values

### 2. Manual preprocessing in each command
- Violates DRY principle
- Easy to forget
- Not declarative

### 3. Custom flag parsing (replace Cobra)
- Too much work
- Lose Cobra's validation and other features
- Would need to maintain our own flag parser

## Conclusion

The declarative preprocessor approach is the best solution:
- Maintains backward compatibility
- Works within Cobra's documented behavior
- Minimal changes required
- Scales to any future NoOptDefVal flags
