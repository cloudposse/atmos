# Fix: terraform.output YAML function preserves newlines (DEV-2982)

## what
- Fixed the `terraform.output` YAML function to properly preserve newlines in multiline string outputs
- Modified the terraform output retrieval logic to bypass yq processing for simple output values
- Updated YAML processing utilities to preserve intentional whitespace in custom tag values
- Added comprehensive tests to verify newline preservation works correctly

## why
- The `terraform.output` function was stripping newlines from multiline strings, converting them to spaces
- This made it impossible to use terraform outputs that contain formatted text, certificates, or other multiline content
- The issue was caused by yq expression evaluation treating multiline strings as flow scalars, which fold newlines into spaces
- Users reported this breaks workflows that depend on preserving the exact format of terraform outputs

## Changes Made

### Core Fix: `internal/exec/terraform_output_utils.go`
- Modified `getTerraformOutputVariable` to directly return simple output values without yq processing
- This preserves the original formatting including all newlines and whitespace
- Complex path queries (dot notation) still use yq for backward compatibility

### Supporting Changes: `pkg/utils/yaml_utils.go`
- Updated `getValueWithTag` to preserve whitespace in YAML node values
- Modified `processCustomTags` to avoid trimming values for custom tags
- These changes ensure the YAML processing pipeline doesn't strip intentional whitespace

### Test Coverage
- Added `yaml_func_terraform_output_newline_test.go`: Unit tests for newline preservation scenarios
- Added `yaml_utils_newline_test.go`: Tests for YAML processing with whitespace preservation
- Added `terraform_output_issue_reproduction_test.go`: Integration test reproducing the exact issue from DEV-2982
- All tests verify that multiline strings like `"bar\nbaz\nbongo\n"` are preserved exactly

## Testing
```bash
# Run the specific newline preservation tests
go test ./internal/exec -run TestYamlFuncTerraformOutputWithNewlines -v
go test ./pkg/utils -run TestProcessCustomTagsPreservesWhitespace -v
go test ./tests -run TestDEV2982TerraformOutputNewlineIssue -v

# Run broader test suite to verify no regressions
go test ./internal/exec -run "TestDescribeComponent|TestExecuteTerraform" -v
go test ./pkg/stack -run "TestStack" -v
```

All tests pass, confirming:
- Newlines are preserved in terraform outputs
- Existing functionality remains intact
- No regressions in stack processing or component handling

## Example
Before fix:
```yaml
# terraform output returns: "bar\nbaz\nbongo\n"
vars:
  foo: !terraform.output component-a foo

# Result: foo = "bar baz bongo"  # newlines converted to spaces
```

After fix:
```yaml
# terraform output returns: "bar\nbaz\nbongo\n"
vars:
  foo: !terraform.output component-a foo

# Result: foo = "bar\nbaz\nbongo\n"  # newlines preserved exactly
```

## references
- Closes LINEAR issue DEV-2982
- Related to terraform output handling in Atmos YAML functions
