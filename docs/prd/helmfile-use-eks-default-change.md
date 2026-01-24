# Migration: Helmfile `use_eks` Default Changed from `true` to `false`

## Summary

The `use_eks` setting in Atmos helmfile configuration now defaults to `false` instead of `true`. This is a **breaking change** for users who rely on EKS integration without explicitly enabling it.

**Affected Symbol:** `UseEKS` in `pkg/config/default.go:58`

**PR:** [#1903 - Modernize Helmfile EKS integration](https://github.com/cloudposse/atmos/pull/1903)

## Why EKS is Now Opt-In

### Before (Legacy Behavior)

Previously, `use_eks` defaulted to `true`, meaning:
- Atmos always attempted to update kubeconfig for EKS clusters before running helmfile commands
- Required AWS authentication even for non-EKS Kubernetes clusters
- Users of GKE, AKS, k3s, or local clusters had to explicitly disable EKS integration
- Caused confusion and failures when AWS credentials were not available

```yaml
# Before: use_eks was true by default (implicit)
components:
  helmfile:
    base_path: "components/helmfile"
    # use_eks: true  # This was the implicit default
```

### After (New Behavior)

Now, `use_eks` defaults to `false`:
- Atmos assumes you have a working kubeconfig and uses it directly
- EKS-specific authentication is opt-in
- Works immediately with any Kubernetes cluster (GKE, AKS, k3s, minikube, etc.)
- No AWS credentials required unless you explicitly enable EKS integration

```yaml
# After: use_eks defaults to false
# You must explicitly enable it for EKS clusters
components:
  helmfile:
    base_path: "components/helmfile"
    use_eks: true  # Now required for EKS users
```

### Rationale

1. **Broader Kubernetes Support**: Most helmfile users are not exclusively using EKS. The default should work for all Kubernetes clusters.

2. **Principle of Least Surprise**: Requiring AWS credentials by default is surprising for users who just want to run `helmfile apply` with their existing kubeconfig.

3. **Explicit Configuration**: EKS integration involves specific AWS API calls (`eks:DescribeCluster`, `eks:ListClusters`). This should be an explicit choice, not a hidden default.

4. **Error Reduction**: Eliminated timeout errors and authentication failures for non-EKS users who didn't know they needed to disable EKS integration.

## How to Restore Prior Behavior

### Option 1: Explicit Configuration (Recommended)

Add `use_eks: true` to your `atmos.yaml`:

```yaml
components:
  helmfile:
    use_eks: true
    kubeconfig_path: /dev/shm  # Recommended for EKS
    cluster_name_template: "{{ .vars.namespace }}-{{ .vars.environment }}-{{ .vars.stage }}-eks-cluster"
```

### Option 2: Environment Variable

Set the environment variable before running Atmos commands:

```bash
export ATMOS_COMPONENTS_HELMFILE_USE_EKS=true
atmos helmfile apply my-component -s prod
```

This is useful for CI/CD pipelines where you want to enable EKS integration without modifying configuration files.

### Option 3: Per-Stack Override

Override the setting in specific stacks:

```yaml
# stacks/prod.yaml
import:
  - catalog/defaults

vars:
  stage: prod

components:
  helmfile:
    my-component:
      settings:
        # Stack-level override (if supported by your component)
```

Note: Stack-level settings depend on component implementation.

## Impact on Existing Configurations

### Configurations That Will Break

| Scenario | Before | After | Fix |
|----------|--------|-------|-----|
| EKS cluster, no explicit `use_eks` | Works (implicit true) | Fails (kubeconfig not updated) | Add `use_eks: true` |
| EKS with `helm_aws_profile_pattern` | Works | Deprecated warning, still works | Migrate to `--identity` flag |
| EKS with `cluster_name_pattern` | Works | Deprecated warning, still works | Migrate to `cluster_name_template` |

### Configurations That Will Continue Working

| Scenario | Notes |
|----------|-------|
| Explicit `use_eks: true` | No change needed |
| Non-EKS clusters (GKE, AKS, k3s) | Works better now (no EKS auth attempts) |
| Explicit `use_eks: false` | No change needed |
| Using `--identity` flag | Works with explicit `use_eks: true` |

### Deprecation Warnings

When using deprecated options, Atmos will log warnings but continue to work:

```
WARN: helm_aws_profile_pattern is deprecated. Use --identity flag for AWS authentication.
WARN: cluster_name_pattern is deprecated. Use cluster_name_template with Go template syntax.
```

These deprecated options will be removed in a future major release.

## Upgrade Steps

### Step 1: Identify Affected Configurations

Search your `atmos.yaml` and stack files for helmfile configuration:

```bash
# Find atmos.yaml files
find . -name "atmos.yaml" -exec grep -l "helmfile" {} \;

# Check for implicit EKS usage (no explicit use_eks)
grep -r "components:" --include="atmos.yaml" | head -5
```

### Step 2: Audit Current Behavior

Before upgrading, verify your current helmfile commands work:

```bash
# Test current behavior
atmos helmfile diff my-component -s prod
```

### Step 3: Add Explicit Configuration

For EKS users, add the required configuration:

```yaml
# atmos.yaml
components:
  helmfile:
    use_eks: true
    kubeconfig_path: /dev/shm
```

### Step 4: Migrate Deprecated Options (Optional but Recommended)

#### Migrate `helm_aws_profile_pattern` to `--identity`

Before:
```yaml
components:
  helmfile:
    helm_aws_profile_pattern: "{namespace}-{tenant}-gbl-{stage}-helm"
```

After:
```yaml
# Remove helm_aws_profile_pattern from config
components:
  helmfile:
    use_eks: true
```

Command:
```bash
atmos helmfile apply my-component -s prod --identity=prod-admin
```

#### Migrate `cluster_name_pattern` to `cluster_name_template`

Before:
```yaml
components:
  helmfile:
    cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-eks-cluster"
```

After:
```yaml
components:
  helmfile:
    cluster_name_template: "{{ .vars.namespace }}-{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}-eks-cluster"
```

### Step 5: Test After Upgrade

After upgrading Atmos, verify your helmfile commands still work:

```bash
# Test with explicit use_eks
atmos helmfile diff my-component -s prod --identity=prod-admin

# Verify kubeconfig is updated
kubectl config view --context=$(kubectl config current-context)
```

### Step 6: Update CI/CD Pipelines

If using environment variables, add:

```yaml
# GitHub Actions example
env:
  ATMOS_COMPONENTS_HELMFILE_USE_EKS: "true"
```

Or update your pipeline scripts to use the `--identity` flag:

```bash
atmos helmfile apply my-component -s prod --identity=ci-automation
```

## Complete Configuration Example

### Before (Legacy)

```yaml
# atmos.yaml (before)
components:
  helmfile:
    base_path: "components/helmfile"
    # use_eks: true  # implicit default
    helm_aws_profile_pattern: "{namespace}-{tenant}-gbl-{stage}-helm"
    cluster_name_pattern: "{namespace}-{tenant}-{environment}-{stage}-eks-cluster"
```

### After (Recommended)

```yaml
# atmos.yaml (after)
components:
  helmfile:
    base_path: "components/helmfile"
    use_eks: true                    # Now explicit
    kubeconfig_path: /dev/shm        # Recommended for EKS
    cluster_name_template: "{{ .vars.namespace }}-{{ .vars.tenant }}-{{ .vars.environment }}-{{ .vars.stage }}-eks-cluster"
```

Command with identity:
```bash
atmos helmfile apply my-component -s prod --identity=prod-admin
```

## Cluster Name Precedence

When `use_eks: true`, the cluster name is resolved in this order (highest to lowest priority):

1. **`--cluster-name` flag**: Runtime override
2. **`cluster_name` config**: Explicit static cluster name
3. **`cluster_name_template`**: Go template with variables
4. **`cluster_name_pattern`**: Deprecated token-based pattern

Example:
```bash
# Override cluster name at runtime
atmos helmfile apply my-component -s prod --cluster-name=my-custom-cluster
```

## Technical Details

### Default Configuration Change

**File:** `pkg/config/default.go`

```go
// Line 58
var defaultCliConfig = schema.AtmosConfiguration{
    Components: schema.Components{
        Helmfile: schema.Helmfile{
            // Changed from true to false
            UseEKS: false, // Changed from true to false - EKS is now opt-in.
        },
    },
}
```

### Environment Variable Mapping

| Config Path | Environment Variable |
|-------------|---------------------|
| `components.helmfile.use_eks` | `ATMOS_COMPONENTS_HELMFILE_USE_EKS` |
| `components.helmfile.kubeconfig_path` | `ATMOS_COMPONENTS_HELMFILE_KUBECONFIG_PATH` |
| `components.helmfile.cluster_name` | `ATMOS_COMPONENTS_HELMFILE_CLUSTER_NAME` |

## Related Documentation

- **Blog Post:** [Modernizing Helmfile EKS Integration](https://atmos.tools/changelog/helmfile-eks-modernization)
- **Helmfile Configuration:** [Helmfile Configuration](https://atmos.tools/cli/configuration/components/helmfile)
- **Identity System:** [Identity System](https://atmos.tools/stacks/auth)
- **PR #1903:** https://github.com/cloudposse/atmos/pull/1903

## FAQ

### Q: My helmfile commands stopped working after upgrade. What happened?

A: If you were using EKS clusters without explicit `use_eks: true`, Atmos no longer updates your kubeconfig automatically. Add `use_eks: true` to your configuration.

### Q: Do I need to change anything if I'm using GKE/AKS/k3s?

A: No. The change actually improves your experience by not attempting EKS authentication.

### Q: Can I still use `helm_aws_profile_pattern`?

A: Yes, but it's deprecated and will log a warning. We recommend migrating to the `--identity` flag for AWS authentication.

### Q: What's the difference between `cluster_name_pattern` and `cluster_name_template`?

A: `cluster_name_pattern` uses simple token replacement (`{namespace}`), while `cluster_name_template` uses Go templates (`{{ .vars.namespace }}`). The template syntax is more powerful and consistent with stack configurations.

### Q: Will the deprecated options be removed?

A: Yes, `helm_aws_profile_pattern` and `cluster_name_pattern` will be removed in a future major release. We recommend migrating now to avoid issues during future upgrades.

## Changelog

| Date | Version | Changes |
|------|---------|---------|
| 2025-12-20 | 1.0 | Initial migration guide |
