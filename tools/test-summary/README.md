# Test Summary Tool

A Go utility for processing and summarizing Go test output in JSON format. This tool generates comprehensive test reports with coverage information, failure summaries, and GitHub Actions integration.

## Overview

The test-summary tool reads `go test -json` output and produces:
- Formatted test result summaries
- Coverage reports with visual badges
- GitHub Step Summary integration
- PR comment-ready markdown output
- Filtered coverage reports for changed files

## Installation

Build the tool from the project root:

```bash
cd tools/test-summary
go build -o test-summary .
```

## Command Line Arguments

### Required Arguments

- **Input file**: Path to the JSON file containing `go test -json` output

### Optional Flags

- `-output-file string`: Path to write the markdown summary (default: writes to stdout)
- `-github-step-summary`: Write output to GitHub Step Summary (`$GITHUB_STEP_SUMMARY`)
- `-show-package-coverage`: Include per-package coverage details in the output
- `-shields-io-badge`: Generate Shields.io badge URLs for test results and coverage
- `-coverage-report-name string`: Custom name for the coverage report (default: "Test Coverage")
- `-pr-filtered-coverage`: Generate coverage report filtered to files changed in the PR
- `-pr-base-branch string`: Base branch for PR comparison (default: "main")

### Examples

#### Basic usage:
```bash
./test-summary test-output.json
```

#### Write to file:
```bash
./test-summary test-output.json -output-file summary.md
```

#### GitHub Actions integration:
```bash
./test-summary test-output.json -github-step-summary -shields-io-badge
```

#### PR-specific coverage:
```bash
./test-summary test-output.json -pr-filtered-coverage -pr-base-branch origin/main
```

#### Full featured output:
```bash
./test-summary test-output.json \
  -output-file test-summary.md \
  -github-step-summary \
  -show-package-coverage \
  -shields-io-badge \
  -coverage-report-name "Atmos Test Coverage" \
  -pr-filtered-coverage
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

### `TEST_SUMMARY_UUID`
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
    TEST_SUMMARY_UUID: "e7b3c8f2-4d5a-4c9b-8e1f-2a3b4c5d6e7f"
  run: |
    ./tools/test-summary/test-summary test-output.json \
      -output-file test-summary.md \
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