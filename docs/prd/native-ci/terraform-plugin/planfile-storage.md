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

All subcommands use **key-based addressing** — the storage key identifies artifacts, not component/stack positional arguments.

```bash
# Upload planfile to configured storage
atmos terraform planfile upload [--component vpc --stack plat-ue2-dev --sha abc123 --planfile plan.tfplan --key custom/key --store s3]

# Download planfile from storage
atmos terraform planfile download <key> [output-path] [--store s3]

# List planfiles in storage
atmos terraform planfile list [prefix] [--store s3 --format table]

# Delete planfile from storage
atmos terraform planfile delete <key> [--store s3 --force]

# Show planfile metadata
atmos terraform planfile show <key> [--store s3 --format yaml]
```

### CLI Subcommand Details (IMPLEMENTED)

**list** (`list [prefix]`) — Lists planfile artifacts, optionally filtered by prefix:
```bash
atmos terraform planfile list                            # all planfiles
atmos terraform planfile list plat-ue2-dev/vpc           # filter by prefix
atmos terraform planfile list --format json              # JSON output
atmos terraform planfile list --store s3                 # use specific store
```
Flags: `--store`, `--format` (table, json, yaml, csv, tsv)

**upload** (`upload [options]`) — Uploads a planfile with metadata:
```bash
atmos terraform planfile upload --component vpc --stack plat-ue2-dev
atmos terraform planfile upload --planfile plan.tfplan --key custom/key
atmos terraform planfile upload --sha abc123 --store s3
```
Flags: `--store`, `--planfile`, `--key`, `--stack`, `--component`, `--sha`
When `--planfile` is omitted, the path is derived from `--component` and `--stack`.

**download** (`download <key> [output-path]`) — Downloads a planfile by storage key:
```bash
atmos terraform planfile download plat-ue2-dev/vpc/abc123.tfplan
atmos terraform planfile download plat-ue2-dev/vpc/abc123.tfplan ./local-plan.tfplan
```
Flags: `--store`
If output-path is not specified, file is written to current directory with the key's basename.

**delete** (`delete <key>`) — Deletes a planfile by storage key with confirmation:
```bash
atmos terraform planfile delete plat-ue2-dev/vpc/abc123.tfplan
atmos terraform planfile delete plat-ue2-dev/vpc/abc123.tfplan --force
```
Flags: `--store`, `--force` (`-f`) to skip confirmation prompt

**show** (`show <key>`) — Shows planfile metadata:
```bash
atmos terraform planfile show plat-ue2-dev/vpc/abc123.tfplan
atmos terraform planfile show plat-ue2-dev/vpc/abc123.tfplan --format json
```
Flags: `--store`, `--format` (json, yaml)

### Flags

- **`--store` flag**: Accepts a named store from config. Available on all subcommands.
- **`--format` flag**: Available on `list` (table, json, yaml, csv, tsv) and `show` (json, yaml).
- **`--force` / `-f` flag**: Available on `delete` to skip confirmation prompt.
- **Command group**: `atmos terraform planfile` is correct. Artifacts in general do not need a CLI interface — they define a generic framework for artifact storage in atmos. Specific implementations (like planfile) expose their own CLI commands.

### Automatic Upload/Download (IMPLEMENTED)

**Upload/download is automatic and event-driven** — no per-command flags needed:

- **Upload**: Automatically triggered by `after.terraform.plan` hook event via `ActionUpload`. The executor resolves the planfile path from `info.PlanFile` (or via `ComponentConfigurationResolver`) and uploads to the configured store.
- **Download**: Automatically triggered by `before.terraform.apply` hook event via `ActionDownload`. The executor resolves the planfile path and downloads from the configured store.
- Upload/download are **always enabled** when CI mode is active (`isActionEnabled()` returns true for `ActionUpload`/`ActionDownload`).

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
