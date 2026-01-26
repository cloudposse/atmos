# YAML Functions YQ Expression Fixes

**Date:** 2025-01-25

## Summary

Fixed documentation inconsistencies in YAML functions for `!terraform.state`, `!terraform.output`, and related
functions.

## Changes

### Documentation Fixes

| File                                               | Fix                                                                    |
|----------------------------------------------------|------------------------------------------------------------------------|
| `website/docs/functions/yaml/terraform.state.mdx`  | Removed incorrect `.outputs.` prefix from bracket notation examples    |
| `website/docs/functions/yaml/terraform.output.mdx` | Fixed confusing component/output name collision in examples            |
| `website/docs/functions/index.mdx`                 | Fixed syntax: `vpc.vpc_id` → `vpc vpc_id` (space-separated parameters) |

### Test Coverage Added

Added bracket notation tests for map keys containing special characters (slashes, hyphens):

- `tests/fixtures/components/terraform/mock/main.tf` - Added `secret_arns_map` output
- `tests/fixtures/scenarios/atmos-terraform-output-yaml-function/stacks/deploy/nonprod.yaml` - Added
  `component-bracket-notation`
- `tests/fixtures/scenarios/atmos-terraform-state-yaml-function/stacks/deploy/nonprod.yaml` - Added
  `component-bracket-notation`

## User-Reported Issue: Bracket Notation with Slashes

A user reported a YAML parsing error with bracket notation containing forward slashes. Investigation confirmed the
syntax is **correct and works**:

```yaml
# All valid syntax forms:
client_id_arn: !terraform.output secrets-manager/auth0 '.secret_arns_map["auth0/app/client-id"]'
client_id_arn: !terraform.output secrets-manager/auth0 .secret_arns_map["auth0/app/client-id"]
client_id_arn: !terraform.output secrets-manager/auth0 {{ .stack }} '.secret_arns_map["auth0/app/client-id"]'
```

## Test Coverage Added

New test files created to increase YAML function coverage:

| File                                             | Tests                                                                    |
|--------------------------------------------------|--------------------------------------------------------------------------|
| `internal/exec/yaml_func_yq_expressions_test.go` | YQ expression patterns, bracket notation, default values, pipe operators |
| `internal/exec/yaml_func_env_test.go`            | `!env` function unit tests with defaults, lists, error cases             |
| `tests/yaml_functions_include_test.go`           | `!include` integration tests for JSON, YAML, tfvars, remote URLs         |

## Verification

```bash
# Run all YAML function tests
go test ./internal/exec -run "TestYQExpression\|TestBracketNotation\|TestProcessTagEnv\|TestEnvFunction" -v
go test ./tests -run "TestYAMLFunction" -v
```
