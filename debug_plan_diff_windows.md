# Debugging Plan-Diff Failure on Windows

## Problem
The `TestMainTerraformPlanDiffIntegration` test passes on Mac/Linux but fails on Windows specifically for the on-the-fly plan generation case.

## Expected Behavior
When running:
```bash
atmos terraform plan-diff component-1 -s nonprod --orig=orig.plan -var foo=new-value
```

The on-the-fly generated plan should have `foo=new-value`, and plan-diff should detect differences and return exit code 2.

## Actual Behavior on Windows
- Exit code: 0 (no differences detected)
- This means both plans have `foo=component-1-a` (the value from the stack, not the command-line override)

## Investigation Findings

### 1. Argument Filtering ✓
- Created tests to verify `-var` flags are correctly preserved through `filterPlanDiffFlags`
- Tests pass - the filtering logic is correct

### 2. Terraform Command Construction ✓
- The command is constructed as: `terraform plan -var-file nonprod-component-1.terraform.tfvars.json -out /tmp/new.plan -var foo=new-value`
- According to Terraform docs, `-var` flags have higher precedence than `-var-file`
- Order doesn't matter for precedence

### 3. File Generation
- The tfvars file is regenerated during on-the-fly plan generation (line 210 in terraform.go)
- This file contains the original stack values: `{"foo": "component-1-a", ...}`
- The `-var` flag should override this, but on Windows it doesn't

## Hypotheses

### Most Likely: Windows File Locking
-  Windows has stricter file locking than Unix
- The tfvars file might still be locked when Terraform tries to read it
- This could cause Terraform to skip the file or read a cached version

### Possible: Command Line Encoding
- Windows uses different character encoding (UTF-16) vs Unix (UTF-8)
- Special characters in arguments might be interpreted differently

### Unlikely: Terraform Version Difference
- Windows CI might use a different Terraform version
- Need to verify same version across platforms

## Reproduction Steps

To reproduce locally on Windows:

```bash
# 1. Navigate to test fixture
cd tests/fixtures/scenarios/plan-diff

# 2. Generate original plan
atmos terraform plan component-1 -s nonprod -out=orig.plan

# 3. Run plan-diff with -var flag (on-the-fly generation)
atmos terraform plan-diff component-1 -s nonprod --orig=orig.plan -var foo=new-value

# Expected: Exit code 2 with diff showing foo change
# Actual on Windows: Exit code 0 (no diff)
```

## Debug Commands

Add logging to see actual terraform command:
```go
// In internal/exec/shell_utils.go, before cmd.Run():
fmt.Fprintf(os.Stderr, "DEBUG: Running command: %s %v\n", command, args)
```

## Next Steps

1. Add debug logging to print the exact terraform command being run on Windows
2. Verify tfvars file contents after generation
3. Check if file is locked/flushed before terraform reads it
4. Consider adding explicit file sync/flush after tfvars write on Windows
5. Test with explicit `-var-file` ordering (move `-var` before `-var-file`)
