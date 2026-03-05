# Native CI Integration - Configuration

> Related: [Overview](../overview.md) | [Planfile Storage](../terraform-plugin/planfile-storage.md) | [Artifact Storage](./artifact-storage.md)

## Configuration Schema (IMPLEMENTED in `pkg/schema/schema.go`)

Configuration is split between `components.terraform` (planfile storage) and `ci` (output behavior) sections in `atmos.yaml`:

```yaml
# atmos.yaml

components:
  terraform:
    # Planfile storage backends (registry pattern)
    planfiles:
      # Default store name (optional — if unset, environment-based detection is used)
      # default: "s3"

      # Stores are tried in priority order; if unavailable, fall through to next
      priority:
        - "github"
        - "s3"
        - "local"

      # Named stores — each backend has its own key/naming pattern
      stores:
        s3:
          type: s3
          options:
            bucket: "my-terraform-planfiles"
            prefix: "atmos/"
            region: "us-east-1"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"

        github:
          type: github-artifacts
          options:
            retention_days: 7
            owner: cloudposse
            repo: github-action-atmos-terraform-plan
            # GitHub uses the artifact name from the implementation layer directly

        azure:
          type: azure-blob
          options:
            account: "mystorageaccount"
            container: "planfiles"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"

        gcs:
          type: gcs
          options:
            bucket: "my-gcs-bucket"
            prefix: "planfiles/"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"

        local:
          type: local
          options:
            path: ".atmos/planfiles"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"

# CI-specific settings (provider-agnostic naming)
ci:
  # Gate for CI auto-detection (bool, default false).
  # When false/unset: CI features only activate with --ci flag (or CI env var).
  # When true: CI features available via auto-detection even without --ci flag.
  # Note: --ci flag bypasses this setting. See ci-detection.md for full precedence table.
  enabled: true

  # Output variables for downstream jobs
  # GitHub: $GITHUB_OUTPUT, GitLab: dotenv artifacts
  output:
    enabled: true
    # Whitelist of variables to export. When omitted, ALL variables are exported.
    # variables:
    #   - has_changes
    #   - has_errors
    #   - exit_code
    #   - resources_to_create
    #   - resources_to_change
    #   - resources_to_replace
    #   - resources_to_destroy
    #   - stack
    #   - component
    #   - command
    #   - summary

  # Job summary with plan/apply results
  # GitHub: $GITHUB_STEP_SUMMARY, GitLab: job artifacts
  summary:
    enabled: true
    # template: "custom-summary.md"

  # Commit status checks
  # GitHub: Check Runs API, GitLab: Commit Status API
  checks:
    enabled: false  # Disabled by default (requires additional permissions)
    context_prefix: "atmos"

  # PR/MR comments
  # GitHub: PR comments, GitLab: MR notes
  comments:
    enabled: true
    behavior: upsert  # create, update, upsert
    # template: "custom-comment.md"

  # Template overrides (shared across all features)
  templates:
    base_path: ".atmos/ci/templates"
    terraform:
      plan: "plan.md"
      apply: "apply.md"
```
