# Terraform Flag Support Analysis in Atmos

## Executive Summary

After thorough analysis of the codebase and testing, I've found that **Atmos has very limited support for native Terraform flags without the `--` separator**. While the `-var` flag has special handling and works without the separator, all other native Terraform flags require the double-dash separator. The documentation is misleading as it shows examples that don't accurately reflect the actual implementation.

**Key Discovery**: Terraform itself accepts both single-dash (`-var`) and double-dash (`--var`) formats for its flags, which is undocumented behavior. This explains why test cases using `--var=foo=bar` pass successfully when these arguments are passed through to Terraform.

## Key Findings

### 1. **Flag Parsing Architecture**

The Terraform commands in Atmos are configured with `DisableFlagParsing = true` (see `cmd/terraform.go:18` and `cmd/terraform_commands.go:299`), which means:
- Atmos bypasses Cobra's standard flag parsing for terraform commands
- All arguments are processed manually through the `processArgsAndFlags()` function
- The `--` separator is the only way to distinguish between Atmos flags and native Terraform flags

### 2. **Explicit Atmos-Only Flags (Double-Dash)**

These flags are explicitly handled by Atmos and work WITHOUT requiring `--`:

#### Parent Command Flags (all terraform subcommands):
- `--append-user-agent` - Sets TF_APPEND_USER_AGENT environment variable
- `--skip-init` - Skip running terraform init
- `--init-pass-vars` - Pass varfile to terraform init
- `--process-templates` - Enable/disable Go template processing
- `--process-functions` - Enable/disable YAML functions
- `--skip` - Skip specific YAML functions
- `--query` / `-q` - Execute on filtered components
- `--components` - Filter by specific components
- `--dry-run` - Simulate without changes
- `--affected` related flags:
  - `--repo-path`
  - `--ref`
  - `--sha`
  - `--ssh-key`
  - `--ssh-key-password`
  - `--include-dependents`
  - `--clone-target-ref`

#### Command-Specific Flags:
- **plan**: `--affected`, `--all`, `--skip-planfile`
- **apply**: `--from-plan`, `--planfile`, `--affected`, `--all`
- **deploy**: `--deploy-run-init`, `--from-plan`, `--planfile`, `--affected`, `--all`
- **clean**: `--everything`, `--force`, `--skip-lock-file`
- **plan-diff**: `--orig` (required), `--new`

### 3. **Special Case: The `-var` Flag**

**IMPORTANT EXCEPTION**: The `-var` flag (single-dash) has special handling in Atmos:

- **Works WITHOUT separator**: `-var name=value` (parsed by `getCliVars()` function in `internal/exec/cli_utils.go:649-675`)
- **Also works WITH separator**: `-- -var name=value` (passed directly to Terraform)
- **Processed internally**: Atmos extracts these variables for its own processing
- **Still passed to Terraform**: The flag is included in the final terraform command

**Verified Terraform Behavior**:
- Terraform accepts both `-var` (single-dash, documented) and `--var` (double-dash, undocumented)
- Both formats work identically in Terraform: `terraform plan -var="test=hello"` and `terraform plan --var="test=hello"`
- This appears to be GNU-style argument parsing where both single and double dashes are accepted

**Note about `--var` (double-dash)**:
- Test cases show `--var=foo=bar` being used and passing successfully
- **Terraform DOES accept `--var`** (double-dash) - this is an undocumented Terraform feature
- Both `-var` (single-dash, documented) and `--var` (double-dash, undocumented) work in Terraform
- Atmos passes `--var` through unchanged to Terraform when used after the `--` separator
- The `--var` format is NOT specially handled by Atmos like `-var` is - it requires the `--` separator

### 4. **Native Terraform Flags (Single-Dash)**

**Most native Terraform flags require the `--` separator**. This includes:

- `-var-file=filename`
- `-target=resource`
- `-destroy`
- `-refresh-only`
- `-parallelism=n`
- `-auto-approve`
- `-compact-warnings`
- `-detailed-exitcode`
- `-input=false`
- `-json`
- `-lock=false`
- `-lock-timeout=duration`
- `-no-color`
- `-refresh=false`
- `-replace=resource`
- `-upgrade`
- `-reconfigure`
- And all other native Terraform flags...

### 5. **How Atmos Uses CLI Variables**

The variables extracted from `-var` flags are used for:
1. **Template Rendering**: Available in Go template expressions
2. **Stack Processing**: Can override stack configuration values
3. **OPA Policy Validation**: Variables are available to OPA policies for validation
4. **Variable Precedence**: CLI vars have highest precedence, overriding varfiles and environment variables

This is stored in `configAndStacksInfo.ComponentSection[cfg.TerraformCliVarsSectionName]` and used throughout the stack processing pipeline.

### 6. **Flag Parsing Logic: How Atmos Distinguishes Flags**

Atmos uses a multi-step process to parse and route flags:

1. **Pre-processing Split**: Arguments are split at `--` position (`cmd_utils.go:8-11`)
   - Everything before `--` goes through Atmos processing
   - Everything after `--` is passed directly to Terraform

2. **Manual Flag Processing** (`processArgsAndFlags()`):
   - Known Atmos flags (starting with `--`) are explicitly handled
   - The `-var` flag is specially parsed even though it's a Terraform flag
   - Unrecognized flags starting with `--` are treated as additional arguments
   - Single-dash flags (except `-var`) are passed through

3. **Variable Extraction** (`getCliVars()`):
   - Specifically looks for `-var` flags in the arguments
   - Extracts key-value pairs for Atmos's own variable processing
   - These variables are used in template rendering and stack processing

4. **Final Assembly**:
   - Atmos flags are processed and removed from the command
   - Remaining arguments (including `-var`) are passed to Terraform
   - Arguments after `--` are appended unchanged

### 6. **Why the Separator is (Mostly) Required**

The root cause is a **fundamental conflict** between flag styles:

1. **Terraform uses single-dash flags**: `-var`, `-target`, `-destroy`
2. **Cobra (used by Atmos) expects double-dash flags**: `--var`, `--target`, `--destroy`
3. **Manual argument processing**: With `DisableFlagParsing = true`, Atmos manually processes arguments
4. **Pass-through logic**: Everything after `--` is passed directly to Terraform (see `internal/exec/terraform.go:429`)

Evidence from the codebase:
- `cmd_utils.go:8-11`: Splits arguments at `--` position
- `cli_utils.go:629-640`: Anything starting with `--` (but not an Atmos flag) gets added to AdditionalArgsAndFlags
- `terraform.go:429`: `allArgsAndFlags = append(allArgsAndFlags, info.AdditionalArgsAndFlags...)`

### 7. **Documentation vs Implementation Gap**

The documentation (e.g., `website/docs/cli/commands/terraform/terraform-plan.mdx`) shows examples like:

```shell
# Documentation shows (and this DOES work for -var only):
atmos terraform plan vpc -s dev -var="instance_count=3"

# But for other flags, you need:
atmos terraform plan vpc -s dev -- -destroy
atmos terraform plan vpc -s dev -- -target=aws_instance.example
```

The documentation is partially correct for `-var` but misleading for other native flags.

### 8. **Special Cases**

Some flags ARE handled automatically by Atmos without user intervention:

1. **Auto-added by Atmos**:
   - `-var-file` (automatically added with generated varfile)
   - `-out` (for plan commands, unless `--skip-planfile`)
   - `-reconfigure` (if configured in atmos.yaml)
   - `-auto-approve` (for deploy command)

2. **Environment Variable Translation**:
   - `--append-user-agent` → `TF_APPEND_USER_AGENT`

### 9. **Code Comment Acknowledgment**

The codebase itself acknowledges this needs modernization:
- `internal/exec/cli_utils.go:147-149`: "Deprecated: use Cobra command flag parser instead"
- Reference to PR #1174 for better flag handling

## Examples

### ✅ What Works Without `--`:
```bash
# Atmos-specific flags work directly
atmos terraform plan myapp -s dev --skip-init
atmos terraform apply myapp -s dev --from-plan --planfile=my.plan
atmos terraform plan --affected --include-dependents
atmos terraform clean myapp -s dev --everything --force

# Special case: -var flag works without separator
atmos terraform plan myapp -s dev -var name=value -var region=us-west-2
atmos terraform apply myapp -s dev -var 'tags={"env":"prod"}'
```

### ❌ What Requires `--` Separator:
```bash
# Most native Terraform flags need the separator
atmos terraform plan myapp -s dev -- -parallelism=10
atmos terraform apply myapp -s dev -- -auto-approve -var-file=extra.tfvars
atmos terraform init myapp -s dev -- -upgrade -reconfigure
atmos terraform plan myapp -s dev -- -destroy -target=aws_instance.example

# The --var format (double-dash) also requires the separator
atmos terraform plan myapp -s dev -- --var="test=value"
```

## Implementation Recommendations

### Short-term Improvements (Documentation & Clarity)

1. **Update Documentation Immediately**
   - Clearly state that `-var` works without the separator (special case)
   - Document that ALL other native Terraform flags require `--`
   - Update examples to show both patterns correctly
   - Add a "Flag Usage" section to the Terraform command docs

2. **Enhance Error Messages**
   - When users try to use native flags without `--`, provide helpful error messages
   - Example: "The flag '-destroy' appears to be a Terraform flag. Please use: atmos terraform plan vpc -s dev -- -destroy"
   - Leverage the existing `doubleDashHint` more prominently

3. **Add Flag Usage Table**
   Create a reference table in documentation:
   ```
   | Flag Type | Example | Requires `--`? |
   |-----------|---------|---------------|
   | Atmos flags | --skip-init, --affected | No |
   | -var flag | -var name=value | No (special) |
   | Other TF flags | -destroy, -target | Yes |
   ```

### Medium-term Improvements (Code Enhancements)

1. **Extend Special Handling to Common Flags**
   Following the `-var` pattern, consider special handling for:
   - `-target` (frequently used for selective updates)
   - `-destroy` (common for teardown operations)
   - `-auto-approve` (CI/CD workflows)
   - `-var-file` (similar to -var usage)

2. **Implement Smart Flag Detection**
   ```go
   // Detect known Terraform flags and provide helpful guidance
   knownTerraformFlags := []string{"-destroy", "-target", "-auto-approve", ...}
   if isKnownTerraformFlag(flag) && !hasDoubleDash {
       return fmt.Errorf("Terraform flag '%s' detected. Use: -- %s", flag, flag)
   }
   ```

3. **Create Flag Router Interface**
   ```go
   type FlagRouter interface {
       IsAtmosFlag(flag string) bool
       IsTerraformFlag(flag string) bool
       RequiresSeparator(flag string) bool
       Process(flag string, value string) error
   }
   ```

### Long-term Improvements (Architecture)

1. **Migrate to Modern Cobra Flag Handling**
   - Remove `DisableFlagParsing = true`
   - Use Cobra's `FParseErrWhitelist.UnknownFlags`
   - Implement custom flag parsing that understands both styles
   - Reference PR #1174 mentioned in comments

2. **Implement Flag Translation Layer**
   ```go
   // Automatically translate between flag styles
   type FlagTranslator struct {
       terraformFlags map[string]FlagConfig
       atmosFlags     map[string]FlagConfig
   }

   func (ft *FlagTranslator) Translate(args []string) (atmosArgs, tfArgs []string) {
       // Smart routing based on flag recognition
   }
   ```

3. **Support Both Flag Styles**
   - Accept both `-var` and `--var` for common flags
   - Internally normalize to appropriate format
   - Maintain backward compatibility

### Testing Recommendations

1. **Add Comprehensive Flag Tests**
   ```go
   func TestFlagParsing(t *testing.T) {
       tests := []struct {
           name     string
           args     []string
           wantAtmos []string
           wantTF    []string
       }{
           {
               name: "var flag without separator",
               args: []string{"plan", "vpc", "-s", "dev", "-var", "name=value"},
               wantTF: []string{"-var", "name=value"},
           },
           {
               name: "destroy flag requires separator",
               args: []string{"plan", "vpc", "-s", "dev", "--", "-destroy"},
               wantTF: []string{"-destroy"},
           },
       }
   }
   ```

2. **Create Integration Tests**
   - Test actual Terraform command execution with various flag combinations
   - Verify that flags are correctly passed through
   - Ensure variable precedence is maintained

### Priority Order

1. **Immediate**: Fix documentation (1-2 days)
2. **Next Sprint**: Implement better error messages and flag detection (1 week)
3. **Future Release**: Extend special handling to more flags (2-3 weeks)
4. **Major Version**: Architectural improvements with Cobra modernization (1-2 months)

## Conclusion

The implementation **requires the `--` separator for MOST native Terraform flags**, with the notable exception of `-var` which has special handling. This hybrid approach creates some confusion:

1. **`-var` works both ways**: Can be used with or without the `--` separator
2. **All other native flags**: Require the `--` separator
3. **Documentation needs clarification**: Should explicitly state which flags work without the separator
4. **Terraform's undocumented behavior**: Terraform accepts both single and double-dash formats for its flags (e.g., both `-var` and `--var` work), though only single-dash is documented

The special handling of `-var` suggests that Atmos could potentially extend this pattern to other commonly-used Terraform flags, but currently it's an exception rather than the rule. The current implementation provides a mostly clean separation between Atmos and Terraform flags, with `-var` being a pragmatic exception due to its frequent use.

**Important Note**: While Terraform accepts `--var` (double-dash), Atmos only provides special handling for `-var` (single-dash). Users wanting to use `--var` must include the `--` separator: `atmos terraform plan myapp -s dev -- --var="test=value"`
