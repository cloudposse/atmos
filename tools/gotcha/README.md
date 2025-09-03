# gotcha

A sophisticated Go test runner with real-time progress tracking, beautiful terminal output, and flexible result formatting. Run tests directly with streaming output, or process existing JSON results from `go test -json`.

## Overview

gotcha provides two main modes:

### Stream Mode (Default)
- **Real-time test execution** with beautiful progress bars and spinners
- **Live progress tracking** with test count and elapsed time display
- **Auto-discovery** of git repository root and config files
- **Package filtering** with regex include/exclude patterns
- **Pass-through arguments** to `go test` with `--` syntax

### Parse Mode
- **Process existing JSON** from `go test -json` output
- **Multiple output formats** (markdown, GitHub step summaries)
- **Coverage analysis** with visual badges and detailed reports
- **GitHub Actions integration** for PR comments and step summaries

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

# Combine gotcha flags with go test flags
gotcha stream --show=failed -- -run "Test.*Load" -v

# Generate GitHub step summaries
gotcha stream --format=github --output=step-summary.md
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

#### Parse Mode Flags
- `--input`: Input file (JSON from `go test -json`). Use `-` or omit for stdin
- `--format`: Output format: `stdin`, `markdown`, `both`, `github` (default: `stdin`)
- `--output`: Output file (default: stdout for stdin/markdown)
- `--coverprofile`: Coverage profile file for detailed analysis
- `--exclude-mocks`: Exclude mock files from coverage calculations (default: `true`)
- `--post-comment`: Post test summary as GitHub PR comment (default: `false`)
- `--generate-summary`: Write test summary to test-summary.md file (default: `false`)
- `--github-token`: GitHub token for authentication (defaults to GITHUB_TOKEN env)
- `--comment-uuid`: UUID for comment identification (defaults to GOTCHA_COMMENT_UUID env)

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

## GitHub Actions Integration

Example workflow usage:

```yaml
- name: Run tests with coverage
  run: go test -json -cover ./... > test-output.json

- name: Generate test summary
  env:
    GOTCHA_COMMENT_UUID: "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f"
  run: |
    ./tools/gotcha/gotcha test-output.json \
      -output-file gotcha.md \
      -github-step-summary \
      -shields-io-badge \
      -show-package-coverage \
      -pr-filtered-coverage
```

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