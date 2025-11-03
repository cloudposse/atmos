# Unified Flag Parsing: Summary & Key Insights

## TL;DR

**Problem**: Atmos has 4 different flag parsing implementations with ~700 lines of duplicated code. Terraform's complex flag syntax (single/double dash, optional values, positional args) makes unified parsing particularly challenging.

**Solution**: Two-phase parsing system with interface-driven design:
1. **Phase 1**: Extract Atmos-specific flags
2. **Phase 2**: Pass-through everything else to underlying tools

**Benefits**: 80-90% test coverage, single implementation, consistent precedence, backward compatible.

---

## The Three-Way Flag Parsing Problem

**The core challenge**: Atmos must support **THREE different types of flags/arguments concurrently** in a single command:

1. **Atmos-style flags** (double-dash GNU-style): `--stack prod`, `--dry-run`, `-s prod`
2. **Terraform-style flags** (single-dash POSIX-style): `-var 'x=1'`, `-out=plan.tfplan`
3. **Positional arguments**: `plan` (subcommand), `vpc` (component name)

### Example: All Three Types Together

```bash
atmos terraform plan vpc --stack prod -var 'env=prod' --dry-run -out=plan.tfplan
                    ^^^  ^^^^^^^^^^^^ ^^^^^^^^^^^^^^^^ ^^^^^^^^^ ^^^^^^^^^^^^^^^^^
                    pos  Atmos        Terraform         Atmos     Terraform
```

**Parser must**:
- Extract Atmos flags (`--stack`, `--dry-run`)
- Extract positional args (`plan`, `vpc`)
- Preserve Terraform flags exactly as-is (`-var 'env=prod'`, `-out=plan.tfplan`)
- Handle all three types in any order
- Maintain Terraform flag order for execution

### Why This Is Uniquely Hard

Standard flag parsers (including Cobra/pflag) **cannot** handle this because:

1. **Ambiguous prefixes**: Both Atmos (`-s`, `--stack`) and Terraform (`-var`, `-out`) use dashes
2. **Unknown flags**: Parser doesn't know which flags belong to which system
3. **Order preservation**: Must preserve Terraform flag order exactly (critical for execution)
4. **Value extraction**: Must distinguish `-s prod` (Atmos) from `-var 'x=1'` (Terraform)
5. **Shorthand ambiguity**: `-s` could be Atmos shorthand OR a Terraform flag OR component name

### Additional Terraform-Specific Challenges

### 1. Mixed Flag Styles
```bash
terraform plan -var 'foo=bar' --help -out=plan.tfplan
               ^^^^^ single    ^^^^^^ double  ^^^ single with =
```

### 2. Optional Boolean Values
```bash
--upload-status          # defaults to true
--upload-status=true     # explicit true
--upload-status=false    # explicit false
```

Standard Cobra boolean flags don't support "flag alone = true" pattern.

### 3. Positional Args + Flags Ambiguity
```bash
terraform import aws_instance.example i-abc123 -var-file=vars.tfvars
                 ^^^^^^^^^^^^^^^^^^^^ ^^^^^^^^  ^^^^^^^^^^^^^^^^^^^
                 positional arg #1    pos arg #2  flag
```

Hard to distinguish positional args from flag values.

### 4. Atmos Wrapper Challenge
```bash
# Atmos flags mixed with Terraform flags - which is which?
atmos terraform plan vpc -s prod --dry-run -var-file=prod.tfvars -out=plan.tfplan
                    ^^^ ^^^^^^^ ^^^^^^^^^ ^^^^^^^^^^^^^^^^^^^^ ^^^^^^^^^^^^^^^^
                    cmd   Atmos   Atmos    Terraform?         Terraform?
```

**Current approach**: `DisableFlagParsing=true` + manual parsing of everything
**Problem**: Error-prone, hard to test, duplicated logic

---

## Proposed Solution

### Two-Phase Parsing Strategy

**Phase 1: Extract Atmos Flags**
- Look for known Atmos flags (`--stack`, `--dry-run`, `--identity`, etc.)
- Support both `--` separator (explicit) and flag recognition (implicit)
- Handle optional boolean flags (`--upload-status`)
- Leave everything else untouched

**Phase 2: Pass-Through to Tool**
- Pass remaining args directly to Terraform/Helmfile/Packer
- No parsing, no modification
- Preserve flag syntax, ordering, quoting

### Example 1: All Three Types Interleaved (Implicit Mode)

```bash
# Input - Atmos flags, Terraform flags, and positional args all mixed
atmos terraform plan vpc --stack prod -var 'env=prod' --dry-run -out=plan.tfplan
                    ^^^  ^^^^^^^^^^^^ ^^^^^^^^^^^^^^^^ ^^^^^^^^^ ^^^^^^^^^^^^^^^^^
                    pos  Atmos        Terraform         Atmos     Terraform

# Phase 1 extracts Atmos flags and positional args:
{
    subCommand: "plan",
    component: "vpc",
    atmosFlags: {
        stack: "prod",
        dryRun: true
    }
}

# Phase 2 passes remaining to Terraform (preserving order):
["-var", "env=prod", "-out=plan.tfplan"]
```

**Key point**: Parser recognizes `--stack` and `--dry-run` as Atmos flags (from registry), leaves `-var` and `-out` untouched for Terraform.

### Example 2: Explicit Mode with `--` Separator (Recommended)

```bash
# Input - Clear separation with --
atmos terraform plan vpc -s prod --upload-status=false -- -var-file=prod.tfvars -out=plan.tfplan
                        ^^^ ^^^^^^^ ^^^^^^^^^^^^^^^^^^^    ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^
                        cmd  Atmos   Atmos flags          Terraform flags (after --)

# Phase 1 extracts everything before --:
{
    subCommand: "plan",
    component: "vpc",
    atmosFlags: {
        stack: "prod",
        uploadStatus: false
    }
}

# Phase 2 passes everything after -- to Terraform:
["-var-file=prod.tfvars", "-out=plan.tfplan"]
```

**Key point**: `--` separator eliminates all ambiguity - everything before is Atmos/positional, everything after is Terraform.

### Example 3: Complex Real-World Usage

```bash
# All three types + both Atmos styles + optional bool flag + separator
atmos terraform plan vpc \
    --stack prod \                    # Atmos flag (double-dash)
    -i admin \                        # Atmos flag (shorthand for --identity)
    --dry-run \                       # Atmos flag (boolean)
    --upload-status=false \           # Atmos flag (optional bool with value)
    -var 'region=us-east-1' \         # Terraform flag (mixed in)
    -- \                              # Separator
    -var 'override=true' \            # Terraform flag (after separator)
    -var-file=common.tfvars \         # Terraform flag
    -out=plan.tfplan                  # Terraform flag

# Phase 1 extracts:
{
    subCommand: "plan",
    component: "vpc",
    atmosFlags: {
        stack: "prod",
        identity: "admin",
        dryRun: true,
        uploadStatus: false
    }
}

# Phase 2 passes to Terraform:
["-var", "region=us-east-1", "-var", "override=true", "-var-file=common.tfvars", "-out=plan.tfplan"]
```

**Key point**: System handles all complexity - Atmos double-dash, Atmos shorthand, Terraform flags, positional args, optional bool values, and `--` separator - all in one command.

---

## Architecture

### Core Interfaces

```go
// PassThroughHandler - smart arg separation
type PassThroughHandler interface {
    SplitAtDoubleDash(args []string) (before, after []string)
    ExtractAtmosFlags(args []string) (atmosFlags map[string]interface{}, remaining []string, error)
    ExtractPositionalArgs(args []string, count int) (positional, remaining []string, error)
}

// OptionalBoolFlag - handles --flag, --flag=true, --flag=false
type OptionalBoolFlag interface {
    Parse(args []string, flagName string) (value bool, present bool, error)
    Remove(args []string, flagName string) []string
}

// FlagParser - unified parsing interface
type FlagParser interface {
    Parse(ctx context.Context, args []string) (*ParsedConfig, error)
    RegisterFlags(cmd *cobra.Command)
    BindToViper(v *viper.Viper) error
}
```

### PassThroughFlagParser Implementation

```go
func (p *PassThroughFlagParser) Parse(ctx context.Context, args []string) (*ParsedConfig, error) {
    // Step 1: Split at -- if present
    beforeDash, afterDash := p.handler.SplitAtDoubleDash(args)

    var toolArgs []string
    if len(afterDash) > 0 {
        // Explicit mode
        atmosFlags, remaining, _ := p.handler.ExtractAtmosFlags(beforeDash)
        toolArgs = append(remaining, afterDash...)
    } else {
        // Implicit mode
        atmosFlags, remaining, _ := p.handler.ExtractAtmosFlags(args)
        toolArgs = remaining
    }

    // Step 2: Handle optional boolean flags (--upload-status)
    for _, flagName := range p.optionalBoolFlags {
        if value, present, _ := p.parseOptionalBoolFlag(toolArgs, flagName); present {
            atmosFlags[flagName] = value
            toolArgs = p.removeFlag(toolArgs, flagName)
        }
    }

    // Step 3: Extract positional args (component, subcommand)
    positional, remaining, _ := p.handler.ExtractPositionalArgs(toolArgs, 2)

    return &ParsedConfig{
        AtmosFlags:       atmosFlags,
        SubCommand:       positional[0],
        ComponentName:    positional[1],
        PassThroughArgs:  remaining,
    }, nil
}
```

**Key Benefits**:
- ✅ Handles both explicit (`--`) and implicit modes
- ✅ Supports optional boolean flags (`--flag`, `--flag=true`, `--flag=false`)
- ✅ Doesn't parse or modify tool flags
- ✅ Preserves argument order
- ✅ Clear error messages
- ✅ Fully testable with unit tests

---

## Testing Strategy

### Unit Tests for Complex Scenarios

```go
func TestPassThroughFlagParser_ComplexScenarios(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        want     *ParsedConfig
    }{
        {
            name: "optional bool flag alone",
            args: []string{"plan", "vpc", "-s", "prod", "--upload-status"},
            want: &ParsedConfig{
                SubCommand:    "plan",
                ComponentName: "vpc",
                AtmosFlags: map[string]interface{}{
                    "stack":        "prod",
                    "uploadStatus": true,
                },
                PassThroughArgs: []string{},
            },
        },
        {
            name: "optional bool flag with false",
            args: []string{"plan", "vpc", "-s", "prod", "--upload-status=false"},
            want: &ParsedConfig{
                SubCommand:    "plan",
                ComponentName: "vpc",
                AtmosFlags: map[string]interface{}{
                    "stack":        "prod",
                    "uploadStatus": false,
                },
                PassThroughArgs: []string{},
            },
        },
        {
            name: "explicit mode with -- separator",
            args: []string{"plan", "vpc", "-s", "prod", "--", "-var-file=prod.tfvars", "-out=plan.tfplan"},
            want: &ParsedConfig{
                SubCommand:    "plan",
                ComponentName: "vpc",
                AtmosFlags: map[string]interface{}{
                    "stack": "prod",
                },
                PassThroughArgs: []string{"-var-file=prod.tfvars", "-out=plan.tfplan"},
            },
        },
        {
            name: "implicit mode without --",
            args: []string{"plan", "vpc", "-s", "prod", "-var-file=prod.tfvars"},
            want: &ParsedConfig{
                SubCommand:    "plan",
                ComponentName: "vpc",
                AtmosFlags: map[string]interface{}{
                    "stack": "prod",
                },
                PassThroughArgs: []string{"-var-file=prod.tfvars"},
            },
        },
        {
            name: "mixed atmos and terraform flags",
            args: []string{"plan", "vpc", "-s", "prod", "--dry-run", "-var", "foo=bar", "-out=plan.tfplan"},
            want: &ParsedConfig{
                SubCommand:    "plan",
                ComponentName: "vpc",
                AtmosFlags: map[string]interface{}{
                    "stack":  "prod",
                    "dryRun": true,
                },
                PassThroughArgs: []string{"-var", "foo=bar", "-out=plan.tfplan"},
            },
        },
        {
            name: "positional args that look like flags",
            args: []string{"workspace", "select", "--", "-s"},
            want: &ParsedConfig{
                SubCommand:      "workspace",
                ComponentName:   "select",
                AtmosFlags:      map[string]interface{}{},
                PassThroughArgs: []string{"-s"},
            },
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            parser := NewPassThroughFlagParser()
            got, err := parser.Parse(context.Background(), tt.args)

            assert.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

**Target Coverage**: 90%+ for flag parsing logic

---

## Migration Strategy

### Phase 1: Core Infrastructure (Week 1)
- Create `pkg/flagparser/` package with interfaces
- Implement `PassThroughHandler` and `OptionalBoolFlag`
- Implement `PassThroughFlagParser`
- Comprehensive unit tests (90%+ coverage)

### Phase 2: Terraform Migration (Week 2)
- Update Terraform command to use `PassThroughFlagParser`
- Remove custom parsing from `cmd/terraform.go` and `internal/exec/terraform.go`
- Integration tests for all Terraform subcommands
- Backward compatibility tests

### Phase 3: Helmfile & Packer (Week 3)
- Apply same pattern to Helmfile and Packer
- Remove duplicated code
- Integration tests

### Phase 4: Standard Commands (Week 4)
- Migrate Validate, Describe, Workflow, etc.
- Use `StandardFlagParser` (simpler than pass-through)
- Middleware pattern for config loading

### Phase 5: Global Flags (Week 5)
- Ensure global flags work in all commands
- Test propagation across command hierarchy

### Phase 6: Cleanup (Week 6)
- Remove deprecated code (`extractTrailingArgs()`, `processArgsAndFlags()`, etc.)
- Update documentation
- Final integration tests

---

## Key Decisions

### 1. Two-Phase Parsing vs. Single Pass

**Decision**: Use two-phase parsing
**Rationale**:
- Terraform's syntax is too complex for single-pass parsing
- Allows us to be selective about what we parse (Atmos flags only)
- Preserves tool flags exactly as entered
- Easier to test and reason about

### 2. Explicit (`--`) vs. Implicit Mode

**Decision**: Support both, recommend explicit
**Rationale**:
- Explicit mode removes ambiguity
- Implicit mode needed for backward compatibility
- Users shouldn't need to change existing commands immediately
- Documentation should encourage `--` for new usage

### 3. Custom Parser vs. Cobra/pflag

**Decision**: Custom parser for Atmos flags, no parsing for tool flags
**Rationale**:
- Cobra/pflag can't handle mixed Atmos/tool flags without errors
- `DisableFlagParsing=true` is too blunt (parses nothing)
- Selective parsing gives us best of both worlds
- Still use Cobra for command structure and help

### 4. Optional Boolean Flag Handling

**Decision**: Custom implementation for `--flag`, `--flag=true`, `--flag=false`
**Rationale**:
- Cobra's `NoOptDefVal` doesn't support `--flag=false`
- This pattern is common in CLI tools (e.g., Docker)
- Worth the small amount of custom code

---

## Success Metrics

- [ ] **Code Reduction**: Remove 500-700 lines of duplicated flag parsing
- [ ] **Test Coverage**: 80-90% for `pkg/flagparser/`
- [ ] **Backward Compatibility**: 100% of existing commands work unchanged
- [ ] **Performance**: No regression in command execution time
- [ ] **Consistency**: All commands use unified system

---

## Risks & Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Breaking existing usage | High | Comprehensive backward compatibility tests, gradual rollout |
| Performance regression | Medium | Benchmark tests, performance profiling |
| Increased complexity | Medium | Clear documentation, simple interfaces, code examples |
| Edge cases not covered | Medium | Extensive unit and integration tests, beta testing |

---

## Next Steps

1. **Review this PRD** with team for alignment on approach
2. **Create POC** of `PassThroughFlagParser` with unit tests
3. **Validate** with one Terraform subcommand (e.g., `plan`)
4. **Iterate** based on findings
5. **Begin migration** following phase plan

---

## References

- **Full PRD**: `docs/prd/unified-flag-parsing.md`
- **Research notes**: `.scratch/unified-flag-parsing-research.md` (working notes, not committed)
- **Current implementation**: `cmd/terraform.go`, `internal/exec/cli_utils.go`
- **Parsing challenges**: See test cases in `internal/exec/terraform_utils_test.go`
