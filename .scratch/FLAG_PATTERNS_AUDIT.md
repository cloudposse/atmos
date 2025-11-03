# Flag Patterns Audit - Current State

## Summary

**Commands using builder pattern:** 23
**Commands using old parser pattern:** 3 (terraform, helmfile, packer)
**Commands using direct Cobra flags:** 2 (auth_console, validate_editorconfig)
**Commands needing migration:** 2

---

## ✅ Commands Using Builder Pattern (23)

### StandardOptionsBuilder (22 commands)
1. auth_env.go
2. auth_list.go
3. auth_logout.go
4. auth_validate.go
5. auth_whoami.go
6. describe_component.go
7. describe_config.go
8. describe_dependents.go
9. describe_stacks.go
10. describe_workflows.go
11. helmfile_generate_varfile.go
12. list_components.go
13. list_values.go
14. list_vars.go
15. list_vendor.go
16. list_workflows.go
17. terraform_generate_varfile.go
18. validate_component.go
19. validate_schema.go
20. validate_stacks.go
21. vendor_diff.go
22. vendor_pull.go

### WorkflowOptionsBuilder (1 command)
23. workflow.go

---

## 🔧 Commands Using Old Parser Pattern (3)

These use custom parsers (TerraformParser, HelmfileParser, PackerParser) that predate the builder pattern:

1. **terraform_commands.go** - `flags.NewTerraformParser()`
   - Pattern: Pass-through args to terraform binary
   - Status: ✅ Working correctly, uses TerraformOptions
   - Note: Already migrated to strongly-typed options

2. **helmfile.go** - `flags.NewHelmfileParser()`
   - Pattern: Pass-through args to helmfile binary
   - Status: ✅ Working correctly, uses HelmfileOptions
   - Note: Already migrated to strongly-typed options

3. **packer.go** - `flags.NewPackerParser()`
   - Pattern: Pass-through args to packer binary
   - Status: ✅ Working correctly, uses PackerOptions
   - Note: Already migrated to strongly-typed options

**Recommendation:** These are fine as-is. They use the correct strongly-typed options pattern.

---

## 🚫 Commands Using Direct Cobra Flags (2)

These directly call `cmd.Flags().StringVar()` etc. instead of using builders:

1. **auth_console.go**
   - Flags: destination, duration, issuer, print-only, no-open
   - Status: ⚠️ Needs migration to AuthOptionsBuilder
   - Complexity: Medium - 5 console-specific flags

2. **validate_editorconfig.go**
   - Flags: 12+ flags (exclude, init, ignore-defaults, dry-run, format, disable-*)
   - Status: ⚠️ Needs migration OR custom builder
   - Complexity: High - complex precedence logic with atmos.yaml

**Recommendation:** Migrate to AuthOptionsBuilder (for auth_console) and consider custom EditorConfigOptionsBuilder (for validate_editorconfig).

---

## 📊 Pattern Coverage

| Pattern | Count | Status |
|---------|-------|--------|
| StandardOptionsBuilder | 22 | ✅ Active |
| WorkflowOptionsBuilder | 1 | ✅ Active |
| AuthOptionsBuilder | 0 | 🚧 Ready (just created) |
| TerraformParser | 1 | ✅ Legacy but correct |
| HelmfileParser | 1 | ✅ Legacy but correct |
| PackerParser | 1 | ✅ Legacy but correct |
| Direct Cobra flags | 2 | ⚠️ Needs migration |

**Builder Pattern Coverage:** 23/28 = 82%

---

## 🎯 Migration Plan

### Priority 1: Migrate to AuthOptionsBuilder
- **auth_console.go** - Use `WithDestination()`, `WithDuration()`, `WithIssuer()`, `WithPrintOnly()`, `WithNoOpen()`
- **auth_validate.go** - Already using StandardOptionsBuilder ✅
- **auth_whoami.go** - Already using StandardOptionsBuilder ✅

### Priority 2: Consider EditorConfigOptionsBuilder
- **validate_editorconfig.go** - Complex command with 12+ flags
  - Option A: Create `EditorConfigOptionsBuilder` with custom flags
  - Option B: Keep as-is (edge case with external library integration)
  - Recommendation: Option B - this command wraps an external library

### Priority 3: Optional - Convert old parsers to builders
- **TerraformParser** → Could create `TerraformOptionsBuilder`
- **HelmfileParser** → Could create `HelmfileOptionsBuilder`
- **PackerParser** → Could create `PackerOptionsBuilder`
- Note: Low priority since these already use strongly-typed options

---

## 🏗️ Builder Pattern Architecture

All builders follow this pattern:

```go
// 1. Options struct (strongly-typed)
type XOptions struct {
    GlobalFlags
    Field1 string
    Field2 bool
}

// 2. Builder (fluent interface)
type XOptionsBuilder struct {
    options []Option
}

func NewXOptionsBuilder() *XOptionsBuilder { ... }
func (b *XOptionsBuilder) WithField1() *XOptionsBuilder { ... }
func (b *XOptionsBuilder) Build() *XParser { ... }

// 3. Parser (uses StandardFlagParser internally)
type XParser struct {
    parser *StandardFlagParser
}

func (p *XParser) RegisterFlags(cmd *cobra.Command) { ... }
func (p *XParser) BindToViper(v *viper.Viper) error { ... }
func (p *XParser) Parse(ctx, args) (*XOptions, error) { ... }
```

Existing builders:
- ✅ StandardOptionsBuilder → StandardParser → StandardOptions
- ✅ WorkflowOptionsBuilder → WorkflowParser → WorkflowOptions
- 🆕 AuthOptionsBuilder → AuthParser → AuthOptions

---

## ✅ Conclusion

The builder pattern migration is **82% complete** (23/28 commands). Only 2 commands need migration:
1. auth_console.go (high priority - use AuthOptionsBuilder)
2. validate_editorconfig.go (low priority - edge case)

The codebase is in excellent shape with consistent, strongly-typed flag handling across almost all commands.
