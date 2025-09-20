# gotcha

A sophisticated Go test runner with real-time progress tracking, beautiful terminal output, and flexible result formatting. Run tests directly with streaming output, or process existing JSON results from `go test -json`.

## Overview

gotcha transforms the Go testing experience by providing intuitive visual feedback, comprehensive CI/CD integration, and intelligent test result analysis. Built with modern Go libraries, it offers real-time progress tracking, multi-platform VCS support, and flexible configuration options.

### Operation Modes

gotcha provides two main modes:

### Stream Mode (Default)
- **Real-time test execution** with automatic TTY detection and graceful degradation
- **Interactive TUI mode** - Beautiful progress bars, spinners, and live updates when TTY is detected
- **Headless mode** - Clean streaming output for CI environments and non-TTY terminals
- **Smart test filtering** with automatic test name detection
- **Package visualization** with clear headers and "No tests" indication
- **Subtest analysis** with inline pass/fail statistics
- **Multi-platform CI integration** with flexible comment posting strategies
- **Pass-through arguments** to `go test` with `--` syntax

### Parse Mode
- **Process existing JSON** from `go test -json` output
- **Multiple output formats** (terminal, markdown, GitHub step summaries)
- **Coverage analysis** with visual badges and detailed reports
- **VCS platform integration** for automated PR/MR comments

## Environment Detection & Modes

### Automatic TTY Detection

Gotcha automatically detects your environment and switches between modes for optimal user experience:

- **Interactive TUI Mode** - When running in a TTY (terminal):
  - Beautiful progress bars with real-time completion percentage
  - Animated spinners during test execution
  - Live test counters and elapsed time
  - Interactive visual feedback

- **Headless Mode** - When running in CI or non-TTY environments:
  - Clean streaming output without interactive elements
  - CI-friendly logging and structured output
  - Same information hierarchy, optimized for parsing
  - Consistent filtering and display logic

### Manual Mode Control

```bash
# Force TTY mode (interactive)
export GOTCHA_FORCE_TTY=true
gotcha stream

# Force non-TTY mode (headless)
export GOTCHA_FORCE_NO_TTY=true  
gotcha stream

# Or use explicit flags (coming soon)
gotcha stream --force-tty
gotcha stream --no-tty
```

### Graceful Degradation

- **Color support**: Automatically detects terminal capabilities (TrueColor → ANSI256 → ANSI → NoColor)
- **CI environments**: Automatically uses headless mode in GitHub Actions, GitLab CI, etc.
- **Piped output**: Maintains colors by default, respects `NO_COLOR` environment variable
- **Consistent behavior**: Both modes show identical information with appropriate formatting

## Key Features

- **Real-time Progress Tracking** - Interactive TUI with live test counts, elapsed time, and progress visualization
- **Multi-Platform CI/CD Integration** - Native support for GitHub Actions with GitLab, Bitbucket, and Azure DevOps planned
- **Flexible Configuration** - YAML config files, environment variables, and CLI flags with clear precedence
- **Smart Test Filtering** - Automatic test name detection and `-run` flag application
- **Advanced Subtest Visualization** - Inline pass/fail statistics and detailed breakdown on failures
- **Package Organization** - Clear package headers with "No tests" indication for empty packages
- **Multiple Output Formats** - Terminal, markdown, and GitHub-specific formatting
- **Coverage Analysis** - Detailed reports with mock file exclusion and visual indicators
- **Performance Optimization** - Intelligent caching system for faster startup and progress bars
- **Comment Posting Strategies** - Adaptive, failure-only, skip-only, and platform-specific posting
- **Cross-Platform Compatibility** - Consistent behavior on Linux, macOS, and Windows
- **Automatic Environment Detection** - TTY-aware with graceful degradation from interactive TUI to headless mode

## Installation

### Via go install (Recommended)

```bash
go install github.com/cloudposse/atmos/tools/gotcha@latest
```

### Build from source (Development)

```bash
cd tools/gotcha
go build -o gotcha .
```

## Quick Start

```bash
# Run all tests with real-time progress (root command)
gotcha

# Explicit stream mode
gotcha stream

# Run specific test by name (automatic -run flag)
gotcha stream TestMyFunction
gotcha stream TestConfigLoad TestStackProcess

# Generate GitHub job summary in CI
gotcha stream --post-comment=adaptive --generate-summary

# Process existing test results
go test -json ./... | gotcha parse --format=markdown

# Run with custom configuration
gotcha stream --config=.gotcha-ci.yaml --show=failed
```

## Usage

### Stream Mode Examples

Run tests directly with real-time output:

```bash
# Run all tests with default settings
gotcha

# Test specific packages
gotcha ./pkg/utils ./internal/...

# Show only failed tests with custom timeout  
gotcha stream --show=failed --timeout=10m

# Apply package filters
gotcha stream --include=".*api.*" --exclude=".*mock.*"

# Pass arguments to go test
gotcha stream -- -race -short -count=3

# Run specific tests using -run flag
gotcha stream -- -run TestConfigLoad
gotcha stream -- -run "TestConfig.*"
gotcha stream -- -run TestStackProcess -race

# Configure coverage packages (useful for monorepos)
gotcha stream --coverprofile=coverage.out -- -coverpkg=github.com/cloudposse/atmos/...
gotcha stream --coverprofile=coverage.out -- -coverpkg=./pkg/...,./internal/...

# Combine gotcha flags with go test flags
gotcha stream --show=failed -- -run "Test.*Load" -v

# Generate GitHub step summaries with adaptive posting
gotcha stream --format=github --output=step-summary.md --post-comment=adaptive

# CI-friendly with job discriminator
gotcha stream --post-comment=adaptive --generate-summary --log-level=warn
```

### Smart Test Filtering

Gotcha automatically detects test names in command arguments and applies appropriate filters:

```bash
# Automatic test filtering (detects test names and adds -run flag)
gotcha stream TestExecute           # → go test ./... -run TestExecute
gotcha stream TestA TestB           # → go test ./... -run "TestA|TestB"
gotcha stream ./pkg TestConfig      # → go test ./pkg/... -run TestConfig

# Subtest support
gotcha stream TestFoo/subtest       # → go test ./... -run "TestFoo/subtest"

# Mixed package paths and test names
gotcha stream ./internal TestLoad TestSave  # → go test ./internal/... -run "TestLoad|TestSave"

# Explicit paths still work normally
gotcha stream ./...                 # → go test ./...
gotcha stream ./pkg/utils           # → go test ./pkg/utils/...

# Pass-through arguments override automatic filtering
gotcha stream -- -run TestSpecific -v  # Respects explicit -run flag

# Root command also supports smart filtering
gotcha TestExecute                  # → go test ./... -run TestExecute
gotcha ./pkg TestConfig             # → go test ./pkg/... -run TestConfig
```

### Parse Mode Examples

Process existing `go test -json` output:

```bash
# Process results from stdin
go test -json ./... | gotcha parse

# Process results from file
gotcha parse test-results.json
gotcha parse --input=results.json --format=markdown

# Generate GitHub step summary
gotcha parse --format=github --output=step-summary.md

# Include coverage analysis
gotcha parse --coverprofile=coverage.out --format=both
```

### Command Line Flags

#### Stream Mode Flags
- `--packages`: Space-separated packages to test (default: `./...`)
- `--show`: Filter displayed tests: `all`, `failed`, `passed`, `skipped` (default: `all`)
- `--timeout`: Test timeout duration (default: `40m`)
- `--output`: Output file for JSON results (default: `test-results.json` in temp dir)
- `--coverprofile`: Coverage profile file for detailed analysis
- `--include`: Regex patterns to include packages (comma-separated, default: `.*`)
- `--exclude`: Regex patterns to exclude packages (comma-separated)
- `--post-comment`: Comment posting strategy: `always`, `never`, `adaptive`, `on-failure`, `on-skip`, or platform name
- `--generate-summary`: Write test summary to summary file (default: `false`)
- `--vcs-platform`: Force specific VCS platform: `github`, `gitlab`, `bitbucket`, `azuredevops`

#### Parse Mode Flags
- `--input`: Input file (JSON from `go test -json`). Use `-` or omit for stdin
- `--format`: Output format: `terminal`, `markdown`, `both`, `github` (default: `terminal`)
- `--output`: Output file (default: stdout for terminal/markdown)
- `--coverprofile`: Coverage profile file for detailed analysis
- `--exclude-mocks`: Exclude mock files from coverage calculations (default: `true`)
- `--post-comment`: Comment posting strategy (same options as stream mode)
- `--generate-summary`: Write test summary to summary file (default: `false`)
- `--github-token`: GitHub token for authentication (defaults to GITHUB_TOKEN env)
- `--comment-uuid`: UUID for comment identification (defaults to GOTCHA_COMMENT_UUID env)
- `--vcs-platform`: Force specific VCS platform (same options as stream mode)

#### Global Flags
- `--log-level`: Set logging level: `debug`, `info`, `warn`, `error`, `fatal` (default: `info`)
- `--no-color`: Disable colored output
- `--config`: Path to custom configuration file (default: search for `.gotcha.yaml`)
- `--no-cache`: Skip cache for current run

## Coverage Analysis

Gotcha provides integrated coverage analysis that automatically runs after tests complete, eliminating the need for separate `go tool cover` commands.

### Automatic Coverage Analysis

When you run tests with `--coverprofile`, Gotcha automatically:
1. Collects coverage data during test execution
2. Analyzes function-level coverage using `go tool cover -func`
3. Displays a comprehensive coverage summary
4. Shows uncovered functions to help identify gaps
5. Checks coverage thresholds (if configured)

### Example Output

```bash
$ gotcha stream ./pkg/utils --coverprofile=coverage.out

✔ All 101 tests passed (77.8% statement coverage)

INFO  Analyzing coverage results...

📊 Function Coverage Summary:
github.com/cloudposse/atmos/tools/gotcha/pkg/utils/helpers.go:17:      IsValidShowFilter       100.0%
github.com/cloudposse/atmos/tools/gotcha/pkg/utils/helpers.go:28:      FilterPackages          100.0%
github.com/cloudposse/atmos/tools/gotcha/pkg/utils/helpers.go:101:     IsTTY                   0.0%
github.com/cloudposse/atmos/tools/gotcha/pkg/utils/test_detection.go:16: IsLikelyTestName      71.4%
...
total:                                                                  (statements)            77.8%

📊 Function Coverage Summary:
   Total Functions: 13
   Average Coverage: 73.2%
   Uncovered Functions: 3

   🔴 Top Uncovered Functions:
      • IsTTY in .../utils/helpers.go
      • EmitAlert in .../utils/helpers.go
      • ShortPackage in .../utils/utils.go

📈 Statement Coverage: 77.8%
```

### Coverage Configuration

Configure coverage behavior in `.gotcha.yaml`:

```yaml
coverage:
  enabled: true
  profile: coverage.out
  
  analysis:
    functions: true      # Show function-level coverage
    statements: true     # Show statement coverage summary
    uncovered: false     # Show only uncovered functions
    exclude:
      - "**/mock*.go"    # Exclude mock files
      - "**/*_test.go"   # Exclude test files
  
  output:
    terminal:
      format: summary    # summary, detailed, or none
      show_uncovered: 10 # Number of uncovered functions to show
  
  thresholds:
    total: 80           # Minimum required coverage percentage
    fail_under: true    # Fail tests if below threshold
```

### Coverage Thresholds

Enforce minimum coverage requirements:

```yaml
coverage:
  thresholds:
    total: 80           # Require 80% overall coverage
    fail_under: true    # Exit with error if threshold not met
```

When enabled, Gotcha will fail the test run if coverage falls below the threshold, making it easy to maintain coverage standards in CI/CD pipelines.

### Excluding Files from Coverage

Control which files are included in coverage analysis:

```yaml
coverage:
  analysis:
    exclude:
      - "**/mock*.go"        # Exclude all mock files
      - "**/*_test.go"       # Exclude test files
      - "**/generated/*.go"  # Exclude generated code
```

## Configuration

### Configuration Precedence

Gotcha uses a clear configuration hierarchy with the following precedence (highest to lowest):

1. **Command line flags** (highest priority)
2. **Environment variables** (with `GOTCHA_` prefix support)
3. **Configuration file** (`.gotcha.yaml`)
4. **Built-in defaults** (lowest priority)

### Configuration File (.gotcha.yaml)

Gotcha automatically searches for `.gotcha.yaml` configuration files in the current directory and parent directories. You can also specify a custom config file with the `--config` flag.

#### Complete Configuration Schema

```yaml
# Output verbosity level
# Options: standard, with-output, minimal, verbose
#   - standard: Default output, no test output shown
#   - with-output: Shows complete stdout/stderr output for failed tests (recommended)
#   - minimal: Only show essential information
#   - verbose: Maximum detail including output for all tests
verbosity: with-output

# Show filter for test display
# Options: all, failed, passed, skipped, collapsed, none
#   - all: Show all tests
#   - failed: Only show failed tests
#   - passed: Only show passed tests
#   - skipped: Only show skipped tests
#   - collapsed: Show tests in collapsed format
#   - none: Don't show individual tests, only summary
show: all

# Alert when tests complete
# Set to true to emit a terminal bell when tests finish
alert: false

# Log level for debugging
# Options: debug, info, warn, error, fatal
log:
  level: info

# Test execution settings
test:
  # Timeout for tests (e.g., "30m", "1h")
  timeout: 30m
  
  # Number of parallel test executions
  # Set to 0 for default (number of CPUs)
  parallel: 0

# Coverage settings
coverage:
  # Enable coverage collection and analysis
  enabled: true
  
  # Coverage profile output file
  profile: coverage.out
  
  # Coverage analysis options
  analysis:
    # Run 'go tool cover -func' after tests to show function coverage
    functions: true
    
    # Show statement coverage summary
    statements: true
    
    # Show only uncovered code (helpful for finding gaps)
    uncovered: false
    
    # Exclude files from analysis (glob patterns)
    exclude:
      - "**/mock*.go"
      - "**/*_test.go"
  
  # Coverage output formats
  output:
    # Terminal output settings
    terminal:
      # Format: summary, detailed, or none
      format: summary
      
      # Show top N uncovered functions
      show_uncovered: 10
  
  # Coverage thresholds
  thresholds:
    # Overall coverage threshold (0 = disabled)
    total: 0
    
    # Fail tests if below threshold
    fail_under: false

# GitHub integration
github:
  # Enable GitHub Actions specific features
  # NOTE: This enables GitHub Actions features when running in GitHub Actions environment
  actions: true
  
  # Create step summary when running in GitHub Actions
  step_summary: true

# TUI (Terminal User Interface) settings
tui:
  # Enable terminal bell alerts in TUI mode
  alert: true
  
  # Color profile for terminal output
  # Options: auto, always, never
  colors: auto

# VCS (Version Control System) integration
vcs:
  # Platform: github, gitlab, bitbucket, azuredevops
  platform: github
  
  # Comment posting strategy
  # Options: always, never, adaptive, on-failure, on-skip, <platform>
  post-comment: adaptive
  
  # Generate job summary (for GitHub Actions)
  generate-summary: true

# Performance settings
cache:
  # Enable test count caching
  enabled: true
  
  # Cache expiration time
  max-age: 24h

# Package filtering
filter:
  # Regular expressions to include packages
  include:
    - ".*"
  
  # Regular expressions to exclude packages
  exclude: []

# Additional test arguments
# These are passed directly to 'go test'
testargs: ""

# Output file for test results
output: ""

# Exclude mock files from coverage
exclude-mocks: true
```

### Environment Variables

All configuration options can be set via environment variables using the `GOTCHA_` prefix.

| Variable | Description | Default |
|----------|-------------|---------|
| **General** | | |
| `GOTCHA_LOG_LEVEL` | Logging level (debug, info, warn, error, fatal) | `info` |
| `GOTCHA_VERBOSITY` | Output verbosity (standard, with-output, minimal, verbose) | `with-output` |
| `GOTCHA_SHOW` | Filter displayed tests (all, failed, passed, skipped, collapsed, none) | `all` |
| `GOTCHA_ALERT` | Enable terminal bell on completion | `false` |
| `GOTCHA_OUTPUT` | Output file path | - |
| **Coverage** | | |
| `GOTCHA_COVERAGE_ENABLED` | Enable coverage analysis | `true` |
| `GOTCHA_COVERAGE_PROFILE` | Coverage profile output file | `coverage.out` |
| `GOTCHA_COVERAGE_ANALYSIS_FUNCTIONS` | Show function coverage | `true` |
| `GOTCHA_COVERAGE_ANALYSIS_STATEMENTS` | Show statement coverage | `true` |
| `GOTCHA_COVERAGE_ANALYSIS_UNCOVERED` | Show only uncovered code | `false` |
| `GOTCHA_COVERAGE_OUTPUT_TERMINAL_FORMAT` | Terminal output format (summary, detailed, none) | `summary` |
| `GOTCHA_COVERAGE_OUTPUT_TERMINAL_SHOW_UNCOVERED` | Number of uncovered functions to show | `10` |
| `GOTCHA_COVERAGE_THRESHOLDS_TOTAL` | Overall coverage threshold | `0` |
| `GOTCHA_COVERAGE_THRESHOLDS_FAIL_UNDER` | Fail if below threshold | `false` |
| `GOTCHA_EXCLUDE_MOCKS` | Exclude mock files from coverage | `true` |
| **VCS Integration** | | |
| `GOTCHA_COMMENT_UUID` | UUID for comment deduplication | - |
| `GOTCHA_POST_COMMENT` | Comment posting strategy | `adaptive` |
| `GOTCHA_JOB_DISCRIMINATOR` | Unique identifier for multi-job CI | - |
| `GOTCHA_VCS_PLATFORM` | Force specific VCS platform | auto-detect |
| **GitHub Integration** | | |
| `GITHUB_TOKEN` | GitHub authentication token | - |
| `GITHUB_STEP_SUMMARY` | Path to GitHub step summary file | - |
| `GITHUB_ACTIONS` | GitHub Actions environment detection | - |
| **Terminal Control** | | |
| `GOTCHA_FORCE_TTY` | Force TTY mode | `false` |
| `GOTCHA_FORCE_NO_TTY` | Force non-TTY mode | `false` |
| `NO_COLOR` | Disable colors (standard convention) | `false` |
| `FORCE_COLOR` | Force color output (1=ANSI, 2=ANSI256, 3=TrueColor) | - |
| **Testing** | | |
| `GOTCHA_USE_MOCK` | Use mock VCS provider for testing | `false` |
| **CI Detection** | | |
| `CI` | CI environment detection | - |

### Custom Configuration File

```bash
# Use custom configuration file
gotcha stream --config=/path/to/custom-config.yaml

# Configuration file for CI environment
gotcha --config=.gotcha-ci.yaml --show=failed
```

## Input Format

The tool expects JSON output from `go test -json`. Generate this with:

```bash
go test -json -cover ./... > test-output.json
```

## Output Features

### Test Results Summary
- Total tests run, passed, failed, and skipped
- Overall test duration
- Package-level results breakdown

### Coverage Reports
- Overall coverage percentage
- Per-package coverage details (with `-show-package-coverage`)
- Filtered coverage for PR changes (with `-pr-filtered-coverage`)

### Visual Elements
- Shields.io badges for test status and coverage (with `-shields-io-badge`)
- Emoji indicators for pass/fail status
- Formatted tables for easy reading

### GitHub Integration
- GitHub Step Summary output (with `-github-step-summary`)
- PR comment-ready markdown format
- UUID-based comment deduplication

## Environment Variables

### `GOTCHA_COMMENT_UUID`
Used for GitHub comment deduplication. When set, adds an invisible HTML comment to the output:
```html
<!-- test-summary-uuid: your-uuid-here -->
```

This allows GitHub Actions to update existing comments instead of creating new ones.

### `GITHUB_STEP_SUMMARY`
Automatically used when `-github-step-summary` flag is provided. Points to the file where GitHub Actions step summaries are written.

## CI/CD Integration

### VCS Platform Support

Gotcha supports multiple VCS platforms through a provider-based architecture:

- **GitHub Actions** - Full support for PR comments and job summaries
- **GitLab CI** - Coming soon
- **Bitbucket Pipelines** - Coming soon  
- **Azure DevOps** - Coming soon

Force a specific platform:
```bash
gotcha stream --vcs-platform=github
# or
export GOTCHA_VCS_PLATFORM=github
```

### GitHub Actions

#### Basic Setup

```yaml
- name: Run tests with gotcha
  run: |
    gotcha stream --post-comment=adaptive --generate-summary --coverprofile=coverage.out
```

#### Multi-Platform CI with Job Discriminator

```yaml
strategy:
  matrix:
    os: [ubuntu-latest, windows-latest, macos-latest]
    
steps:
  - name: Run tests
    env:
      GOTCHA_JOB_DISCRIMINATOR: ${{ matrix.os }}
      GOTCHA_POST_COMMENT: adaptive
    run: |
      gotcha stream --generate-summary --coverprofile=coverage.out
```

#### Advanced Workflow Example

```yaml
- name: Run tests with coverage
  run: go test -json -cover ./... > test-output.json

- name: Generate test summary
  env:
    GOTCHA_COMMENT_UUID: "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f"
    GOTCHA_JOB_DISCRIMINATOR: ${{ matrix.target }}
  run: |
    gotcha parse test-output.json \
      --format=both \
      --post-comment=adaptive \
      --generate-summary \
      --coverprofile=coverage.out
```

### Comment Posting Strategies

The `--post-comment` flag supports multiple strategies for controlling when CI comments are posted:

| Strategy | Behavior | Use Case |
|----------|----------|----------|
| `always` | Always post comment regardless of results or platform | CI jobs that should always report status |
| `never` | Never post comment | Local development or testing |
| `adaptive` | Linux always posts, other platforms only on failures/skips | Multi-platform CI optimization (recommended) |
| `on-failure` | Only post when tests fail | Minimize noise, focus on problems |
| `on-skip` | Only post when tests are skipped | Track incomplete test coverage |
| `linux`/`darwin`/`windows` | Only post on specific OS | Platform-specific reporting |

#### Examples

```bash
# Always post (default when flag is present)
gotcha stream --post-comment

# Explicit strategies
gotcha stream --post-comment=always
gotcha stream --post-comment=adaptive  # Recommended for multi-platform CI
gotcha stream --post-comment=on-failure
gotcha stream --post-comment=linux

# Environment variable support
export GOTCHA_POST_COMMENT=adaptive
gotcha stream  # Uses adaptive strategy
```

### Multi-Job Comment Handling

For CI workflows with multiple jobs (different OS, Go versions, etc.), use job discriminators to create separate comments:

```yaml
env:
  GOTCHA_JOB_DISCRIMINATOR: ${{ matrix.os }}-go${{ matrix.go-version }}
  GOTCHA_POST_COMMENT: adaptive
```

This creates unique comments for each job combination while using adaptive posting strategy.

### Platform Detection

Gotcha automatically detects the platform using:
- `runtime.GOOS` for reliable OS detection
- `RUNNER_OS` environment variable for display purposes
- `GITHUB_ACTIONS`, `GITLAB_CI`, etc. for CI environment detection

## Visual Features

### Test Output Formatting

Gotcha provides rich visual feedback during test execution with a carefully designed information hierarchy:

#### Package Headers
- **Format**: `▶ package.name` with blue bold styling
- **Purpose**: Clear visual separation between packages in multi-package test runs
- **Display**: Shows when entering a new package context
- **No tests indication**: Shows `No tests` in gray when package has no runnable tests

#### Test Status Symbols
- ✔ **Pass**: Green color for immediate success identification
- ✘ **Fail**: Red color for immediate failure identification  
- ⊘ **Skip**: Amber/orange color for skipped test identification

#### Subtest Visualization
- **Inline Summary**: Parent tests show `[X/Y passed]` format for quick subtest overview
- **Detailed Breakdown**: Failed parent tests display comprehensive subtest results
- **Real-time Updates**: Subtest counts update as tests complete

#### Visual Hierarchy
1. **Test Status Symbols** (Highest priority) - Immediately visible pass/fail/skip indicators
2. **Test Names** (Secondary priority) - Light gray for readability without competing with status
3. **Duration/Metadata** (Tertiary priority) - Dark gray, available when needed but de-emphasized
4. **Package Headers** (Context/Navigation) - Blue bold for clear package delineation

### Progress Indicators

- **Real-time Progress Bar** (TUI mode) - Visual test completion percentage
- **Test Counters** - Running tally of passed/failed/skipped tests  
- **Elapsed Time** - Live timer showing test execution duration
- **Spinner Animation** - Visual feedback during test execution
- **Mini Progress Dots** - Colored dots representing subtest progress (up to 10 dots max)

### Example Output

```
▶ github.com/cloudposse/atmos/tools/gotcha/pkg/utils
✔ TestPasses (0.01s)
✘ TestFails (0.02s)
⊘ TestSkipped (0.00s)
✘ TestWithSubtests (1.23s) [2/5 passed]
  Passed:
    • ValidInput
    • EdgeCase
  Failed:
    • InvalidInput
    • EmptyInput
  Skipped:
    • ConditionalTest

▶ github.com/cloudposse/atmos/tools/gotcha/pkg/constants
  No tests

▶ github.com/cloudposse/atmos/tools/gotcha/pkg/cache
✔ TestCacheLoad (0.05s) [3/3 passed]
```

### Color Support

#### Environment Detection
- **TrueColor**: Modern terminals with full RGB support
- **ANSI256**: GitHub Actions and most CI environments
- **ANSI**: Basic CI environments (default fallback)
- **NoColor**: Disabled via `--no-color` flag or `NO_COLOR` environment variable

#### Color Control
- **CLI Flag**: `--no-color` disables all color output
- **Environment Variables**: 
  - `NO_COLOR=1` to disable colors globally
  - `FORCE_COLOR=1/2/3` to force ANSI/ANSI256/TrueColor
- **Default Behavior**: Colors enabled by default, even when piping
- **Precedence**: CLI flag > NO_COLOR env > FORCE_COLOR env > terminal detection

## Performance Features

### Caching System

Gotcha implements an intelligent caching system to improve performance and user experience:

#### Cache Benefits
- **Instant Progress Bars** - Use cached test counts for immediate progress visualization
- **Reduced Startup Time** - Skip expensive test discovery in large codebases
- **Historical Tracking** - Analyze test performance trends over time
- **Persistent Preferences** - Remember user settings across sessions

#### Cache Configuration
- **Location**: `.gotcha/cache.yaml` (automatically added to `.gitignore`)
- **Environment Variables**:
  - `GOTCHA_CACHE_ENABLED`: Enable/disable caching (default: `true`)
  - `GOTCHA_CACHE_DIR`: Cache directory location (default: `.gotcha`)
  - `GOTCHA_CACHE_MAX_AGE`: Maximum cache entry age (default: `24h`)
- **CLI Flag**: `--no-cache` to skip cache for current run

#### Cache Invalidation
- **Timestamp-based expiration** - Entries expire after max age
- **go.mod tracking** - Invalidate when go.mod modification time changes  
- **Manual invalidation** - Use `--no-cache` flag when needed
- **Automatic cleanup** - Remove stale entries on save

### Test Completion Timing
- **Completion Display**: "Tests completed in X.XXs" shown as structured log message
- **Precision**: 2 decimal places for seconds
- **Consistent Styling**: Uses duration styling for visual consistency

## Output Examples

### Basic Summary
```markdown
# Test Results

## Summary
- ✅ **Passed**: 156 tests
- ❌ **Failed**: 2 tests  
- ⏭️ **Skipped**: 1 test
- ⏱️ **Duration**: 45.2s

## Failed Tests
- `TestConfigLoad` in `pkg/config`
- `TestStackProcess` in `pkg/stack`
```

### With Shields.io Badges
```markdown
# Test Results

![Tests](https://img.shields.io/badge/tests-156%20passed%2C%202%20failed-red)
![Coverage](https://img.shields.io/badge/coverage-87.5%25-yellow)

[Rest of summary...]
```

### PR Filtered Coverage
Shows coverage only for files that changed in the current PR, helping reviewers focus on new code coverage.

## Error Handling

The tool handles various error conditions gracefully:
- Invalid JSON input
- Missing coverage data
- Git repository issues (for PR filtering)
- File I/O errors

Exit codes:
- `0`: Success
- `1`: Error occurred (details written to stderr)

## Inspirations

gotcha draws inspiration from several excellent Go testing tools:

- **[gotestdox](https://github.com/bitfield/gotestdox)** - BDD-style test runner that converts Go test function names into readable specifications
- **[gotest](https://github.com/rakyll/gotest)** - Colorized `go test` output with enhanced readability and formatting  
- **[gotestsum](https://github.com/gotestyourself/gotestsum)** - Advanced test runner with multiple output formats and JUnit XML support

Each of these tools brings unique strengths to Go testing, and gotcha aims to combine the best aspects: real-time progress tracking, beautiful terminal output, flexible formatting options, and comprehensive test result analysis.

## Technical Notes

Gotcha is built using modern Go libraries and follows established patterns for maintainability and extensibility:

- **Charm Ecosystem**: Uses Bubble Tea for TUI components, Lipgloss for consistent styling, and Charmbracelet Log for structured logging
- **Configuration Management**: Viper provides flexible configuration with support for YAML files, environment variables, and CLI flags
- **VCS Abstraction**: Provider-based architecture enables easy extension to new CI/CD platforms (GitHub, GitLab, Bitbucket, Azure DevOps)
- **Structured Logging**: Context-aware logging with color profile detection and environment-specific formatting
- **Cross-Platform Design**: Uses Go's standard library for portable file operations and platform detection
- **Interface-Driven**: Extensive use of interfaces for testability and future extensibility
- **Performance Optimized**: Intelligent caching, efficient streaming, and minimal memory overhead

The architecture prioritizes modularity, testability, and graceful degradation when dependencies are unavailable.

## Development

### Running Tests
```bash
go test ./...
```

### Coverage Report
```bash
go test -cover ./...
```

### Linting
```bash
golangci-lint run
```