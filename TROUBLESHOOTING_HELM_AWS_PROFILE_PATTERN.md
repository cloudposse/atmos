# Troubleshooting: helm_aws_profile_pattern Still Being Used After Commenting Out

## Issue

Users report that even after commenting out `helm_aws_profile_pattern` from their `atmos.yaml`, they still see:
- Deprecation warning: "helm_aws_profile_pattern is deprecated, use --identity flag instead"
- Error: "The config profile (...) could not be found"

## Root Cause Analysis

The `helm_aws_profile_pattern` configuration value can come from multiple sources, in this order of precedence:

### 1. Environment Variable (Highest Priority)
```bash
export ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN="{namespace}--gbl-{stage}-helm"
```

**Check:** Run `echo $ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN` to see if it's set.

### 2. atmos.yaml Configuration File
```yaml
components:
  helmfile:
    helm_aws_profile_pattern: "{namespace}-{tenant}-gbl-{stage}-helm"
```

**Check:** Search for the pattern in your config:
```bash
grep -r "helm_aws_profile_pattern" . --include="*.yaml"
```

Note: You might have multiple `atmos.yaml` files:
- Project root: `./atmos.yaml`
- Home directory: `~/.atmos/atmos.yaml`
- System: `/usr/local/etc/atmos/atmos.yaml`

### 3. Default Value in Code
The default value in `pkg/config/default.go` is **empty string** (`""`), which correctly falls back to ambient AWS credentials.

## How to Fix

### Option 1: Remove All Instances (Recommended)
1. Remove or comment out `helm_aws_profile_pattern` from ALL `atmos.yaml` files
2. Unset the environment variable:
   ```bash
   unset ATMOS_COMPONENTS_HELMFILE_HELM_AWS_PROFILE_PATTERN
   ```
3. Use ambient AWS credentials (env vars, instance profiles, etc.) or the `--identity` flag

### Option 2: Use --identity Flag
The `--identity` flag takes precedence over `helm_aws_profile_pattern`:

```bash
atmos helmfile diff my-component -s my-stack --identity=my-identity
```

### Option 3: Explicitly Disable Identity Auth
To use the profile pattern instead of identity auth:

```bash
atmos helmfile diff my-component -s my-stack --identity=false
```

This will fall back to:
1. `helm_aws_profile_pattern` if set
2. Ambient AWS credentials if pattern is not set

## Verification

After making changes, verify the configuration is correct:

```bash
# Check what atmos.yaml file is being used
atmos describe config

# Check if the pattern is set
atmos describe config | grep helm_aws_profile_pattern

# Check environment variables
env | grep ATMOS_COMPONENTS_HELMFILE
```

## Expected Behavior

When `helm_aws_profile_pattern` is NOT set (empty):
- ✅ No deprecation warning
- ✅ Falls back to ambient AWS credentials
- ✅ `aws eks update-kubeconfig` runs without `--profile` flag

When `helm_aws_profile_pattern` IS set:
- ⚠️  Deprecation warning shown
- 📋 Profile name computed from pattern
- 🔑 `aws eks update-kubeconfig --profile=<computed-profile>` is used

## Testing the Fix

Run your helmfile command with increased logging:

```bash
ATMOS_LOGS_LEVEL=Debug atmos helmfile diff my-component -s my-stack
```

Look for these log lines:
- "Using AWS auth" - shows the source (identity/pattern/ambient)
- "helm_aws_profile_pattern is deprecated" - only appears if pattern is being used
