# Secrets Masking Example

This example demonstrates Atmos's automatic secrets masking feature.

## Overview

Atmos automatically masks sensitive values in terminal output to prevent accidental exposure of secrets. The masking system includes:

- **Built-in patterns**: AWS keys, API tokens, GitHub tokens, and 120+ patterns from the Gitleaks library
- **Custom patterns**: User-defined regex patterns in `atmos.yaml`
- **Custom literals**: Exact string matches for known secret values

## Configuration

See `atmos.yaml` for the masking configuration:

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "[REDACTED]"
      patterns:
        - 'demo-key-[A-Za-z0-9]{16}'
      literals:
        - "super-secret-demo-value"
```

## Testing the Feature

1. **Run a terraform plan with secrets in output**:
   ```bash
   cd examples/secrets-masking
   atmos terraform plan secrets-demo -s demo-dev-test
   ```

2. **Verify masking in terraform output**:
   The component outputs secrets which will be masked as `[REDACTED]` in the output.

3. **Disable masking to compare**:
   ```bash
   atmos terraform plan secrets-demo -s demo-dev-test --mask=false
   ```

## What Gets Masked

1. **Built-in patterns** (always active):
   - AWS Access Key IDs (`AKIA...`)
   - AWS Secret Access Keys
   - GitHub tokens (`ghp_...`, `gho_...`, `ghu_...`)
   - Generic API keys and passwords
   - JWT tokens
   - Private keys

2. **Custom patterns** (from `atmos.yaml`):
   - `demo-key-XXXX...` format
   - `internal-XXXX...` format
   - `tkn_live_...` and `tkn_test_...` tokens

3. **Custom literals** (from `atmos.yaml`):
   - `super-secret-demo-value`
   - `my-api-key-12345`

## Masking Coverage

Secrets are masked across all output channels:
- Terraform/Helmfile command output (stdout/stderr)
- Custom command output
- Atmos logs
- Error messages
- Documentation display

## Disabling Masking

To disable masking for debugging (not recommended in CI/CD):

```bash
# Via command-line flag
atmos terraform plan component -s stack --mask=false

# Via environment variable
export ATMOS_TERMINAL_MASK_ENABLED=false
```
