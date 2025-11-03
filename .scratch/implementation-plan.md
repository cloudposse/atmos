# Unified Flag Parsing - Implementation Plan

## Overview

This document outlines the step-by-step implementation plan for the unified flag parsing system, following the user's requirement to build it **irrespective of Terraform considerations** and test exhaustively FIRST before integration.

## Current State Analysis

### Hacks and Workarounds to Eliminate

#### 1. `DisableFlagParsing = true` (Terraform, Helmfile, Packer)

**Current hack:**
```go
// cmd/terraform.go
terraformCmd.DisableFlagParsing = true  // Forces manual parsing of EVERYTHING
```

**Why it exists:** Terraform uses single-dash flags (`-var`, `-out`) that Cobra would try to parse

**Problems:**
- Loses Cobra's built-in `--` separator handling
- No Viper integration possible
- Must manually parse all args
- Can't use Cobra's flag validation

**Will be eliminated by:** Two-phase parsing where Atmos flags are parsed by Cobra, rest passed through

#### 2. Manual `os.Args` Parsing for `--` Separator

**Current hack:**
```go
// cmd/args_separator.go:67-109
func ExtractSeparatedArgs(cmd *cobra.Command, args []string, osArgs []string) *SeparatedCommandArgs {
    separatorIndex := lo.IndexOf(osArgs, "--")
    // 40+ lines of manual parsing logic
    // Build lookup map, filter args, reconstruct slices
}
```

**Why it exists:** When `DisableFlagParsing=true`, Cobra consumes `--` and we lose position info

**Problems:**
- Complex manual logic (40+ lines)
- Requires passing `os.Args` everywhere
- Doesn't work with `cmd.SetArgs()` in tests (uses global `os.Args`)
- Fragile - sensitive to arg ordering

**Will be eliminated by:** Using Cobra's `cmd.ArgsLenAtDash()` when flag parsing is enabled

#### 3. Manual Identity Flag Parsing

**Current hack:**
```go
// cmd/identity_flag.go:30-53
func GetIdentityFromFlags(cmd *cobra.Command, osArgs []string) string {
    // Manually parse os.Args to work around Cobra NoOptDefVal quirk
    identity := extractIdentityFromArgs(osArgs)
    // 70+ lines of manual parsing for --identity and -i
}
```

**Why it exists:** Cobra's NoOptDefVal doesn't work correctly with positional args

**Problems:**
- Manual parsing of `--identity`, `--identity=value`, `-i`, `-i=value`
- Must distinguish `--identity value` from `--identity` (interactive)
- Can't leverage Cobra's built-in flag parsing
- Doesn't integrate with Viper

**Will be eliminated by:** Proper NoOptDefVal handling with manual precedence checking (but cleaner)

#### 4. Two Different Separator Implementations

**Current hack:**
- `flag_utils.go::ParseSeparatedArgs()` - Uses `ArgsLenAtDash()` (clean)
- `args_separator.go::ExtractSeparatedArgs()` - Manual `os.Args` parsing (hack)

**Why it exists:** Different commands have different needs (DisableFlagParsing or not)

**Problems:**
- Duplicated logic with different approaches
- Developers must know which one to use
- Inconsistent behavior across commands

**Will be eliminated by:** Single unified parser that works for all commands

#### 5. Package-Level Flag Variables

**Current hack:**
```go
// cmd/version/version.go
var (
    checkFlag     bool
    versionFormat string
)

func init() {
    versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "...")
}
```

**Why it exists:** Traditional Cobra pattern, simple but not testable

**Problems:**
- Global state makes testing difficult
- No env var support
- No config file support
- No precedence enforcement

**Will be eliminated by:** Parser as struct field, values retrieved from Viper

#### 6. No Precedence Enforcement

**Current hack:** Each command manually checks flags, then env vars, then config (if at all)

**Problems:**
- Inconsistent precedence order across commands
- Manual implementation prone to bugs
- Easy to forget env var support

**Will be eliminated by:** Viper automatic precedence (flag > env > config > default)

## Implementation Phases

### Phase 0: Preparation (Current)

- [x] Research current implementation
- [x] Document hacks and workarounds
- [x] Create PRD with requirements
- [x] Merge latest main branch changes
- [ ] Finalize implementation plan
- [ ] Get user approval to proceed

### Phase 1: Core Flag Parser Infrastructure (Week 1)

**Goal:** Build standalone `pkg/flagparser/` package with 90%+ coverage

#### Tasks:

1. **Create package structure**
   ```
   pkg/flagparser/
   ├── parser.go           # FlagParser interface
   ├── standard.go         # StandardFlagParser implementation
   ├── passthrough.go      # PassThroughFlagParser implementation
   ├── registry.go         # FlagRegistry for reusable flags
   ├── options.go          # Options pattern for configuration
   ├── types.go            # Flag type definitions
   ├── parser_test.go      # 90%+ coverage
   ├── standard_test.go
   ├── passthrough_test.go
   └── mock_parser_test.go # Generated mocks
   ```

2. **Define core interfaces**
   ```go
   type FlagParser interface {
       RegisterFlags(cmd *cobra.Command)
       BindToViper(v *viper.Viper) error
       Parse(ctx context.Context, args []string) (*ParsedConfig, error)
   }

   type PassThroughHandler interface {
       ExtractAtmosFlags(args []string) (atmosFlags map[string]interface{}, remaining []string, error)
       SplitAtDoubleDash(args []string) (before, after []string)
   }
   ```

3. **Implement StandardFlagParser**
   - Register flags with Cobra
   - Bind to Viper with env var support
   - Handle NoOptDefVal for identity pattern
   - Pure functions, no I/O

4. **Implement PassThroughFlagParser**
   - Two-phase parsing (extract Atmos flags, pass rest through)
   - Use Cobra's `ArgsLenAtDash()` for `--` separator
   - Extract known Atmos flags from mixed args
   - Preserve tool flag ordering

5. **Implement FlagRegistry**
   - Reusable flag definitions (stack, identity, dry-run, etc.)
   - Pre-configured common flags
   - Easy to extend

6. **Write comprehensive tests**
   - Unit tests for all functions (90%+ coverage)
   - Table-driven tests with edge cases
   - Mock component integration tests
   - TestKit integration for isolation
   - **100+ test scenarios** as specified in PRD

#### Success Criteria:
- [ ] All interfaces defined and documented
- [ ] StandardFlagParser implemented with tests
- [ ] PassThroughFlagParser implemented with tests
- [ ] 90%+ test coverage
- [ ] No external dependencies (pure functions)
- [ ] Mock component tests passing

### Phase 2: Integration with Mock Component (Week 2)

**Goal:** Test parser with mock component, verify all edge cases work

#### Tasks:

1. **Create test harness with mock component**
   ```go
   // pkg/flagparser/integration_test.go
   func TestWithMockComponent(t *testing.T) {
       // Use pkg/component/mock
       // Test all flag patterns with actual component execution
   }
   ```

2. **Test all edge cases from PRD**
   - Component names that look like flags (`-s`, `--help`)
   - Stack names with special chars (`prod/us-east-1`)
   - Values with `=`, `-`, `--`, quotes
   - Unicode, long args, whitespace
   - 100+ scenarios documented in PRD

3. **Test identity flag pattern thoroughly**
   - `--identity=value` (equals)
   - `--identity value` (space)
   - `--identity` (interactive/NoOptDefVal)
   - No flag (env var/config)
   - Precedence: flag > env > config

4. **Test with TestKit for isolation**
   - Verify no pollution between tests
   - Clean RootCmd state
   - Works with subtests

#### Success Criteria:
- [ ] All 100+ edge cases tested
- [ ] Mock component integration working
- [ ] Identity flag pattern verified
- [ ] TestKit integration verified
- [ ] All tests passing in isolation

### Phase 3: Viper Integration & Config Loader (Week 2-3)

**Goal:** Integrate with Viper for automatic precedence

#### Tasks:

1. **Create ConfigLoader interface**
   ```go
   type ConfigLoader interface {
       Load(ctx context.Context, opts ...LoadOption) (*Config, error)
       Viper() *viper.Viper
       Reload(ctx context.Context) error
   }
   ```

2. **Implement ViperConfigLoader**
   - Load config files
   - Bind environment variables
   - Automatic precedence handling
   - Integration with existing `pkg/config/`

3. **Test precedence order**
   - Flag > env var > config > default
   - With all flag types (string, bool, int)
   - With NoOptDefVal flags

4. **Fix log level initialization**
   - Early extraction before full config load
   - Support `--logs-level`, `ATMOS_LOGS_LEVEL`, config file
   - Eliminate manual `parseFlags()` hack

#### Success Criteria:
- [ ] ConfigLoader interface defined
- [ ] ViperConfigLoader implemented
- [ ] Precedence order enforced automatically
- [ ] Log level initialization fixed
- [ ] All precedence tests passing

### Phase 4: Middleware Pattern (Week 3)

**Goal:** Create composable middleware for config pipeline

#### Tasks:

1. **Define middleware interface**
   ```go
   type CobraMiddleware func(cmd *cobra.Command, args []string) error

   func ComposeMiddleware(middlewares ...CobraMiddleware) CobraMiddleware
   ```

2. **Implement core middleware**
   - ConfigMiddleware (load config)
   - AuthMiddleware (handle identity)
   - ValidationMiddleware (validate flags)

3. **Test middleware composition**
   - Chain multiple middleware
   - Error handling
   - Context propagation

#### Success Criteria:
- [ ] Middleware pattern implemented
- [ ] Core middleware working
- [ ] Composition tested
- [ ] Context propagation verified

### Phase 5: Command Registry Integration (Week 3-4)

**Goal:** Make it easy for CommandProvider implementations to use parser

#### Tasks:

1. **Update example CommandProvider**
   ```go
   type VersionCommandProvider struct {
       parser flagparser.FlagParser
       loader config.ConfigLoader
   }

   func (v *VersionCommandProvider) GetCommand() *cobra.Command {
       cmd := &cobra.Command{...}
       v.parser.RegisterFlags(cmd)
       v.parser.BindToViper(v.loader.Viper())
       cmd.PersistentPreRunE = middleware.ConfigMiddleware(v.loader)
       return cmd
   }
   ```

2. **Create migration guide**
   - Before/after examples
   - Step-by-step instructions
   - Common pitfalls

3. **Test with multiple command types**
   - Simple command (about, version)
   - Command with flags (describe, list)
   - Command with subcommands (auth, terraform)

#### Success Criteria:
- [ ] CommandProvider integration pattern documented
- [ ] Example implementations working
- [ ] Migration guide created
- [ ] Multiple command types tested

### Phase 6: Custom Command Support (Week 4)

**Goal:** Dynamic flag registration from YAML config

#### Tasks:

1. **Implement CustomCommandFlagParser**
   - Parse flag spec from YAML
   - Dynamic flag registration
   - Support NoOptDefVal via `no_opt_default` field
   - Type validation (string, bool, int, float)

2. **Update YAML schema**
   ```yaml
   commands:
     - name: deploy
       flags:
         - name: environment
           type: string
           required: true
         - name: identity
           type: string
           no_opt_default: "__SELECT__"  # Enable identity pattern
   ```

3. **Update JSON schemas**
   - `pkg/datafetcher/schema/` updates
   - Validation for new fields

4. **Test custom commands**
   - Simple custom command
   - Custom command with 10+ flags
   - Custom command with identity flag
   - Custom command with required validation

#### Success Criteria:
- [ ] CustomCommandFlagParser implemented
- [ ] YAML schema updated
- [ ] JSON schemas updated
- [ ] Custom commands working with all patterns

### Phase 7: Baseline Testing (Week 4-5)

**Goal:** Establish baseline behavior BEFORE migrating production commands

#### Tasks:

1. **Document current Terraform behavior**
   - Run all terraform commands with various flag combinations
   - Capture exact output and behavior
   - Create baseline test suite

2. **Document current Helmfile/Packer behavior**
   - Same as Terraform

3. **Create regression test suite**
   - Captures current behavior exactly
   - Will be used to verify no regressions during migration

4. **Run baseline tests**
   - All tests pass with current implementation
   - Documented in `tests/baseline/`

#### Success Criteria:
- [ ] Terraform baseline documented
- [ ] Helmfile baseline documented
- [ ] Packer baseline documented
- [ ] Regression test suite created
- [ ] All baseline tests passing

### Phase 8: Migration - Pass-Through Commands (Week 5)

**Goal:** Migrate Terraform, Helmfile, Packer to unified parser

**CRITICAL:** Only start this phase after Phases 1-7 are complete and all tests passing.

#### Tasks:

1. **Migrate Terraform command**
   - Remove `DisableFlagParsing = true`
   - Use PassThroughFlagParser
   - Verify all baseline tests still pass
   - Add new tests for improvements

2. **Migrate Helmfile command**
   - Same pattern as Terraform

3. **Migrate Packer command**
   - Same pattern as Terraform

4. **Remove deprecated code**
   - Delete `ExtractSeparatedArgs` from `args_separator.go`
   - Keep only `ParseSeparatedArgs` from `flag_utils.go`
   - Delete manual identity parsing from `identity_flag.go`

#### Success Criteria:
- [ ] Terraform migrated, all tests passing
- [ ] Helmfile migrated, all tests passing
- [ ] Packer migrated, all tests passing
- [ ] No regressions in baseline tests
- [ ] Deprecated code removed

### Phase 9: Migration - Standard Commands (Week 5-6)

**Goal:** Migrate standard commands to unified parser

#### Tasks:

1. **Migrate version command** (simplest)
   - Example migration for documentation
   - Remove package-level variables
   - Add env var support
   - Verify tests pass

2. **Migrate describe commands**
   - Use StandardFlagParser
   - Viper integration
   - Tests

3. **Migrate list, validate, vendor commands**
   - Same pattern

#### Success Criteria:
- [ ] All standard commands migrated
- [ ] All tests passing
- [ ] No package-level flag variables remain

### Phase 10: Documentation & Cleanup (Week 6)

**Goal:** Clean up, document, and prepare for release

#### Tasks:

1. **Update CLAUDE.md**
   - Document unified parser pattern
   - Add examples
   - Update testing guidelines

2. **Create developer guide**
   - Already in `.scratch/flag-handling-dev-guide.md`
   - Move to `docs/`
   - Add real examples from migrated commands

3. **Update PRD**
   - Mark implementation complete
   - Document lessons learned

4. **Final testing**
   - Full integration test suite
   - Performance benchmarks
   - Coverage report (target 90%+)

5. **Code cleanup**
   - Remove all deprecated code
   - Update comments
   - Run linters

#### Success Criteria:
- [ ] Documentation complete
- [ ] All deprecated code removed
- [ ] 90%+ test coverage achieved
- [ ] All linters passing
- [ ] Ready for PR

## Testing Strategy

### Unit Tests (Target: 90%+ coverage)

- **FlagParser**: Every method, all edge cases
- **ConfigLoader**: Precedence order, file loading, env vars
- **Middleware**: Composition, error handling
- **PassThroughHandler**: Separator parsing, flag extraction

### Integration Tests

- **With mock component**: 100+ edge case scenarios from PRD
- **With TestKit**: Isolation and cleanup
- **With real commands**: Version, describe, terraform

### Regression Tests

- **Baseline tests**: Current behavior captured
- **No regressions**: All baseline tests pass after migration

### Performance Tests

- **Benchmark flag parsing**: Compare old vs new
- **No performance regression**: New system must be as fast or faster

## Success Metrics

- [ ] **Test Coverage**: 90%+ for `pkg/flagparser/`
- [ ] **Code Reduction**: Remove 500+ lines of manual parsing
- [ ] **Consistency**: All commands use unified system
- [ ] **No Regressions**: All baseline tests pass
- [ ] **Performance**: No regression in command execution time

## Risk Mitigation

### Risk: Breaking Existing Commands

**Mitigation:**
- Baseline tests before migration
- Phase-by-phase migration
- Each command tested independently
- Keep old code until new code fully tested

### Risk: Performance Regression

**Mitigation:**
- Benchmark tests
- Profile flag parsing
- Optimize if needed

### Risk: Complexity Increase

**Mitigation:**
- Clear interfaces
- Comprehensive documentation
- Code examples for every pattern
- Developer guide

## Timeline

- **Week 1**: Phase 1 (Core Infrastructure)
- **Week 2**: Phases 2-3 (Mock Integration, Viper)
- **Week 3**: Phases 4-5 (Middleware, Command Registry)
- **Week 4**: Phases 6-7 (Custom Commands, Baseline)
- **Week 5**: Phase 8 (Pass-Through Migration)
- **Week 6**: Phases 9-10 (Standard Commands, Docs)

**Total: 6 weeks to complete implementation**

## Next Steps

1. **Get approval** on this implementation plan
2. **Start Phase 1**: Create `pkg/flagparser/` package
3. **Build iteratively**: Complete each phase before moving to next
4. **Test exhaustively**: 90%+ coverage minimum
5. **Integrate carefully**: No regressions

## Notes

- This plan follows the user's requirement: "Build standalone, test exhaustively FIRST, then integrate"
- No production commands are touched until Phases 1-7 are complete
- Baseline tests ensure no regressions
- Interface-driven design enables testing without real dependencies
