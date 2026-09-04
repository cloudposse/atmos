# Atmos Pre-commit Hooks Integration

## Problem Statement

### Background

The [pre-commit](https://pre-commit.com/) framework is the de-facto industry standard multi-language package manager for Git pre-commit hooks. Teams using Terraform, OpenTofu, Helmfile, and Atmos rely on pre-commit to shift security scanning, documentation formatting, and linting left into local developer environments.

### Current Friction

1. **Ad-Hoc `local` Hooks**: Today, users who want to run Atmos validation or documentation generation on commit must manually write custom `local` repo hooks in their `.pre-commit-config.yaml` using `language: system`, invoking raw bash commands or complex flag invocations. Even Atmos's own repository does this today:
   ```yaml
   - repo: local
     hooks:
       - id: atmos-validate-editorconfig
         name: Validate EditorConfig
         entry: atmos validate --affected --exclude 'tests/fixtures/**' --exclude '**/*.go' --exclude 'README.md' --format rich
         language: system
         pass_filenames: false
         always_run: true
   ```
2. **Missing Remote Provider Contract**: Without a `.pre-commit-hooks.yaml` manifest in `github.com/cloudposse/atmos`, users cannot reference `repo: https://github.com/cloudposse/atmos` with a tagged release (`rev: vX.Y.Z`).
3. **Out-of-Sync Documentation**: Developers frequently edit Terraform component variables, inputs, outputs, or stack definitions without re-running `atmos docs generate readme`, resulting in PRs with out-of-date documentation or CI failures in documentation check steps.
4. **Broken Stack Configurations Committed**: Developers frequently commit invalid YAML, broken inheritance references, or malformed component catalogs that only fail later in CI plan runs or during deployment.
5. **Inconsistent Environment Matrix**: Teams lack a standardized way to ensure all engineers run identical versions of Atmos checks without forcing local manual installations before every commit.

---

## Goals and Non-Goals

### Primary Goals

1. **Official Pre-Commit Manifest**: Ship an official, maintained `.pre-commit-hooks.yaml` in the root of `github.com/cloudposse/atmos`.
2. **Turnkey Hook Catalog**: Provide curated, high-value hooks covering:
   - Documentation generation (`atmos docs generate readme`).
   - Stack manifest validation (`atmos validate stacks`).
   - Comprehensive project validation (`atmos validate`).
   - EditorConfig compliance (`atmos validate editorconfig`).
   - Component schema and policy validation (`atmos validate component`).
   - Terraform component linting (`atmos terraform lint`).
   - Vendoring consistency checks (`atmos vendor diff`).
3. **Dual Runtime Support**:
   - **`language: golang`**: Hermetic, self-compiling execution using pre-commit's built-in Go virtual environment (`go install ./...`), pinned to the specified Git revision `rev: vX.Y.Z`.
   - **`language: system`**: Fast execution delegating to the locally installed `atmos` binary (via Aqua, Homebrew, Devbox, or container runtime).
4. **Git-Aware Scoping**: Seamlessly support both `--affected` (validating only files changed since the Git merge-base) and staged file filtering.
5. **Zero-Configuration Degradation**: Seamlessly format terminal output via `pkg/io/` and `pkg/ui/` so errors are readable both in interactive terminals and non-TTY pre-commit logs.
6. **Dogfooding**: Update the Atmos repository's own `.pre-commit-config.yaml` to consume the standard Atmos hooks.

### Non-Goals

1. **Replacing Pre-Commit**: Atmos is not building its own Git hook manager; this feature enables Atmos to integrate seamlessly with the existing Python-based `pre-commit` ecosystem.
2. **Slow End-to-End Operations in Hooks**: Operations requiring live cloud authentication, remote state locking, or full `terraform plan`/`apply` will not be bundled as default pre-commit hooks, preserving sub-second to low-second commit responsiveness.

---

## Architecture & Design Specification

### 1. Repository Manifest: `.pre-commit-hooks.yaml`

A top-level manifest `.pre-commit-hooks.yaml` will be added to the repository root. Pre-commit reads this file when a downstream repository specifies `repo: https://github.com/cloudposse/atmos`.

#### Hook Definition Catalog

```yaml
# .pre-commit-hooks.yaml
- id: atmos-generate-docs
  name: Atmos Generate Documentation
  description: Automatically generate component and stack README documentation.
  entry: atmos docs generate readme
  language: golang
  pass_filenames: false
  files: ^(stacks/.*|components/.*|atmos\.yaml|README\.yaml)$

- id: atmos-validate-stacks
  name: Atmos Validate Stacks
  description: Validate stack manifest configurations against Atmos schemas and inheritance rules.
  entry: atmos validate stacks --affected
  language: golang
  pass_filenames: false
  always_run: false
  files: ^stacks/.*\.ya?ml$

- id: atmos-validate
  name: Atmos Validate Project
  description: Comprehensive validation of Atmos configuration schema, stack manifests, and workflows.
  entry: atmos validate --affected
  language: golang
  pass_filenames: false
  always_run: true

- id: atmos-validate-editorconfig
  name: Atmos Validate EditorConfig
  description: Validate non-Go files against EditorConfig rules using Atmos native validator.
  entry: atmos validate editorconfig --affected
  language: golang
  pass_filenames: false
  always_run: true

- id: atmos-terraform-lint
  name: Atmos Terraform Lint
  description: Run TFLint on Terraform components configured in Atmos stacks.
  entry: atmos terraform lint
  language: golang
  pass_filenames: false
  files: ^components/terraform/.*

- id: atmos-vendor-diff
  name: Atmos Vendor Diff
  description: Ensure vendored components have not drifted from their vendor specifications.
  entry: atmos vendor diff
  language: golang
  pass_filenames: false
  files: ^(components/.*|vendor\.yaml)$

# System-language variants (for environments using pre-installed Atmos binaries)
- id: atmos-generate-docs-system
  name: Atmos Generate Documentation (System)
  description: Generate documentation using the system-installed atmos binary.
  entry: atmos docs generate readme
  language: system
  pass_filenames: false
  files: ^(stacks/.*|components/.*|atmos\.yaml|README\.yaml)$

- id: atmos-validate-stacks-system
  name: Atmos Validate Stacks (System)
  description: Validate stack manifests using the system-installed atmos binary.
  entry: atmos validate stacks --affected
  language: system
  pass_filenames: false
  files: ^stacks/.*\.ya?ml$

- id: atmos-validate-system
  name: Atmos Validate Project (System)
  description: Comprehensive project validation using the system-installed atmos binary.
  entry: atmos validate --affected
  language: system
  pass_filenames: false
  always_run: true
```

---

### 2. Execution Modes: `language: golang` vs `language: system`

| Consideration | `language: golang` | `language: system` |
| :--- | :--- | :--- |
| **Installation** | Managed entirely by `pre-commit` via `go install ./...` in an isolated cache directory (`~/.cache/pre-commit/`). | Requires `atmos` pre-installed on the developer host or in the container. |
| **Prerequisites** | Go toolchain installed on host. | Any installation method (Homebrew, Aqua, Devbox, Docker, GitHub Actions). |
| **Hermeticity** | Exact version locked to `rev: vX.Y.Z` in `.pre-commit-config.yaml`. | Uses whichever binary is first in `$PATH`. |
| **First-Run Latency** | ~10-15s (one-time compilation per `rev`). Subsequent runs are instantaneous. | Instantaneous (no compile step). |
| **Best For** | Standard Go/IaC developer setups and teams seeking strictly reproducible versions. | CI containers, developers managing tools via Aqua/Brew, or environments without Go. |

Both variants will be supported:
- The default hooks (`atmos-generate-docs`, `atmos-validate-stacks`, etc.) use `language: golang` for maximum portability and version fidelity.
- Companion `-system` hooks (`atmos-generate-docs-system`, `atmos-validate-stacks-system`, `atmos-validate-system`) use `language: system` for zero-overhead local toolchains.

---

### 3. File Scoping, `--affected`, and Working Directory

#### Git Staged vs. Affected Scoping

Pre-commit hooks typically operate in one of two modes:
1. **File-list passed via CLI (`pass_filenames: true`)**: Pre-commit appends staged file paths to the command.
2. **Repository/Index-level inspection (`pass_filenames: false`)**: The command handles discovery internally.

For Atmos, `pass_filenames: false` combined with `--affected` is the optimal design:
- Stack manifests frequently rely on deep multi-file inheritance (`import: [ catalog/*, mixins/* ]`). Validating only an isolated file passed as an argument can miss upstream broken imports.
- `atmos validate --affected` and `atmos validate stacks --affected` internally compute affected stacks using the Git merge-base (`pkg/validation/affected.go`).
- The `files:` regex filter in `.pre-commit-hooks.yaml` ensures that hooks only trigger when relevant files (e.g., `^stacks/.*\.ya?ml$`) are staged.

#### Base Path Resolution

Per the [Base Path Resolution Semantics PRD](./base-path-resolution-semantics.md), `atmos` discovers the repository root by locating `atmos.yaml` or traversing up to the Git root. Pre-commit executes hooks from the root of the repository where `.pre-commit-config.yaml` is located, guaranteeing consistent base-path resolution for all stack and component lookups.

---

### 4. Output Formatting & Terminal Degradation

Pre-commit captures stdout and stderr from hooks and only displays output when a hook fails (non-zero exit code) or when output is modified:

- **Clean Streaming**: Atmos separates data streams (`pkg/io/`) from UI messages (`pkg/ui/`).
- **TTY Auto-Detection**: When executed under pre-commit, stdout is not a TTY. Atmos's zero-configuration degradation automatically strips interactive spinners and TrueColor sequences while preserving clean diagnostics.
- **Rich Diagnostics**: By utilizing `ATMOS_VALIDATION_FORMAT=plain` or leveraging Atmos's error builders (`errors/`), pre-commit outputs clear file, line, and column pointers for manifest errors.

---

### 5. Exit Code Contract

In adherence to the [Exit Codes PRD](./exit-codes.md):
- **Exit Code 0**: Check passed cleanly. If `atmos docs generate readme` re-generated a file without error, and pre-commit detects a staged modification, pre-commit will mark the commit as interrupted so the user can inspect the changes.
- **Exit Code 1**: Validation failure, schema violation, or lint error. Pre-commit halts the commit and prints the diagnostic report.
- **Exit Code 2**: CLI configuration or invalid flag syntax error.

---

## Configuration & Usage Examples

### Example 1: Standard Configuration (`language: golang`)

In a consumer infrastructure repository's `.pre-commit-config.yaml`:

```yaml
repos:
  - repo: https://github.com/cloudposse/atmos
    rev: v1.227.0
    hooks:
      # Automatically regenerate component/stack READMEs
      - id: atmos-generate-docs

      # Validate stack manifests against schema and inheritance rules
      - id: atmos-validate-stacks

      # Validate non-Go files against .editorconfig
      - id: atmos-validate-editorconfig
```

### Example 2: System-Installed Atmos (e.g. via Aqua / Homebrew)

For developers who manage Atmos via Aqua or Homebrew and want instant execution without compiling Go:

```yaml
repos:
  - repo: https://github.com/cloudposse/atmos
    rev: v1.227.0
    hooks:
      - id: atmos-validate-stacks-system
      - id: atmos-generate-docs-system
```

### Example 3: Advanced Monorepo Setup with Custom Args

```yaml
repos:
  - repo: https://github.com/cloudposse/atmos
    rev: v1.227.0
    hooks:
      - id: atmos-validate-stacks
        args: [--exclude, "stacks/experimental/**"]

      - id: atmos-terraform-lint
        args: [--error-mode, strict]
```

---

## Dogfooding in the Atmos Repository

The Atmos repository itself will dogfood this implementation by updating `.pre-commit-config.yaml`:

**Before:**
```yaml
- repo: local
  hooks:
    - id: atmos-validate-editorconfig
      name: Validate EditorConfig
      entry: atmos validate --affected --exclude 'tests/fixtures/**' --exclude '**/*.go' --exclude 'README.md' --format rich
      language: system
      pass_filenames: false
      always_run: true
```

**After:**
```yaml
- repo: local
  hooks:
    - id: atmos-validate-editorconfig
      name: Validate EditorConfig
      entry: atmos validate editorconfig --affected --exclude 'tests/fixtures/**' --exclude '**/*.go' --exclude 'README.md' --format rich
      language: system
      pass_filenames: false
      always_run: true
```
*(And in testing/CI, validating the packaged `.pre-commit-hooks.yaml` directly against the current branch using `pre-commit try-repo .`).*

---

## Implementation Plan

### Phase 1: Core Hook Manifest Definition
1. Create `.pre-commit-hooks.yaml` in the root of the repository.
2. Define primary hooks: `atmos-generate-docs`, `atmos-validate-stacks`, `atmos-validate`, and `atmos-validate-editorconfig`.
3. Define companion `-system` hooks for environments utilizing pre-installed binaries.
4. Verify local installation with `pre-commit try-repo . atmos-validate-stacks`.

### Phase 2: Specialized Hooks & File Filtering Refinements
1. Add `atmos-terraform-lint` and `atmos-vendor-diff` hooks.
2. Fine-tune regex patterns (`files:` and `exclude:`) to ensure hooks do not trigger on unrelated changes (e.g., Markdown edits shouldn't trigger stack validation unless stacks are modified).
3. Ensure `--affected` flag works properly when called from pre-commit subshells where `git rev-parse --show-toplevel` is active.

### Phase 3: CI Integration & Pre-Commit Validation Test Suite
1. Add a GitHub Actions workflow job or test case in `.github/workflows/test.yml` running `pre-commit run --all-files` or `pre-commit try-repo .`.
2. Ensure Go build caching works correctly when pre-commit compiles `atmos` with `language: golang`.

### Phase 4: Documentation & Community Templates
1. Document pre-commit hook integration in `website/docs/integrations/pre-commit.md`.
2. Include pre-commit configuration in `atmos init` templates (`cmd/init/init.go` and `pkg/generator`).
3. Add a section in `CLAUDE.md` and `docs/development.md` explaining hook maintenance.

---

## Summary of Related PRDs

- [Custom Hooks](./custom-hooks.md) — Atmos lifecycle hooks for component events (`before/after.terraform.*`).
- [GitOps Enablement](./git-ops.md) — Git operations and local Git hook management.
- [Exit Codes](./exit-codes.md) — Standardized CLI exit codes and error wrapping.
- [Base Path Resolution Semantics](./base-path-resolution-semantics.md) — Directory resolution for monorepos and nested components.
- [Native CI Integration](./native-ci-integration.md) — Consistency between local hooks and CI pipeline validation.
