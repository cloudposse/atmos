# Terraform Backend Key Prefix — Slash Preservation Option

**Version:** 1.0
**Last Updated:** 2026-04-11

---

## Executive Summary

Atmos auto-generates the `workspace_key_prefix` (S3), `prefix` (GCS), and `key`
(Azure) for Terraform backends based on the component name or `metadata.name`.
As part of this auto-generation, Atmos replaces all `/` characters with `-` in
the component name. This means a component like `services/consul` gets a
flattened backend key `services-consul/workspace/terraform.tfstate` instead of
the hierarchical `services/consul/workspace/terraform.tfstate`.

Users with large component libraries want to preserve the `/`
separator to maintain directory hierarchy in their state buckets, matching the
organization of their component source directories. Currently, the only
workaround is to set `workspace_key_prefix` explicitly via Go templates, which
bypasses the cleaner `metadata.name` approach.

This PRD proposes a new `atmos.yaml` setting that controls whether Atmos
replaces `/` with `-` in auto-generated backend key prefixes, giving users
opt-in control without breaking existing configurations.

---

## Problem Statement

### Current Behavior

When Atmos auto-generates backend key prefixes, it applies a hard-coded
`strings.ReplaceAll(prefix, "/", "-")` transformation in three places:

1. **S3 backend** — `workspace_key_prefix` in
   `internal/exec/stack_processor_backend.go:setS3BackendDefaults` (line 95)
2. **GCS backend** — `prefix` in
   `internal/exec/stack_processor_backend.go:setGCSBackendDefaults` (line 110)
3. **Azure backend** — `key` in
   `internal/exec/stack_processor_backend.go:setAzureBackendKey` (line 158)

The priority chain for the prefix value is:

1. Explicit `workspace_key_prefix` / `prefix` / `key` in backend config → used
   as-is (no replacement)
2. `metadata.name` → value used, but `/` replaced with `-`
3. `metadata.component` (base component) → value used, but `/` replaced with `-`
4. Atmos component name → value used, but `/` replaced with `-`

### User Impact

For a component defined as `services/consul`:

| Setting                                             | Current result                                | Desired result                                |
|-----------------------------------------------------|-----------------------------------------------|-----------------------------------------------|
| Component name (default)                            | `services-consul/workspace/terraform.tfstate` | `services/consul/workspace/terraform.tfstate` |
| `metadata.name: services/consul`                    | `services-consul/workspace/terraform.tfstate` | `services/consul/workspace/terraform.tfstate` |
| Explicit `workspace_key_prefix: "{{ .component }}"` | `services/consul/workspace/terraform.tfstate` | N/A (already works)                           |

Users can work around the issue by setting `workspace_key_prefix` explicitly
with a Go template, but this bypasses the cleaner `metadata.name` mechanism and
requires every component to carry the template override.

### Scale

Users with many components organized in directory hierarchies
(`services/consul`, `services/vault`, `platform/eks`, `platform/rds`, etc.)
want the state bucket structure to mirror the component directory structure.
Flat `-` separated prefixes make it difficult to navigate the bucket.

---

## Proposed Solution

### New Configuration Setting

Add a `terraform.workspace.prefix_separator` setting in `atmos.yaml`:

```yaml
terraform:
  workspace:
    prefix_separator: "/"   # Preserve slashes in auto-generated key prefixes
```

| Value | Behavior                                      | Default?                      |
|-------|-----------------------------------------------|-------------------------------|
| `"-"` | Replace `/` with `-` (current behavior)       | **Yes** (backward compatible) |
| `"/"` | Preserve `/` as-is (hierarchical state paths) | No                            |

The setting affects ONLY auto-generated prefixes. Explicitly configured
`workspace_key_prefix` / `prefix` / `key` values are never modified.

### Alternative Considered: Boolean Flag

A `terraform.workspace.preserve_slashes: true` boolean was considered but
rejected in favor of `prefix_separator` because:

1. The separator approach is more flexible — users could theoretically use
   other separators (though `/` and `-` are the practical choices).
2. The setting name makes the behavior self-documenting.
3. It maps directly to the `strings.ReplaceAll(prefix, "/", separator)` call
   in the code, making the implementation trivial.

---

## Implementation

### Code Changes

**`internal/exec/stack_processor_backend.go`**

Modify the three `strings.ReplaceAll` calls to use the configured separator
instead of the hard-coded `"-"`:

```go
// Before:
backend["workspace_key_prefix"] = strings.ReplaceAll(workspaceKeyPrefix, "/", "-")

// After:
separator := getWorkspacePrefixSeparator(&atmosConfig)
if separator != "/" {
    backend["workspace_key_prefix"] = strings.ReplaceAll(workspaceKeyPrefix, "/", separator)
} else {
    backend["workspace_key_prefix"] = workspaceKeyPrefix
}
```

Apply the same change to `setGCSBackendDefaults` (prefix) and
`setAzureBackendKey` (key component).

The helper function uses a pointer receiver to avoid copying the large
`AtmosConfiguration` struct (~6 KB):

```go
// getWorkspacePrefixSeparator returns the configured separator for auto-generated
// backend key prefixes. Defaults to "-" for backward compatibility.
func getWorkspacePrefixSeparator(atmosConfig *schema.AtmosConfiguration) string {
    if atmosConfig != nil && atmosConfig.Terraform.Workspace.PrefixSeparator != "" {
        return atmosConfig.Terraform.Workspace.PrefixSeparator
    }
    return "-"
}
```

The three setter functions (`setS3BackendDefaults`, `setGCSBackendDefaults`,
`setAzureBackendKey`) must be updated to accept `*schema.AtmosConfiguration`
as an additional parameter (passed by pointer per coding guidelines for large
data structures).

**`pkg/schema/schema.go`**

Add the workspace configuration to the Terraform section:

```go
type TerraformConfiguration struct {
// ... existing fields ...
Workspace WorkspaceConfig `yaml:"workspace,omitempty" json:"workspace,omitempty" mapstructure:"workspace"`
}

type WorkspaceConfig struct {
    PrefixSeparator string `yaml:"prefix_separator,omitempty" json:"prefix_separator,omitempty" mapstructure:"prefix_separator"`
}
```

**Default value:** `"-"` (set during config initialization, preserving backward
compatibility).

### Affected Functions

| Function                | File                         | Change                   |
|-------------------------|------------------------------|--------------------------|
| `setS3BackendDefaults`  | `stack_processor_backend.go` | Use configured separator |
| `setGCSBackendDefaults` | `stack_processor_backend.go` | Use configured separator |
| `setAzureBackendKey`    | `stack_processor_backend.go` | Use configured separator |

### Migration

**No migration required.** The default value `"-"` preserves the existing
behavior. Users opt in to slash preservation by setting
`terraform.workspace.prefix_separator: "/"` in `atmos.yaml`.

**Warning:** Changing the separator on an existing project will change the
backend key paths, which means Terraform will not find existing state files at
the old paths. Users must migrate their state files (e.g. `terraform state mv`
or bucket-level rename) when changing this setting. The documentation should
include a clear warning about this.

---

## Testing

### Unit Tests

1. **Default behavior preserved** — component `services/consul` with default
   config produces `services-consul` prefix.
2. **Slash preservation** — component `services/consul` with
   `prefix_separator: "/"` produces `services/consul` prefix.
3. **Explicit config unaffected** — explicit `workspace_key_prefix` is never
   modified regardless of separator setting.
4. **metadata.name with slashes** — `metadata.name: services/consul` with
   `prefix_separator: "/"` produces `services/consul`.
5. **All backends** — S3, GCS, and Azure backends all respect the setting.
6. **Empty/missing config** — missing `workspace` section defaults to `"-"`.

### Integration Tests

- Stack fixture with `prefix_separator: "/"` and a hierarchical component name.
- Verify `atmos describe component` output shows the correct backend
  configuration with preserved slashes.

---

## Documentation

### `atmos.yaml` Reference

Add `terraform.workspace.prefix_separator` to the configuration reference:

```yaml
terraform:
  workspace:
    # Controls how '/' in component names is handled when auto-generating
    # backend key prefixes (workspace_key_prefix for S3, prefix for GCS,
    # key for Azure).
    #
    # "-" (default): Replace '/' with '-' → services/consul becomes services-consul
    # "/": Preserve '/' as-is → services/consul stays services/consul
    #
    # WARNING: Changing this setting on an existing project will change state
    # file paths. Existing state must be migrated manually.
    prefix_separator: "-"
```

### Website Documentation Updates

The following docs must be updated to document `terraform.workspace.prefix_separator`:

| File                                                      | What to add                                                                                                                                                      |
|-----------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `website/docs/cli/configuration/components/terraform.mdx` | Add `workspace` section with `prefix_separator` to the YAML example and the attribute reference table. This is the primary Terraform configuration reference.    |
| `website/docs/stacks/components/terraform/backend.mdx`    | Add a note explaining how `prefix_separator` interacts with auto-generated `workspace_key_prefix`. Include the before/after example for hierarchical components. |
| `website/docs/stacks/backend.mdx`                         | Add a cross-reference to the new setting in the general backend overview.                                                                                        |

### Migration Guide

Document the state migration process for users switching from `-` to `/`:

1. Identify affected components.
2. Rename state paths in the backend (S3 bucket, GCS bucket, Azure storage).
3. Update `atmos.yaml` with `prefix_separator: "/"`.
4. Verify with `atmos describe component` that backend paths are correct.
5. Run `atmos terraform plan` to confirm no unexpected changes.

---

## Risks and Mitigations

| Risk                                                        | Mitigation                                                        |
|-------------------------------------------------------------|-------------------------------------------------------------------|
| Changing separator breaks existing state paths              | Default is `"-"` (backward compatible); warning in docs           |
| Users accidentally change separator without migrating state | Add a warning log at startup when `prefix_separator` is not `"-"` |
| Separator value other than `-` or `/` could cause problems  | Validate the setting; only allow `-` and `/`                      |

---

## Open Questions

1. **Should the setting support per-component override?** The current proposal
   is global (`atmos.yaml`). Per-component override via `metadata` would add
   complexity but allow gradual migration.

2. **Should Atmos offer a migration helper?** A command like
   `atmos terraform migrate-state --from-separator="-" --to-separator="/"` could
   automate the rename. This is out of scope for v1 but could be a follow-up.

3. **Environment variable override?** Should there be an `ATMOS_TERRAFORM_WORKSPACE_PREFIX_SEPARATOR`
   env var? Follows the existing pattern but may be overkill for this setting.

---

## Release Checklist

### Blog Post

Create a changelog blog post in `website/blog/` (`.mdx` format). For
reference, see `website/blog/2026-04-03-aws-security-compliance.mdx`.

Requirements:
- Use `.mdx` extension with YAML front matter.
- Read `website/blog/tags.yml` — only use tags defined there (likely
  `feature` or `enhancement`).
- Read `website/blog/authors.yml` — use an existing author entry.
- Include `<!--truncate-->` after the intro paragraph.
- Sections: What Changed, Why This Matters, How to Use It.
- Include the `atmos.yaml` configuration example and a before/after
  comparison showing the effect on state bucket paths.
- Include a warning about state migration for existing projects.

### Roadmap Entry

Add a shipped milestone to the appropriate initiative in
`website/src/data/roadmap.js`. For instructions, see
`.claude/agents/roadmap.md`.

Requirements:
- Find the Terraform or configuration management initiative (or the
  closest match).
- Add a milestone with `status: 'shipped'`, the PR number, and link
  the blog post via `changelog: '<blog-slug>'`.
- Update the initiative's `progress` percentage.
