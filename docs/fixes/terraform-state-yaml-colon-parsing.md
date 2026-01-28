# Fix: terraform.state YAML Tag Incorrectly Parses String Values Ending with Colons

**Date**: 2025-01-28

**GitHub Issue**: [#2031](https://github.com/cloudposse/atmos/issues/2031)

## Problem

The `!terraform.state` YAML function (and other YAML functions that use `EvaluateYqExpression`) incorrectly parses string values that end with colons (`:` or `::`). Instead of returning the string value, the function returns a map with the string as a key and `null` as the value.

### Example

Given a Terraform output:

```hcl
output "db_secret_arn_with_key" {
  value = "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::"
}
```

And this stack configuration:

```yaml
components:
  terraform:
    my-component:
      settings:
        map_secrets:
          DB_PASSWORD: !terraform.state rds/postgres db_secret_arn_with_key
```

**Expected result:**
```yaml
DB_PASSWORD: "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password::"
```

**Actual result (before fix):**
```yaml
DB_PASSWORD:
  "arn:aws:secretsmanager:us-east-2:123456789012:secret:my-secret-AbCdEf:password:": null
```

## Root Cause

The `EvaluateYqExpression` function in `pkg/utils/yq_utils.go` uses yq to evaluate expressions on YAML data. When yq returns a scalar value with `UnwrapScalar: true`, the result is an unquoted string. When this string is then parsed by `yaml.Unmarshal`, strings ending with colons are misinterpreted as YAML map syntax.

For example, the string `password::` is parsed by the YAML parser as:
```yaml
password:: null  # A map with key "password:" and null value
```

This is standard YAML behavior - a string ending with a colon followed by whitespace or end-of-input is interpreted as a map key.

## Solution

Added two helper functions to detect and handle this edge case:

1. **`isScalarString`**: Pre-checks if the yq result looks like a scalar string that should not be parsed as YAML. This catches strings that:
   - Start with `#` (would be stripped as comments)
   - Are empty
   - End with colons but don't contain `: ` pattern (likely ARNs or similar identifiers)

2. **`isMisinterpretedScalar`**: Post-checks if the YAML parser has misinterpreted a scalar string as a single-key map with a null value. If the parsed node is a mapping with one key-value pair where the value is null and the key matches the original string minus the trailing colon(s), it's likely a misinterpreted scalar.

The fix is applied in `EvaluateYqExpression` after yq returns its result and before parsing it as YAML. If either check passes, the original string is returned directly without YAML parsing.

## Files Changed

- `pkg/utils/yq_utils.go`: Added `isScalarString` and `isMisinterpretedScalar` helper functions; modified `EvaluateYqExpression` to use these checks
- `pkg/utils/yq_utils_test.go`: Added comprehensive tests for the new functions and regression tests for the bug

## Testing

New tests added:
- `TestIsScalarString`: Tests all edge cases for the pre-check function
- `TestIsMisinterpretedScalar`: Tests the post-check function
- `TestEvaluateYqExpression_StringEndingWithColon`: Integration tests for various string formats ending with colons, including AWS ARNs

## Impact

This fix affects all YAML functions that use `EvaluateYqExpression`:
- `!terraform.state`
- `!terraform.output`
- `!atmos.Component`
- Any other function using yq expression evaluation

String values ending with colons will now be correctly preserved as strings instead of being converted to maps.
