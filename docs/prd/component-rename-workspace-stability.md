# PRD: Component Rename Workspace Stability

**Version:** 4.0  
**Last Updated:** 2026-04-15  
**Issue:** [#2244](https://github.com/cloudposse/atmos/issues/2244)

---

## Problem

Renaming an Atmos component (changing its YAML key, or renaming the stack/account it belongs to) silently changes the Terraform workspace name. Terraform treats the new workspace as empty and plans to recreate all resources, while the old workspace retains orphaned state.

### How workspace names are derived today

`BuildTerraformWorkspace` (`internal/exec/stack_utils.go`) uses this priority chain:

| Priority | Source | Example |
|----------|--------|---------|
| 1 | `metadata.terraform_workspace_template` | Go template |
| 2 | `metadata.terraform_workspace_pattern` | Token pattern |
| 3 | `metadata.terraform_workspace` | Static string |
| 4 | Stack prefix only (no base component) | `ue1-dev` |
| 5 | `{stack_prefix}-{yaml_key}` | `ue1-dev-rds-hello-world-service` |

Priority 5 couples the workspace name to the YAML key. Renaming `rds/hello-world-service` to `hello-world-service/rds` changes the workspace from `ue1-dev-rds-hello-world-service` to `ue1-dev-hello-world-service-rds`. The current workaround (`metadata.terraform_workspace: "ue1-dev-rds-hello-world-service"`) hard-codes the stack name, breaks inheritance, and must be set proactively — before the rename happens.

### The collision problem

Beyond rename-triggered breakage, a second hazard exists: **workspace name collision**. If component A is renamed to a YAML key whose computed workspace name matches component B's workspace, both components operate against the same Terraform state. This is undetectable until state corruption occurs.

### The catalogue problem

Abstract components in the catalogue have no stack context. If we require `metadata.name` on every component instance, teams must add it manually to every concrete stack manifest. For large repos with hundreds of stack instances this is impractical — especially for the common multi-instance pattern where the same abstract component is instantiated more than once within the same stack with different YAML keys.

---

## Options

### Option A — `metadata.name` controls the workspace name

**What:** Extend `BuildTerraformWorkspace` to use `metadata.name` (already used for backend key prefix stability) as the component segment in the fallback workspace name.

**New priority table:**

| Priority | Source | Notes |
|----------|--------|-------|
| … | (1–3 unchanged) | |
| 4 | Stack prefix only (no base component) | Unchanged |
| **5** | **`{stack_prefix}-{metadata.name}`** | **New** |
| 6 | `{stack_prefix}-{yaml_key}` | Current behaviour |

**Usage:**
```yaml
hello-world-service/rds:         # YAML key (can change freely)
  metadata:
    name: rds/hello-world-service # Stable identity → workspace: ue1-dev-rds-hello-world-service
```

**Catalogue pattern:** A catalogue (abstract) component can set `metadata.name` once. All concrete stack instances that inherit it get a stable workspace segment automatically — no per-instance override needed — as long as each instance has a distinct YAML key in the stack manifest.

**Devil's advocate:**
- Still opt-in. Users who didn't set `metadata.name` before renaming get no protection.
- `metadata.name` becomes an immutable commitment. Changing it recreates the problem.
- Only stabilises the component segment. Stack/account renames still change the workspace prefix.
- No collision detection: two components with the same `metadata.name` silently share a workspace.
- The catalogue single-abstract-multiple-instance case (two YAML keys inheriting the same `metadata.name` in the same stack) requires a per-instance override — which is exactly the manual work users want to avoid.

**Verdict:** Excellent for new projects and single-instance catalogue patterns. Needs Option F (the init subcommand) to be practical for existing projects.

---

### Option B — `atmos rename component` migration command

**What:** An interactive CLI command that discovers every stack where a component is deployed, migrates Terraform workspace state (pull/push via backend API), and updates YAML manifests.

```
> atmos rename component rds/hello-world-service hello-world-service/rds

Found 2 stacks: ue1-dev, ue2-dev
Rename and migrate workspaces? [y/N]: y
  [ue1-dev] ue1-dev-rds-hello-world-service → ue1-dev-hello-world-service-rds ✓
  [ue2-dev] ue2-dev-rds-hello-world-service → ue2-dev-hello-world-service-rds ✓
```

**Devil's advocate:**
- Only helps users who know they are renaming. Does nothing for stack/account renames.
- Requires Terraform execution with credentials for every backend. Multi-account environments make this a substantial orchestration problem.
- YAML rewriting risks destroying anchors, comments, and formatting.
- Not atomic: a partial failure leaves some stacks migrated, others not.
- High implementation cost; helps only with deliberate, planned renames.

**Verdict:** High value as a convenience tool. Not a primary safety mechanism. Phase 2+ work.

---

### Option C — Workspace identity lock file (in-repo)

**What:** Atmos automatically writes a lock file (`.atmos/workspace-locks.yaml`) on the first `plan`/`apply` for a given component+stack pair, recording the computed workspace name. Future runs use the locked name regardless of YAML key changes. The lock file is committed to the repo — like `go.sum` or `package-lock.json`.

```yaml
# .atmos/workspace-locks.yaml — auto-generated by Atmos; commit to repo
locks:
  rds/hello-world-service@ue1-dev: "ue1-dev-rds-hello-world-service"
  rds/hello-world-service@ue2-dev: "ue2-dev-rds-hello-world-service"
```

When a component is renamed and has no lock entry, Atmos warns and falls back to normal derivation. `atmos workspace lock migrate <old> <new>` transfers the lock entry.

**Collision detection:** Before writing a new entry, Atmos verifies the workspace value is not already claimed:
```
ERROR: Workspace 'ue1-dev-db' is already locked by component 'db@ue1-dev'.
       Cannot assign it to 'database@ue1-dev'. Resolve the conflict first.
```

**Catalogue pattern:** The lock file keys on `{yaml_key}@{stack}`, so multiple instances of the same abstract component in the same stack each get their own independent lock entry. No per-instance `metadata.name` is required.

**Devil's advocate:**
- Atmos writing files during `plan`/`apply` is unexpected. Read-only CI pipelines cannot commit the file back; if not committed, the workspace name is re-derived on the next run.
- Concurrent CI runs adding new components conflict on the lock file.
- Bootstrapping an existing repo requires a separate `atmos workspace lock generate --all` command.

**Verdict:** Solves zero-proactive-effort and the catalogue multi-instance problem. The commit discipline is the main operational cost. Right choice for Phase 2.

---

### Option D — `atmos-state` branch (remote auto-committed lock)

**What:** Store the workspace lock registry on a dedicated, non-protected branch (e.g., `atmos-state`), analogous to `gh-pages`. Atmos pushes updates after every `plan`/`apply`. No files are added to the main branch.

```
main branch:        stacks/*.yaml  (user-managed)
atmos-state branch: workspace-locks.yaml  (Atmos-managed)
```

**Devil's advocate:**
- Concurrent runs race to push, requiring a fetch-rebase-retry loop with real latency.
- Requires repo write access from every CI job that runs `plan` or `apply`.
- Lock changes are invisible in PRs — reviewers cannot see or approve workspace assignments.
- Branch deletion or force-push destroys all workspace assignments with no recovery.

**Verdict:** Solves the main-branch commit conflict but introduces worse problems. Offer as an opt-in alternative backend (`workspace_locks.backend: git-branch` in `atmos.yaml`), not the default.

---

### Option E — GUID written into stack YAML (auto-mutate user files)

**What:** On first provision, Atmos generates a UUID and writes it into the component's `metadata` block in the stack YAML file itself.

**Devil's advocate:**
- Atmos mutating user-managed YAML destroys formatting, anchors, and comments. Explicitly rejected in the original issue.
- UUIDs are opaque; workspace names become meaningless.
- Read-only CI cannot write UUIDs.
- Every new component triggers a dirty working tree.

**Verdict:** Not viable.

---

### Option F — `atmos workspace init` migration subcommand

**What:** A one-time CLI command that walks all deployed component instances, computes their current workspace names, and writes `metadata.name` into the component definitions. This is a user-initiated, one-time migration: the user reviews the diff and commits it. Unlike Option E (auto-mutate during `plan`/`apply`), this runs explicitly, produces a predictable diff, and never runs again unless invoked.

```
> atmos workspace init [--format=key|key-hash|uuid] [--component=<name>] [--stack=<name>]

Computing workspace names for all components...
  Writing metadata.name to stacks/catalog/rds.yaml       rds/hello-world-service
  Writing metadata.name to stacks/catalog/vpc.yaml       vpc
  Writing metadata.name to stacks/ue1-dev.yaml           app-overrides → skipped (already set)

3 components updated. Review the diff and commit.
```

**Name format options — analysis:**

| Format | Example `metadata.name` | Resulting workspace | Notes |
|--------|------------------------|---------------------|-------|
| `--format=key` *(default)* | `rds/hello-world-service` | `ue1-dev-rds-hello-world-service` | Human-readable. No change to existing workspace names. The right default. |
| `--format=key-hash` | `rds-hello-a3f2` | `ue1-dev-rds-hello-a3f2` | Workspace names change, but are still partially readable and guaranteed unique per instance. |
| `--format=uuid` | `a3f2c891-7d1b-4e6a-b3d9` | `ue1-dev-a3f2c891-7d1b-4e6a-b3d9` | Opaque, maximally collision-proof. Not recommended unless uniqueness is a hard requirement. |

**The right format:** `--format=key` (the default) writes the current YAML key as `metadata.name`. This pins workspace identity to the moment of the migration run with no change to workspace names, no opaque identifiers, and no disruption to existing infrastructure. Use `--format=key-hash` only when you expect future YAML key collisions and need a uniqueness guarantee baked in from day one.

> **Important:** `metadata.name` must contain only the **component-identity segment** (e.g., `rds/hello-world-service`), not the full computed workspace name (e.g., `ue1-dev-rds-hello-world-service`). Writing the full workspace name would produce `{stack_prefix}-{full_workspace_name}` — a doubled prefix. The init command must strip the stack prefix before writing.

**Catalogue pattern:** The init command writes `metadata.name` to the catalogue (abstract component) file when a canonical name can be determined — specifically when all concrete instances of the component share the same YAML key segment. For multi-instance stacks where the same abstract component appears under multiple YAML keys, the command writes per-instance overrides to the concrete stack files, not the catalogue.

**Devil's advocate:**
- Writes to user YAML files. Safer than Option E (user-controlled, one-time, reviewable) but still modifies files that users own.
- Only useful for `--format=key`. The hash and UUID formats change existing workspace names, which is exactly the disruption users are trying to avoid.
- Does not solve the ongoing problem — it gives you a snapshot that is stable from the migration date forward, but future new components still need manual `metadata.name` entries (or a lock file from Option C).
- Catalogue multi-instance: when two YAML keys in the same stack inherit the same abstract component, the init command must write per-instance `metadata.name` overrides. Users may be surprised to find their clean catalogue pattern now has instance-level overrides scattered across stack files.

**Verdict:** The best bridge for existing projects migrating to Option A. Run once, commit, done. Use `--format=key` (default). Does not replace Option C for ongoing zero-effort stability.

---

### Option G — Remote object store lock backend (S3 / GCS / Azure Blob)

**What:** Store the workspace lock registry as a single YAML object in a remote object store — most commonly the same S3 bucket (and prefix) already used for Terraform backend state. Atmos reads the lock on every `plan`/`apply`, writes new entries back using atomic conditional-write semantics, and never touches the repo or any Git branch.

```yaml
# atmos.yaml — configure the lock backend
workspace_locks:
  backend: s3                           # or: local (default), git-branch
  s3:
    bucket: "my-terraform-state-bucket"
    key: ".atmos/workspace-locks.yaml"  # or derive from existing backend config
    region: "us-east-1"
    # dynamodb_table: "terraform-locks"  # optional: reuse existing DynamoDB lock table
```

```yaml
# s3://my-terraform-state-bucket/.atmos/workspace-locks.yaml
locks:
  rds/hello-world-service@ue1-dev: "ue1-dev-rds-hello-world-service"
  rds/hello-world-service@ue2-dev: "ue2-dev-rds-hello-world-service"
  rds-primary@ue1-dev: "ue1-dev-rds-primary"
  rds-replica@ue1-dev: "ue1-dev-rds-replica"
```

**Synergy with Terraform state bucket:** Teams using S3-backend Terraform already have:
- An S3 bucket with versioning enabled.
- IAM roles/credentials for CI with read/write access to the bucket.
- Optionally, a DynamoDB table for Terraform state locking.

The workspace lock file can live alongside Terraform state at a well-known key derived from the existing `atmos.yaml` backend configuration. No new bucket, no new IAM policy, no new credentials.

**Concurrent-write safety:** Unlike the git-branch option (Option D), S3 supports atomic conditional writes:
- **ETag-based optimistic locking:** Read the current object + ETag. Write with `If-Match: <etag>`. If another process wrote between read and write, the `PreconditionFailed` error triggers a read-retry loop. Typical lock contention is milliseconds, not git-rebase seconds.
- **DynamoDB lock table (optional):** If a DynamoDB table is already configured, Atmos can acquire a lock row before writing, matching Terraform's own locking strategy exactly.
- **S3 conditional creates (`If-None-Match: *`):** For bootstrapping, ensures the initial write only succeeds once.

**Catalogue pattern:** Same as Option C — the lock file keys on `{yaml_key}@{stack}`, so multi-instance stacks are handled automatically. No `metadata.name` required.

**Bootstrap:** `atmos workspace lock generate [--backend=s3]` scans all stacks, computes current workspace names, and writes the initial lock object. No Terraform runs needed.

**Devil's advocate:**
- Invisible in PRs: workspace assignment changes cannot be reviewed or approved before they take effect. A bad rename can propagate silently until the next `plan`.
- Requires S3 (or equivalent) credentials from every CI job that runs `plan` or `apply`. For some teams, plan jobs are intentionally read-only and adding write permissions is a policy violation.
- The lock is outside the repo. If the bucket is deleted, versioning is off, or the key is accidentally overwritten, all workspace assignments are lost. Recovery requires re-running the bootstrap command, which re-derives from the _current_ YAML keys — defeating the purpose if renaming already happened.
- Multi-cloud teams need GCS or Azure Blob support, not just S3. Abstractions help, but each backend adds implementation and test surface.
- Bootstrapping from a completely fresh environment (new CI runner with no prior lock state) re-derives workspace names from YAML keys, which is correct for new components but silently wrong for already-renamed components.

**Mitigation for the visibility problem:** An `atmos workspace lock diff` command can compare the remote lock state against the current YAML configuration and show what would change. CI can run this as a read-only check in plan jobs and post the diff as a PR comment — providing review visibility without writing to S3.

**Verdict:** The best option for teams that:
1. Already use S3 backend and have the bucket + credentials in place.
2. Run concurrent CI pipelines that cannot commit back to the repo.
3. Have multi-instance catalogue patterns (so Option A+F are insufficient).

This is strictly better than Option D (git-branch) on all axes: atomic writes, no git race, no repo write access required, durable storage. The main trade-off vs. Option C (in-repo) is visibility: workspace assignment changes are not in the PR diff.

**Recommended as the default Phase 2 backend for AWS-native teams.** Option C (in-repo) remains the better choice for teams that prioritise reviewability over operational simplicity.

---

## The catalogue / multi-instance problem in full

This is the central tension. The options split into two camps:

**`metadata.name`-based options (A + F)** work well when:
- Each abstract component is instantiated **once per stack** under a consistent YAML key. In this case, setting `metadata.name` in the catalogue file propagates to all instances via inheritance — zero per-instance config required.
- Teams are willing to run the init command once and commit the result.

They break down when:
- The **same abstract component is instantiated more than once in the same stack** under different YAML keys (e.g., `rds-primary` and `rds-replica`, both inheriting from `rds`). Each instance needs its own `metadata.name`; inheritance cannot provide different values. This forces per-instance overrides in concrete stack files.

**Lock-file-based options (C, D, and G)** handle all patterns natively:
- The lock file keys on `{yaml_key}@{stack}`, so two instances of the same abstract component automatically get independent entries.
- No `metadata.name` required at all. No per-instance config. No catalogue changes.
- Option C cost: commit discipline + new Atmos-managed file in the repo.
- Option D cost: repo write access from CI + race conditions on concurrent pushes.
- Option G cost: S3 credentials from CI + workspace changes invisible in PRs.

**Recommendation given catalogue concern:** Teams with multi-instance patterns should adopt either Option C (in-repo lock, best visibility) or Option G (S3 lock, best for concurrent CI / AWS-native teams). Option F (init subcommand) is the right migration tool for teams adopting Option A with single-instance patterns.

---

## Comparison

| | A — `metadata.name` | B — Rename cmd | C — Lock file (repo) | D — `atmos-state` branch | E — GUID in YAML | F — Init subcommand | G — S3 lock backend |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| Zero proactive setup | ❌ | ❌ | ✅ | ✅ | ✅ | ❌ (one-time run) | ✅ |
| Survives component rename | ✅ if set | ✅ migrates | ✅ locked | ✅ locked | ✅ | ✅ after migration | ✅ locked |
| Survives stack/account rename | ❌ prefix changes | ✅ migrates | ✅ full name locked | ✅ full name locked | ✅ | ❌ prefix changes | ✅ full name locked |
| Catalogue single-instance | ✅ inherit | n/a | ✅ auto | ✅ auto | ❌ | ✅ writes once | ✅ auto |
| Catalogue multi-instance | ❌ per-instance override | n/a | ✅ auto | ✅ auto | ❌ | ❌ per-instance override | ✅ auto |
| Collision detection | ❌ | ✅ pre-check | ✅ on write | ✅ on write | ❌ | ❌ (validate stacks) | ✅ on write |
| Reviewable in PRs | ✅ | ✅ | ✅ | ❌ | ✅ | ✅ | ❌ |
| No CI write-back required | ✅ | ✅ | ❌ must commit | ❌ must push | ❌ | ✅ (one-time only) | ❌ must write S3 |
| Concurrent-safe writes | n/a | n/a | ❌ merge conflicts | ❌ push races | n/a | n/a | ✅ ETag CAS |
| Human-readable workspaces | ✅ | ✅ | ✅ | ✅ | ❌ | ✅ (default format) | ✅ |
| Backwards compatible | ✅ fully | ✅ | ⚠️ bootstrap | ⚠️ bootstrap | ❌ breaking | ✅ one-time opt-in | ⚠️ bootstrap |
| Implementation effort | Trivial | High | Medium | High | Medium | Medium | Medium |

---

## Recommendation

### Phase 1 — Ship now

**A + F together:**

1. Extend `BuildTerraformWorkspace` to honour `metadata.name` (< 10 LOC, fully backwards-compatible).
2. Implement `atmos workspace init --format=key` as a migration subcommand that writes `metadata.name` into component YAML files (one-time, user-reviewed). Catalogue components get the canonical name; per-instance overrides are written only when required by multi-instance stacks.

This gives every project a concrete migration path:
- **New projects:** set `metadata.name` in catalogue components from day one.
- **Existing single-instance projects:** run `atmos workspace init`, review the diff, commit.
- **Existing multi-instance projects:** use the init command for single-instance components; accept that multi-instance stacks need per-instance overrides OR wait for Phase 2.

### Phase 2 — Follow-up: workspace lock backends

**Option C (in-repo lock file)** is the right default for teams that prioritise PR reviewability and want workspace assignment changes to be explicit, diffable, and approvable. The operational cost is commit discipline and merge conflicts on concurrent component additions — manageable with GitOps practices.

**Option G (S3/remote object store lock)** is the right choice for teams that:
- Already use S3 backend (bucket + credentials already in CI).
- Run concurrent pipelines that cannot commit back to the main branch.
- Have multi-instance catalogue patterns.

Option G is strictly better than Option D (git-branch) on every axis. Option D should not be implemented as a standalone backend.

**Configuration design:** Both C and G should be implemented as backends for a single `workspace_locks` subsystem, configured in `atmos.yaml`:

```yaml
workspace_locks:
  backend: local        # or: s3, git-branch (opt-in, not recommended)
  local:
    path: .atmos/workspace-locks.yaml
  s3:
    bucket: "${ATMOS_STATE_BUCKET}"
    key: ".atmos/workspace-locks.yaml"
    region: "${AWS_REGION}"
    dynamodb_table: "${ATMOS_LOCK_TABLE}"  # optional
  # gcs / azure-blob: future backends, same interface
```

This reuses the same multi-provider registry pattern already established in `pkg/store/`.

Option B (`atmos rename component`) as a convenience tool for planned renames, after the lock subsystem is stable.

### Answers to new questions

**Q: Should we enforce `metadata.name` on every component instance?**  
Not as a hard runtime error. As an opt-in lint rule (`atmos lint stacks --rule workspace-identity`). Enforcement at plan time would be a breaking change for all existing configurations and makes catalogue patterns unnecessarily painful.

**Q: What is the right name format for the init subcommand?**  
`--format=key` (the default): writes the current YAML key. Workspace names are unchanged; no opaque identifiers introduced. Use `--format=key-hash` only if you need a uniqueness guarantee baked in from day one. Never use `--format=uuid` — opaque workspace names make debugging impossible.

**Q: What `metadata.name` value should go into a catalogue abstract component?**  
The canonical component name (the YAML key segment that uniquely identifies the component type, e.g., `rds` or `rds/hello-world-service`). This is the component-segment only — never the full workspace name. The stack prefix is always prepended at workspace-build time; writing the full workspace name in `metadata.name` would double the prefix.

**Q: How do we handle the multi-instance case (same abstract component, two YAML keys, same stack)?**  
Option A requires per-instance `metadata.name` overrides in the concrete stack files. Option C (lock file) handles it automatically — each `{yaml_key}@{stack}` is a separate lock entry. If the multi-instance pattern is common in your repo, adopt the lock file (Phase 2) instead of fighting with per-instance overrides.

**Q: Can workspace identity persist dynamically without predefining it for every instance?**  
Yes — any lock-file backend (Option C, G) is precisely this mechanism. The workspace is computed from the YAML key the first time any `plan`/`apply` runs, written to the lock backend, and reused on every future run regardless of YAML changes. No predefinition is required. Choose Option C for in-repo visibility or Option G for concurrent-safe remote storage.

**Q: Should the lock file live in S3 alongside Terraform state?**  
Yes, for AWS-native teams. The S3 backend (Option G) reuses the existing state bucket, IAM roles, and optionally the DynamoDB lock table. CI jobs already have the credentials; no new permissions policy needed. Concurrent writes are handled via ETag-based conditional writes (`If-Match`), which are atomic and much simpler than a git rebase loop. The main trade-off is that workspace assignment changes are not visible in pull requests — mitigated by an `atmos workspace lock diff` command that CI can run and post as a PR comment.

**Q: How are collisions detected in Phase 1 (before the lock file exists)?**  
`atmos validate stacks` computes the resolved workspace name for every component+stack pair and reports duplicates. The init subcommand also performs a pre-write collision check before modifying any files.

---

## Implementation (Phase 1)

### 1. `internal/exec/stack_utils.go` — ~8 lines

```go
// In the Priority 5/6 fallback block:
componentIdentity := configAndStacksInfo.Context.Component  // current behaviour
if name, ok := componentMetadata["name"].(string); ok && name != "" {
    componentIdentity = name  // metadata.name takes precedence
}
workspace = fmt.Sprintf("%s-%s", contextPrefix, componentIdentity)
```

### 2. `atmos workspace init` subcommand

- Scans all stack files and resolves component instances.
- Computes the current workspace name for each instance.
- Determines whether `metadata.name` can be written to the catalogue (abstract) component or must be written per-instance (multi-instance stacks).
- Performs a collision check before writing anything.
- Writes `metadata.name` to the appropriate YAML files.
- Reports what was written and what was skipped (already set).
- `--dry-run` flag prints the changes without writing.
- `--stack` / `--component` flags scope the operation.

### Unit tests (`internal/exec/stack_utils_test.go`)

| Scenario | `metadata.name` | YAML key | Expected workspace |
|----------|-----------------|----------|--------------------|
| Name set, component renamed | `rds/hello-world-service` | `hello-world-service/rds` | `ue1-dev-rds-hello-world-service` |
| Name not set | — | `rds/hello-world-service` | `ue1-dev-rds-hello-world-service` |
| Name not set, component renamed | — | `hello-world-service/rds` | `ue1-dev-hello-world-service-rds` |
| Static override wins | `rds/hws` | any | value of `metadata.terraform_workspace` |
| Template override wins | `rds/hws` | any | result of `metadata.terraform_workspace_template` |
| No base component | `rds/hws` | any | stack prefix only (unchanged) |

### Migration guide

**New projects:** Set `metadata.name` in catalogue components from day one. It is a permanent commitment — changing it is a rename operation.

**Existing projects:**
```bash
atmos workspace init --dry-run   # preview changes
atmos workspace init             # write metadata.name to YAML files
git diff                         # review
git commit -am "chore: pin workspace identity via metadata.name"
atmos terraform plan --all       # confirm zero diff
```

**Multi-instance stacks:** After the init command, verify that per-instance overrides were written correctly for any stack where the same abstract component appears under multiple YAML keys.

---

## Related PRDs

| PRD | Relationship |
|-----|-------------|
| `workspace-key-prefixes.md` | Introduced `metadata.name` for backend key prefix stability. This PRD extends it to workspace name stability. |
| `terraform-workspace-key-prefix-slash-preservation.md` | `/`→`-` substitution applies equally to workspace names. |
| `metadata-inheritance.md` | `metadata.name` can be inherited from abstract components, pinning workspace identity for all derived single-instance components in one place. |
