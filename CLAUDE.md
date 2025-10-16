# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Atmos is a sophisticated Go CLI tool for managing complex cloud infrastructure using Terraform. It provides:
- **Stack-based configuration management** with hierarchical YAML configs
- **Multi-cloud orchestration** for Terraform, Helmfile, and Packer
- **Component architecture** for reusable infrastructure patterns
- **Advanced templating** with Go templates and Gomplate functions
- **Workflow orchestration** for complex deployment pipelines
- **Policy validation** using OPA and JSON Schema
- **Vendoring system** for external components
- **Terminal UI** with rich interactive components

## Essential Commands

### Development Workflow
```bash
# Build the project
make build                   # Build default binary to ./build/atmos
make build-linux             # Build for Linux
make build-windows           # Build for Windows
make build-macos             # Build for macOS

# Testing
make testacc                 # Run acceptance tests
make testacc-cover           # Run tests with coverage
make testacc-coverage        # Generate coverage HTML report

# Code Quality
make lint                    # Run golangci-lint (only files changed from origin/main)
make get                     # Download dependencies

# Version and validation
./build/atmos version        # Test built binary
make version                 # Build and test version
```

### Key Atmos Commands
```bash
# Core functionality
atmos terraform plan <component> -s <stack>
atmos terraform apply <component> -s <stack>
atmos describe component <component> -s <stack>
atmos list components
atmos list stacks
atmos workflow <workflow> -f <file>

# Validation and schema
atmos validate stacks
atmos validate component <component> -s <stack>
atmos validate schema [<schema-key>]
atmos validate schema --schemas-atmos-manifest <path-to-schema>

# Vendoring and dependencies
atmos vendor pull
# atmos vendor diff  # Not currently registered
```

## Architecture Overview

### Core Package Structure
- **`cmd/`** - Cobra CLI command definitions, each command in separate file
- **`internal/exec/`** - Core business logic and orchestration engine
- **`pkg/`** - Reusable packages organized by domain:
  - `config/` - Configuration loading, parsing, and merging
  - `stack/` - Stack processing and inheritance logic
  - `component/` - Component lifecycle management
  - `utils/` - Shared utilities (YAML, JSON, file operations)
  - `validate/` - Schema and policy validation
  - `workflow/` - Workflow orchestration
  - `hooks/` - Event-driven hooks system
  - `telemetry/` - Usage analytics and reporting

### Key Architectural Concepts

**Stack Processing Pipeline**:
1. Load base configuration (`atmos.yaml`)
2. Process stack imports and inheritance hierarchy
3. Apply component configurations and overrides
4. Render templates with context-aware functions
5. Generate final component configuration

**Template System**:
- Go text/template with custom functions
- Gomplate integration for advanced templating
- Context-aware functions: `atmos.Component()`, `terraform.output()`, etc.
- Store integration for runtime secret resolution

**Component Lifecycle**:
- Discovery via filesystem scanning
- Configuration merging from multiple sources
- Variable file generation for tools (Terraform, Helmfile)
- Backend configuration generation
- Execution orchestration with progress tracking

## Code Patterns & Conventions

### Comment Style (MANDATORY)
- **All comments must end with periods** - Comments should be complete sentences ending with a period
- This is enforced by golangci-lint's `godot` linter and will cause build failures if not followed
- Applies to all comments: single-line, multi-line, inline, and documentation comments
- This rule ensures consistency and readability across the codebase
- Always treat comments as complete sentences rather than sentence fragments
- Examples:
  ```go
  // CORRECT: This function processes the input data.
  // WRONG: This function processes the input data
  ```

### Import Organization (MANDATORY)
- **Group imports into three sections** separated by blank lines:
  1. **Go native imports** - Standard library packages (fmt, os, strings, etc.)
  2. **3rd-party imports** - External packages from github.com, gopkg.in, etc. (NOT github.com/cloudposse/atmos)
  3. **Atmos imports** - Packages from github.com/cloudposse/atmos
- **Sort alphabetically within each group** - Ignore alias prefixes when sorting
- **Maintain import aliases** - Keep existing aliases like `cfg`, `log`, `u`, `errUtils`, etc.
- Examples:
  ```go
  // CORRECT: Three groups, sorted alphabetically
  import (
      "errors"
      "fmt"
      "strings"

      "github.com/go-git/go-git/v5/plumbing"
      giturl "github.com/kubescape/go-git-url"
      "github.com/spf13/cobra"
      "github.com/spf13/pflag"

      errUtils "github.com/cloudposse/atmos/errors"
      "github.com/cloudposse/atmos/internal/tui/templates/term"
      cfg "github.com/cloudposse/atmos/pkg/config"
      log "github.com/cloudposse/atmos/pkg/logger"
      "github.com/cloudposse/atmos/pkg/perf"
      "github.com/cloudposse/atmos/pkg/schema"
      u "github.com/cloudposse/atmos/pkg/utils"
  )

  // WRONG: Mixed groups, not sorted
  import (
      "errors"
      "fmt"

      "github.com/cloudposse/atmos/pkg/perf"

      log "github.com/cloudposse/atmos/pkg/logger"
      "github.com/go-git/go-git/v5/plumbing"
      "github.com/spf13/cobra"

      errUtils "github.com/cloudposse/atmos/errors"
      cfg "github.com/cloudposse/atmos/pkg/config"
  )
  ```

### Performance Tracking (MANDATORY)
- **Add `defer perf.Track()` to all public functions** and critical private functions
- **Always include a blank line after the perf.Track call** to separate instrumentation from logic
- **Package prefix naming:** Use `"packagename.FunctionName"` format
- **Parameter usage:**
  - Use `atmosConfig` parameter if the function has it
  - Use `nil` if the function doesn't have `atmosConfig` parameter
- Examples:
  ```go
  // CORRECT: With atmosConfig parameter and blank line
  func ProcessComponent(atmosConfig *schema.AtmosConfiguration, component string) error {
      defer perf.Track(atmosConfig, "exec.ProcessComponent")()

      // Function logic starts here
      return nil
  }

  // CORRECT: Without atmosConfig parameter and blank line
  func ValidateInput(input string) error {
      defer perf.Track(nil, "utils.ValidateInput")()

      // Function logic starts here
      return nil
  }

  // WRONG: Missing blank line after perf.Track
  func ProcessComponent(atmosConfig *schema.AtmosConfiguration, component string) error {
      defer perf.Track(atmosConfig, "exec.ProcessComponent")()
      // Function logic starts here (no blank line)
      return nil
  }

  // WRONG: Incorrect package prefix
  func ProcessComponent(atmosConfig *schema.AtmosConfiguration, component string) error {
      defer perf.Track(atmosConfig, "ProcessComponent")() // Missing package prefix

      return nil
  }
  ```

### Configuration Loading
Configuration follows strict precedence: CLI flags → ENV vars → config files → defaults
```go
// Use Viper for configuration management
viper.SetConfigName("atmos")
viper.AddConfigPath(".")
viper.AutomaticEnv()
viper.SetEnvPrefix("ATMOS")
```

### Error Handling (MANDATORY)
- **All errors MUST be wrapped using static errors defined in `errors/errors.go`**
- **Use `errors.Join` for combining multiple errors** - preserves all error chains
- **Use `fmt.Errorf` with `%w` for adding string context** - when you need formatted strings
- **Use `errors.Is()` for error checking** - robust against wrapping
- **NEVER use dynamic errors directly** - triggers linting warnings
- **See `docs/prd/error-handling-strategy.md`** for complete guidelines

#### Error Handling Patterns

**Combining Multiple Errors:**
```go
// ✅ CORRECT: Use errors.Join (unlimited errors, no formatting)
return errors.Join(errUtils.ErrFailedToProcess, underlyingErr)

// Note: Go 1.20+ supports fmt.Errorf("%w: %w", ...) but errors.Join is preferred
```

**Adding String Context:**
```go
return fmt.Errorf("%w: component=%s stack=%s", errUtils.ErrInvalidComponent, component, stack)
```

**Checking Errors:**
```go
// ✅ CORRECT: Works with wrapped errors
if errors.Is(err, context.DeadlineExceeded) { ... }

// ❌ WRONG: Breaks with wrapping
if err.Error() == "context deadline exceeded" { ... }
```

**Static Error Definitions:**
```go
// Define in errors/errors.go
var (
    ErrInvalidComponent = errors.New("invalid component")
    ErrInvalidStack     = errors.New("invalid stack")
)
```

### Testing Strategy
- **Unit tests**: Focus on pure functions, use table-driven tests
- **Integration tests**: Test command flows end-to-end using `tests/` fixtures
- **Mock interfaces**: Use generated mocks for external dependencies
- Target >80% coverage, especially for `pkg/` and `internal/exec/`
- **Comments must end with periods**: All comments should be complete sentences ending with a period (enforced by golangci-lint)

### Test Quality (MANDATORY)
- **Test behavior, not implementation** - Verify inputs/outputs, not internal state
- **Never test stub functions** - Either implement the function or remove the test
- **Avoid tautological tests** - Don't test that hardcoded stubs return hardcoded values
- **Make code testable** - Use dependency injection to avoid hard dependencies on `os.Exit`, `CheckErrorPrintAndExit`, or external systems
- **No coverage theater** - Each test must validate real behavior, not inflate metrics
- **Remove always-skipped tests** - Either fix the underlying issue or delete the test
- **Table-driven tests need real scenarios** - Use production-like inputs, not contrived data

### Test Skipping Conventions (MANDATORY)
- **ALWAYS use `t.Skipf()` instead of `t.Skip()`** - Provide clear reasons for skipped tests
- **NEVER use `t.Skipf()` without a reason**
- Examples:
  ```go
  // WRONG: No reason provided
  t.Skipf("Skipping test")

  // CORRECT: Clear reason with context
  t.Skipf("Skipping symlink test on Windows: symlinks require special privileges")
  t.Skipf("Skipping test: %s", dynamicReason)
  ```
- **For CLI tests**:
  - Tests automatically build a temporary binary for each test run
  - When coverage is disabled: builds a regular binary
  - When coverage is enabled (GOCOVERDIR set): builds with coverage instrumentation
  - TestMain MUST call `os.Exit(m.Run())` to propagate the test exit code

### CLI Command Structure & Examples
Atmos uses **embedded markdown files** for maintainable examples:

```go
//go:embed markdown/atmos_example_usage.md
var exampleUsageMarkdown string

// Follow this pattern for new commands
var exampleCmd = &cobra.Command{
    Use:   "example [component] -s [stack]",
    Short: "Brief description with **markdown** formatting",
    Long: `Detailed description with context using markdown formatting.

Use **bold** for emphasis and \`code\` for technical terms.
Supports multiple paragraphs and formatting.`,
    // Examples are loaded from embedded markdown files
    RunE: func(cmd *cobra.Command, args []string) error {
        // Validate inputs
        // Call pkg/ or internal/exec/ functions
        // Handle and format output
        return nil
    },
}
```

**Example File Convention** (`cmd/markdown/atmos_example_usage.md`):
```markdown
- Basic usage

\`\`\`
$ atmos example <component> -s <stack>
\`\`\`

- With output format

\`\`\`
$ atmos example <component> -s <stack> --format=yaml|json
\`\`\`

- Write result to file

\`\`\`
$ atmos example <component> -s <stack> --file output.yaml
\`\`\`
```

**Usage System**:
- Examples auto-load from `cmd/markdown/*_usage.md` files via `//go:embed`
- Use `utils.PrintfMarkdown()` to render markdown content
- Register examples in `cmd/markdown_help.go` `examples` map with suggestion URLs
- File naming: `atmos_<command>_<subcommand>_usage.md`

### File Organization (MANDATORY)
- **Prefer many small files over few large files** - follow Go idiom of focused, single-purpose files
- **NEVER use `//revive:disable:file-length-limit`** - split large files instead
  - Default limit: 600 lines (enforced by golangci-lint)
  - For new code: ALWAYS split into focused files if approaching limit
  - Example split pattern:
    ```go
    // formatter.go (733 lines) → Split into:
    formatter_table.go  // Table rendering (236 lines)
    formatter_tree.go   // Tree rendering (451 lines)
    formatter_utils.go  // Shared utilities (68 lines)
    ```
- **One command per file** in `cmd/`
- **One implementation per file** for interfaces:
  ```go
  // pkg/store/
  store.go              // Interface definition
  aws_ssm_store.go     // AWS SSM implementation
  azure_keyvault_store.go // Azure implementation
  google_secretmanager_store.go // Google implementation
  ```
- **Test file naming symmetry** - test files mirror implementation structure:
  ```go
  // Implementation files
  aws_ssm_store.go
  azure_keyvault_store.go

  // Corresponding test files
  aws_ssm_store_test.go
  azure_keyvault_store_test.go
  ```
- **Group related functionality** in `pkg/` subpackages by domain
- **Co-locate tests** with implementation (`_test.go` alongside `.go` files)
- **Mock files alongside interfaces** they mock
- **Shared test utilities** in `tests/` directory for integration tests

## Template Functions

Atmos provides extensive template functions available in stack configurations:

### Core Functions
- `atmos.Component(component, stack)` - Get component configuration
- `atmos.Stack(stack)` - Get stack configuration
- `atmos.Setting(path)` - Get setting from atmos.yaml

### Integration Functions
- `terraform.output(component, stack, output)` - Get Terraform output
- `terraform.state(component, stack, path)` - Query Terraform state
- `exec(command, args...)` - Execute shell commands
- `env(var)` - Get environment variable

### Store Functions (Runtime Secret Resolution)
- `store.get(type, key)` - Get value from external store
- Supports: AWS SSM, Azure Key Vault, Google Secret Manager, Redis, Artifactory
- **See `pkg/store/registry.go`** for the authoritative list of supported store providers

## Testing Guidelines

### Test Strategy with Preconditions
Atmos uses **precondition-based test skipping** to provide a better developer experience. Tests check for required preconditions (AWS profiles, network access, Git configuration) and skip gracefully with helpful messages rather than failing. See:
- **[Testing Strategy PRD](docs/prd/testing-strategy.md)** - Complete design document
- **[Tests README](tests/README.md)** - Practical testing guide with examples
- **[Test Preconditions](tests/test_preconditions.go)** - Helper functions for precondition checks

### Running Tests
```bash
# Quick tests only (skip long-running tests >2s)
make test-short
go test -short ./...

# All tests including long-running ones
make testacc
go test ./...

# With coverage
make test-short-cover
make testacc-cover

# Bypass all precondition checks
export ATMOS_TEST_SKIP_PRECONDITION_CHECKS=true
go test ./...

# Run with verbose output to see skips
go test -v ./...
```

### Test File Locations
- Unit tests: `pkg/**/*_test.go`
- Integration tests: `tests/**/*_test.go` with fixtures in `tests/test-cases/`
- Command tests: `cmd/**/*_test.go`
- Test helpers: `tests/test_preconditions.go`

### Writing Tests with Preconditions
```go
func TestAWSFeature(t *testing.T) {
    // Check AWS precondition at test start
    tests.RequireAWSProfile(t, "profile-name")
    // ... test code
}

func TestGitHubVendoring(t *testing.T) {
    // Check GitHub access with rate limits
    rateLimits := tests.RequireGitHubAccess(t)
    if rateLimits != nil && rateLimits.Remaining < 20 {
        t.Skipf("Need at least 20 GitHub API requests, only %d remaining", rateLimits.Remaining)
    }
    // ... test code
}
```

### Running Specific Tests
```bash
# Run specific test
go test ./pkg/config -run TestConfigLoad
# Run with coverage
go test ./pkg/config -cover
# Integration tests
go test ./tests -run TestCLI
```

### Test Data
Use fixtures in `tests/test-cases/` for integration tests. Each test case should have:
- `atmos.yaml` - Configuration
- `stacks/` - Stack definitions
- `components/` - Component configurations

### Golden Snapshots (MANDATORY)
- **NEVER modify files under `tests/test-cases/` or `tests/testdata/`** unless explicitly instructed
- These directories contain golden snapshots that are sensitive to even minor changes
- Golden snapshots are used to verify expected output remains consistent
- If you need to update golden snapshots, do so intentionally and document the reason

## Common Development Tasks

### Adding New CLI Command

**Atmos uses the command registry pattern** for organizing built-in commands. Follow these steps:

1. **Create command package directory**: `cmd/[command]/`
2. **Implement CommandProvider interface**:
   ```go
   // cmd/mycommand/mycommand.go
   package mycommand

   import (
       "github.com/spf13/cobra"
       "github.com/cloudposse/atmos/cmd/internal"
       e "github.com/cloudposse/atmos/internal/exec"
   )

   var mycommandCmd = &cobra.Command{
       Use:   "mycommand",
       Short: "Brief description",
       Long:  `Detailed description.`,
       RunE: func(cmd *cobra.Command, args []string) error {
           return e.ExecuteMyCommand(cmd, args)
       },
   }

   func init() {
       internal.Register(&MyCommandProvider{})
   }

   type MyCommandProvider struct{}

   func (m *MyCommandProvider) GetCommand() *cobra.Command {
       return mycommandCmd
   }

   func (m *MyCommandProvider) GetName() string {
       return "mycommand"
   }

   func (m *MyCommandProvider) GetGroup() string {
       return "Other Commands" // See docs/developing-atmos-commands.md
   }
   ```
3. **Add blank import to `cmd/root.go`**: `_ "github.com/cloudposse/atmos/cmd/mycommand"`
4. **Implement business logic** in `internal/exec/mycommand.go`
5. **Add tests** in `cmd/mycommand/mycommand_test.go`
6. **Create Docusaurus documentation** in `website/docs/cli/commands/<command>/<subcommand>.mdx`
7. **Build website to verify**: `cd website && npm run build`
8. **Create pull request following template format**

**See:**
- **[docs/developing-atmos-commands.md](docs/developing-atmos-commands.md)** - Complete guide with patterns and examples
- **[docs/prd/command-registry-pattern.md](docs/prd/command-registry-pattern.md)** - Architecture and design decisions

### Documentation Requirements (MANDATORY)
- **All new commands/flags/parameters MUST have Docusaurus documentation**
- **Use definition lists `<dl>` instead of tables** for arguments and flags:
  ```mdx
  ## Arguments

  <dl>
    <dt>`component`</dt>
    <dd>Atmos component name</dd>

    <dt>`stack`</dt>
    <dd>Atmos stack name</dd>
  </dl>

  ## Flags

  <dl>
    <dt>`--stack` / `-s`</dt>
    <dd>Atmos stack (required)</dd>

    <dt>`--format`</dt>
    <dd>Output format: `yaml`, `json`, or `table` (default: `yaml`)</dd>
  </dl>
  ```

- **Follow Docusaurus conventions** from existing files:
  ```mdx
  ---
  title: atmos command subcommand
  sidebar_label: subcommand
  sidebar_class_name: command
  id: subcommand
  description: Brief description of what the command does
  ---
  import Screengrab from '@site/src/components/Screengrab'
  import Terminal from '@site/src/components/Terminal'

  :::note Purpose
  Use this command to [describe purpose with links to concepts].
  :::

  <Screengrab title="atmos command subcommand --help" slug="atmos-command-subcommand--help" />

  ## Usage

  ```shell
  atmos command subcommand <args> [options]
  ```

  ## Examples

  ```shell
  atmos command subcommand example1
  atmos command subcommand example2 --flag=value
  ```
  ```

- **File location**: `website/docs/cli/commands/<command>/<subcommand>.mdx`
- **Link to core concepts** using `/core-concepts/` paths
- **Include purpose note** and help screengrab
- **Use consistent section ordering**: Usage → Examples → Arguments → Flags

### Website Documentation Build (MANDATORY)
- **ALWAYS build the website after any documentation changes** to verify there are no broken links or formatting issues
- **Build command**: Run from the `website/` directory:
  ```bash
  cd website
  npm run build
  ```
- **When to build**:
  - After adding/modifying any `.mdx` or `.md` files in `website/docs/`
  - After adding images to `website/static/img/`
  - After changing navigation in `website/sidebars.js`
  - After modifying any component in `website/src/`
- **What to check**:
  - Build completes without errors
  - No broken links reported
  - No missing images
  - Proper rendering of MDX components
- **Example workflow**:
  ```bash
  # 1. Make documentation changes
  vim website/docs/cli/commands/describe/stacks.mdx

  # 2. Build to verify
  cd website
  npm run build

  # 3. If errors, fix and rebuild
  # 4. Commit changes only after successful build
  ```

### Pull Request Requirements (MANDATORY)
- **Follow the pull request template** in `.github/PULL_REQUEST_TEMPLATE.md`:
  ```markdown
  ## what
  - High-level description of changes in plain English
  - Use bullet points for clarity

  ## why
  - Business justification for the changes
  - Explain why these changes solve the problem
  - Use bullet points for clarity

  ## references
  - Link to supporting GitHub issues or documentation
  - Use `closes #123` if PR closes an issue
  ```
- **Add changelog blog post for feature releases**:
  - PRs labeled `minor` or `major` MUST include a blog post in `website/blog/`
  - Create a new file: `website/blog/YYYY-MM-DD-feature-name.md`
  - Follow the format of existing blog posts (see template below)
  - Include `<!--truncate-->` marker after the introduction paragraph
  - The CI workflow will fail and comment on the PR if this is missing

### Blog Post Guidelines (MANDATORY)

Blog posts serve different audiences and must be tagged appropriately:

#### Audience Types

**1. User-Facing Posts** (Features, Improvements, Bug Fixes)
- **Audience**: Teams using Atmos to manage infrastructure
- **Focus**: How the change benefits users, usage examples, migration guides
- **Required tags**: Choose one or more:
  - `feature` - New user-facing capabilities
  - `enhancement` - Improvements to existing features
  - `bugfix` - Important bug fixes that affect users
- **Example tags**: `[feature, terraform, workflows]`

**2. Contributor-Facing Posts** (Refactoring, Internal Changes, Developer Tools)
- **Audience**: Atmos contributors and core developers
- **Focus**: Internal code structure, refactoring, developer experience
- **Required tag**: `contributors`
- **Additional tags**: Describe the technical area
- **Example tags**: `[contributors, atmos-core, refactoring]`

#### Blog Post Template

```markdown
---
slug: descriptive-slug
title: "Clear, Descriptive Title"
authors: [atmos]
tags: [primary-tag, secondary-tag, ...]  # See audience types above
---

Brief introduction paragraph explaining what changed and why it matters.

<!--truncate-->

## What Changed

Describe the change with code examples or visuals.

## Why This Matters / Impact on Users

Explain the benefits or reasoning.

## [For User Posts] How to Use It

Provide practical examples and usage instructions.

## [For Contributor Posts] For Atmos Contributors

Clarify this is internal with zero user impact, link to technical docs.

## Get Involved

- Link to relevant documentation
- Encourage discussion/contributions
```

#### Tag Reference

**Primary Audience Tags:**
- `feature` - New user-facing feature
- `enhancement` - Improvement to existing feature
- `bugfix` - Important bug fix
- `contributors` - For Atmos core contributors (internal changes)

**Secondary Technical Tags (for contributor posts):**
- `atmos-core` - Changes to Atmos codebase/internals
- `refactoring` - Code refactoring and restructuring
- `testing` - Test infrastructure improvements
- `ci-cd` - CI/CD pipeline changes
- `developer-experience` - Developer tooling improvements

**Secondary Technical Tags (for user posts):**
- `terraform` - Terraform-specific features
- `helmfile` - Helmfile-specific features
- `workflows` - Workflow features
- `validation` - Validation features
- `performance` - Performance improvements
- `cloud-architecture` - Cloud architecture patterns (user-facing)

**General Tags:**
- `announcements` - Major announcements
- `breaking-changes` - Breaking changes requiring migration

- **Use `no-release` label for documentation-only changes**
- **Ensure all CI checks pass** before requesting review

### Checking PR Security Alerts and CI Status
Use the GitHub CLI (`gh`) to inspect PR checks and security alerts:

```bash
# View PR checks status
gh pr checks {pr-number} --repo {owner/repo}

# Get check run annotations for a specific check (e.g., linting issues)
gh api repos/{owner/repo}/check-runs/{check-run-id}/annotations

# Get code scanning alerts for the repository
gh api repos/{owner/repo}/code-scanning/alerts

# Example for Atmos repository:
gh pr checks 1450 --repo cloudposse/atmos
gh api repos/cloudposse/atmos/check-runs/49737026433/annotations
```

### Responding to PR Review Threads (MANDATORY)
- **ALWAYS reply to specific review threads** - Do not create new PR comments
- **Use GraphQL API to reply to threads**:
  ```bash
  gh api graphql -f query='
  mutation {
    addPullRequestReviewThreadReply(input: {
      pullRequestReviewThreadId: "PRRT_kwDOEW4XoM5..."
      body: "Your response here"
    }) {
      comment { id }
    }
  }'
  ```
- Get unresolved threads:
  ```bash
  gh api graphql -f query='
  query {
    repository(owner: "cloudposse", name: "atmos") {
      pullRequest(number: 1504) {
        reviewThreads(first: 50) {
          nodes {
            id
            isResolved
            path
            line
            comments(first: 1) {
              nodes { body }
            }
          }
        }
      }
    }
  }' | jq -r '.data.repository.pullRequest.reviewThreads.nodes[] | select(.isResolved == false)'
  ```

### Adding Template Function
1. Implement in `internal/exec/template_funcs.go`
2. Register in template function map
3. Add comprehensive tests
4. Document in website if user-facing

### Bug Fixing Workflow (MANDATORY)
1. **Write a test to reproduce the bug** - create failing test that demonstrates the issue
2. **Run the test to confirm it fails** - verify the test reproduces the expected behavior
3. **Fix the bug iteratively** - make changes and re-run test until it passes
4. **Verify fix doesn't break existing functionality** - run full test suite

```go
// Example: Test should describe the expected behavior, not that it's a bug fix
func TestParseConfig_HandlesEmptyStringInput(t *testing.T) {
    // Setup conditions that reproduce the issue
    input := ""

    // Call the function that should handle this case
    result, err := ParseConfig(input)

    // Assert the expected behavior (this should initially fail)
    assert.NoError(t, err)
    assert.Equal(t, DefaultConfig, result)
}

// Or for error conditions:
func TestValidateStack_ReturnsErrorForInvalidFormat(t *testing.T) {
    invalidStack := "malformed-stack-config"

    err := ValidateStack(invalidStack)

    assert.Error(t, err)
    assert.Contains(t, err.Error(), "invalid format")
}
```

### Extending Store Integration
1. Implement interface in `pkg/store/`
2. Add to store registry
3. Update configuration schema
4. Add integration tests with mocks

### Stack Processing Changes
1. Core logic in `pkg/stack/` and `internal/exec/stack_processor_utils.go`
2. Test with multiple inheritance scenarios
3. Validate template rendering still works
4. Update schema if configuration changes

## Critical Development Requirements

### Test Coverage (MANDATORY)
- **80% minimum coverage** on new/changed lines (enforced by CodeCov)
- ALL new features MUST include comprehensive unit tests
- Integration tests required for CLI commands using `tests/` fixtures
- Tests exclude mock files: `**/mock_*.go`, `mock_*.go`, `**/mock/*.go`
- Run `make testacc-coverage` to generate coverage reports

### Environment Variable Conventions (MANDATORY)
- **ALWAYS use `viper.BindEnv()`** for environment variable binding
- **EVERY env var MUST have an ATMOS_ alternative**:
  ```go
  // WRONG: Only binding external env var
  viper.BindEnv("GITHUB_TOKEN")

  // CORRECT: Provide Atmos alternative
  viper.BindEnv("ATMOS_GITHUB_TOKEN", "GITHUB_TOKEN")
  viper.BindEnv("ATMOS_PRO_TOKEN", "ATMOS_PRO_TOKEN")
  ```

### Structured Logging vs UI Output (MANDATORY)
- **Distinguish between logging and UI output**:
  ```go
  // WRONG: Using logging for user interface
  log.Info("Enter your password:")
  log.Error("Invalid input, please try again")

  // CORRECT: Use UI output for user interaction
  fmt.Fprintf(os.Stderr, "Enter your password: ")
  fmt.Fprintf(os.Stderr, "❌ Invalid input, please try again\n")

  // CORRECT: Use logging for system/debug information
  log.Debug("Processing authentication", "user", username)
  log.Error("Authentication failed", "error", err, "user", username)
  ```

- **UI Output Rules**:
  - User prompts, status messages, progress indicators → stderr
  - Error messages requiring user action → stderr
  - Data/results for piping → stdout
  - **Never use logging for UI elements**

- **Logging Rules**:
  - System events, debugging, error tracking → logging
  - **Logging should not affect execution** - disabling logging completely should not break functionality
  - Use structured logging without message interpolation
  - Follow logging hierarchy: `LogFatal > LogError > LogWarn > LogDebug > LogTrace`
  - Use appropriate levels per `docs/logging.md` guidance
  - Production should have LogError/LogWarn enabled, Debug/Trace disabled

### Output Conventions (MANDATORY)
- **Most text UI MUST go to stderr** to enable proper piping
- **Only data/results go to stdout** for piping compatibility
- **Examples**:
  ```go
  import "github.com/cloudposse/atmos/pkg/utils"

  // WRONG: UI to stdout (breaks piping)
  fmt.Println("Processing component...")
  fmt.Print(componentData)

  // CORRECT: Use TUI function for UI messages, stdout for data
  utils.PrintfMessageToTUI("Processing component...\n")
  fmt.Print(componentData) // Data goes to stdout for piping

  // ACCEPTABLE: Direct stderr as last resort
  fmt.Fprintf(os.Stderr, "Processing component...\n")
  fmt.Print(componentData) // Data goes to stdout for piping
  ```

### Schema Updates (MANDATORY)
- **Update ALL schema files** when adding Atmos configuration options:
  - `/pkg/datafetcher/schema/config/global/1.0.json`
  - `/pkg/datafetcher/schema/atmos/manifest/1.0.json`
  - `/pkg/datafetcher/schema/stacks/stack-config/1.0.json`
  - `/pkg/datafetcher/schema/vendor/package/1.0.json`
- Validate schema changes don't break existing configurations

### Styling & Theme (MANDATORY)
- **Use consistent Atmos theme colors** from `pkg/ui/theme/colors.go`:
  - Success: `ColorGreen` (#00FF00)
  - Info: `ColorCyan` (#00FFFF)
  - Error: `ColorRed` (#FF0000)
  - Selected: `ColorSelectedItem` (#10ff10)
  - Border: `ColorBorder` (#5F5FD7)
- Use `theme.Styles` and `theme.Colors` for consistent formatting

### Template Integration (MANDATORY)
- **All new configs MUST support Go templating** using existing utilities
- Use `FuncMap()` from `internal/exec/template_funcs.go` for template functions
- Available template functions:
  ```go
  // In YAML configurations
  {{ atmos.Component "vpc" "dev" }}
  {{ atmos.Store "ssm" "prod" "app" "secret_key" }}
  {{ atmos.GomplateDatasource "data" }}
  ```
- Test template rendering with various contexts

### Code Reuse (MANDATORY)
- **Search for existing methods** before implementing new functionality
- Look for patterns in `internal/exec/` and `pkg/` that can be refactored
- Prefer extending existing utilities over creating duplicates
- Common reusable patterns:
  - File operations: `pkg/utils/file_utils.go`
  - YAML processing: `pkg/utils/yaml_utils.go`
  - Component processing: `internal/exec/component_utils.go`
  - Stack processing: `internal/exec/stack_processor_utils.go`

### Cross-Platform Compatibility (MANDATORY)
- **Atmos MUST work on Linux, macOS, and Windows** - write portable implementations
- **Prefer SDKs over calling binaries** when available:
  ```go
  // WRONG: Calling external binary (platform-specific)
  cmd := exec.Command("terraform", "plan")

  // CORRECT: Using SDK (cross-platform)
  import "github.com/hashicorp/terraform-exec/tfexec"
  tf, err := tfexec.NewTerraform(workingDir, execPath)
  ```
- Use Go's standard library for cross-platform operations:
  - `filepath.Join()` instead of hard-coded path separators
  - `os.PathSeparator` and `os.PathListSeparator` for paths
  - `runtime.GOOS` for OS-specific logic when unavoidable
- Test on all supported platforms or use build constraints when necessary
- Handle platform differences in file permissions, path lengths, and environment variables

### Multi-Provider Interface Pattern (MANDATORY)
- **Create interfaces for multi-provider functionality**:
  ```go
  // Define interface for the capability
  type SecretStore interface {
      Get(ctx context.Context, key string) (string, error)
      Put(ctx context.Context, key, value string) error
      Delete(ctx context.Context, key string) error
  }

  // Implement for each provider
  type AWSSSMStore struct { /* ... */ }
  type AzureKeyVaultStore struct { /* ... */ }
  type GoogleSecretManagerStore struct { /* ... */ }

  func (s *AWSSSMStore) Get(ctx context.Context, key string) (string, error) {
      // AWS SSM implementation
  }
  ```

- **Generate mocks for all interfaces** (no cloud connectivity required for tests):
  ```go
  //go:generate mockgen -source=secret_store.go -destination=mock_secret_store.go

  func TestSecretProcessing(t *testing.T) {
      mockStore := NewMockSecretStore(ctrl)
      mockStore.EXPECT().Get(gomock.Any(), "test-key").Return("test-value", nil)
      // Test logic without real cloud calls
  }
  ```

- **Provider registry pattern**:
  ```go
  type ProviderRegistry struct {
      stores map[string]SecretStore
  }

  func (r *ProviderRegistry) Register(name string, store SecretStore) {
      r.stores[name] = store
  }

  func (r *ProviderRegistry) Get(name string) (SecretStore, error) {
      // Return appropriate implementation
  }
  ```

- **Examples in codebase**: `pkg/store/` (AWS SSM, Azure Key Vault, Google Secret Manager)

### Telemetry Integration (MANDATORY)
- **New commands automatically get telemetry** via `RootCmd.ExecuteC()` in `cmd/root.go:174`
- **No additional telemetry code needed** for standard Cobra commands added to RootCmd
- **For non-standard execution paths**, use:
  ```go
  import "github.com/cloudposse/atmos/pkg/telemetry"

  // For cobra commands
  telemetry.CaptureCmd(cmd, err)

  // For command strings
  telemetry.CaptureCmdString("command name", err)
  ```
- **Never capture user data** - only command paths and error states (boolean)

## Development Environment

### Prerequisites
- Go 1.24+ (see go.mod for exact version)
- golangci-lint for linting
- Make for build automation

### IDE Configuration
The project includes Cursor rules in `.cursor/rules/atmos-rules.mdc` covering:
- Code structure and patterns
- Testing requirements
- Documentation standards
- Quality checks and linting

### Build Process
- CGO disabled for static binaries
- Cross-platform builds supported
- Version injected at build time via ldflags
- Binary output to `./build/` directory

### Compilation Requirements (MANDATORY)
- **ALWAYS compile after making changes** - Run `go build` after ANY code modification
- **Verify no compilation errors** before proceeding with further changes or commits
- **Run tests to ensure functionality** - Execute `go test ./...` (tests handle binary requirements automatically)
- **Never assume code changes work** without compilation verification
- **Use compile-and-test pattern**: `go build . && go test ./... 2>&1`
- **Fix compilation errors immediately** - Do not proceed with additional changes until compilation succeeds
- **This prevents undefined function/variable errors** that waste time and create broken commits

### Pre-commit Checks (MANDATORY)
- **NEVER use `--no-verify` flag** when committing - pre-commit hooks must always run
- **Fix all linting errors** before committing - run `make lint` to check
- **Pre-commit hooks ensure code quality** - they run go-fumpt, golangci-lint, and go mod tidy
- **If pre-commit fails, fix the issues** - do not bypass with --no-verify
- **Run `make lint` before committing** to catch issues early
- **All commits must pass pre-commit checks** to maintain code standards
