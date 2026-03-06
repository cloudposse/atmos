# Native CI Integration - Planfile Storage

> Related: [Artifact Storage](../framework/artifact-storage.md) | [Configuration](../framework/configuration.md) | [Implementation Status](../framework/implementation-status.md)

## FR-5: Planfile Storage

**Requirement**: Store and retrieve planfiles across CI jobs.

**Behavior**:
- Upload planfile after successful `terraform plan`
- Download planfile before `terraform apply`
- Support multiple storage backends (S3, GitHub Artifacts, Azure Blob, GCS, local)
- Store metadata sidecar with plan details (no DynamoDB)
- Key pattern configurable per-store via `components.terraform.planfiles.stores.<name>.options.key_pattern`

**Storage Backends**:
| Backend | Key | Description |
|---------|-----|-------------|
| `s3` | `s3://bucket/prefix/...` | AWS S3 with metadata sidecar |
| `github-artifacts` | Artifact name | GitHub Actions artifacts API |
| `azure-blob` | Container/blob path | Azure Blob Storage |
| `gcs` | `gs://bucket/...` | Google Cloud Storage |
| `local` | File path | Local filesystem (dev/testing) |

## CLI Commands (IMPLEMENTED)

### `atmos terraform planfile` Subcommand Group

All subcommands use **component/stack addressing** — consistent with `atmos terraform plan <component> -s <stack>`. Storage keys are derived internally from component + stack + SHA via `KeyPattern.GenerateKey()`.

SHA is resolved automatically: env vars (`ATMOS_CI_SHA`, `GIT_COMMIT`, `CI_COMMIT_SHA`, `COMMIT_SHA`) → git HEAD.

```bash
# List planfiles (component and stack are optional filters)
atmos terraform planfile list [component] [-s stack]         # current SHA
atmos terraform planfile list [component] [-s stack] --all   # all SHAs

# Upload planfile (component required, stack required)
atmos terraform planfile upload <component> -s <stack> [--planfile path] [--sha sha]

# Download planfile (component required, stack required)
atmos terraform planfile download <component> -s <stack> [--output path]

# Show planfile metadata (component required, stack required)
atmos terraform planfile show <component> -s <stack> [--format yaml]

# Delete planfiles (component and stack are optional filters)
atmos terraform planfile delete [component] [-s stack]          # current SHA, confirmation
atmos terraform planfile delete [component] [-s stack] --all    # all SHAs, confirmation
atmos terraform planfile delete [component] [-s stack] --force  # skip confirmation
```

### CLI Subcommand Details (IMPLEMENTED)

**list** (`list [component]`) — Lists planfile artifacts, filtered by component, stack, and SHA:
```bash
atmos terraform planfile list                            # all planfiles for current SHA
atmos terraform planfile list --all                      # all planfiles across all SHAs
atmos terraform planfile list vpc                        # filter by component
atmos terraform planfile list vpc -s plat-ue2-dev        # filter by component + stack
atmos terraform planfile list --format json              # JSON output
atmos terraform planfile list --store s3                 # use specific store
```
Flags: `--store`, `--format` (table, json, yaml, csv, tsv), `--all`

**upload** (`upload <component>`) — Uploads a planfile with metadata:
```bash
atmos terraform planfile upload vpc -s plat-ue2-dev
atmos terraform planfile upload vpc -s plat-ue2-dev --planfile plan.tfplan
atmos terraform planfile upload vpc -s plat-ue2-dev --sha abc123 --store s3
```
Flags: `--store`, `--planfile`, `--sha`, `--lockfile`
When `--planfile` is omitted, the path is derived from the component and stack.

**download** (`download <component>`) — Downloads a planfile by component + stack:
```bash
atmos terraform planfile download vpc -s plat-ue2-dev
atmos terraform planfile download vpc -s plat-ue2-dev --output ./local-plan.tfplan
```
Flags: `--store`, `--output` / `-o` (defaults to `plan.tfplan`)

**delete** (`delete [component]`) — Deletes planfiles with confirmation:
```bash
atmos terraform planfile delete                             # delete all for current SHA
atmos terraform planfile delete vpc                         # delete for component + current SHA
atmos terraform planfile delete vpc -s plat-ue2-dev         # delete for component + stack + current SHA
atmos terraform planfile delete vpc -s plat-ue2-dev --all   # delete for component + stack across all SHAs
atmos terraform planfile delete --force                     # skip confirmation prompt
```
Flags: `--store`, `--force` (`-f`), `--all`

**show** (`show <component>`) — Shows planfile metadata:
```bash
atmos terraform planfile show vpc -s plat-ue2-dev
atmos terraform planfile show vpc -s plat-ue2-dev --format json
```
Flags: `--store`, `--format` (json, yaml)

### Flags

- **`--stack` / `-s` flag**: Persistent flag on `planfile` parent command, inherited by all subcommands. Specifies the stack name.
- **`--store` flag**: Accepts a named store from config. Available on all subcommands (command-specific).
- **`--format` flag**: Available on `list` (table, json, yaml, csv, tsv) and `show` (json, yaml).
- **`--all` flag**: Available on `list` and `delete` only. Bypasses SHA filtering to show/delete planfiles across all SHAs.
- **`--force` / `-f` flag**: Available on `delete` only. Skips interactive confirmation prompt.
- **`--output` / `-o` flag**: Available on `download` only. Specifies output path (defaults to `plan.tfplan`).
- **Command group**: `atmos terraform planfile` is correct. Artifacts in general do not need a CLI interface — they define a generic framework for artifact storage in atmos. Specific implementations (like planfile) expose their own CLI commands.

### SHA Resolution (all commands)

SHA is resolved automatically using this priority chain:
1. `--sha` flag (upload only)
2. `ATMOS_CI_SHA` env var
3. `GIT_COMMIT` env var
4. `CI_COMMIT_SHA` env var
5. `COMMIT_SHA` env var
6. `git rev-parse HEAD` (via `pkg/git.NewDefaultGitRepo().GetCurrentCommitSHA()`)

When `--all` is set (list/delete), SHA filtering is skipped entirely.

### Automatic Upload/Download (IMPLEMENTED)

**Upload/download is automatic and event-driven** — no per-command flags needed:

- **Upload**: Automatically triggered by the `after.terraform.plan` hook event. The plugin's `uploadPlanfile()` handler resolves the planfile path from `ctx.Info.PlanFile` and uploads to the configured store via `ctx.CreatePlanfileStore()`.
- **Download**: Automatically triggered by the `before.terraform.apply` hook event. The plugin's `downloadPlanfile()` handler resolves the planfile path and downloads from the configured store.
- Upload/download are **always enabled** when CI mode is active (no config gate — they run whenever the handler is invoked).

**Existing plan command flags (IMPLEMENTED):**

| Flag | Description |
|------|-------------|
| `--ci` | Enable CI mode (auto-detected from `CI` env var) |
| `--skip-planfile` | Skip writing the plan to a file |

**Not yet implemented:**

| Flag | Description | Status |
|------|-------------|--------|
| `--verify-plan` | Verify plan hasn't changed (uses plan-diff) | Not Started |

## Backend Configuration Example

```yaml
components:
  terraform:
    planfiles:
      priority:
        - "github"
        - "s3"
        - "local"
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
        local:
          type: local
          options:
            path: ".atmos/planfiles"
            key_pattern: "{{ .Stack }}/{{ .Component }}/{{ .SHA }}.tfplan"
```

See [Configuration](../framework/configuration.md) for full schema details.

## GitHub Artifacts Lookup Strategy

The GitHub Artifacts store starts with **simple SHA-based lookup** — find artifacts matching the current commit SHA. The lookup logic is encapsulated behind a method so it can later be extended to support:

- **Merge-commit traversal** — walking PR commit history to find artifacts from pre-merge commits
- **Squash-merge support** — looking up artifacts by PR number when the original SHA no longer exists

**Cross-workflow access**: The GitHub Artifacts store must support downloading artifacts from other workflow runs (e.g., apply workflow downloading planfiles uploaded by the plan workflow). This is the primary use case.

## Store Type Validation

All store types (`s3`, `github-artifacts`, `azure-blob`, `gcs`, `local`) are accepted in configuration validation. Unimplemented backends fail at runtime only when actually selected via `--store` or priority. Users can pre-configure future backends without breaking current functionality.
