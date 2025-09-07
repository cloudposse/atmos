# Development Guide

This guide covers the development workflow for contributing to Atmos.

## Prerequisites

- Go 1.24+ (see go.mod for exact version)
- Make
- Git

## Quick Start

1. Clone the repository
2. Run the development setup:
   ```bash
   atmos dev setup
   ```
   This will:
   - Install Go dependencies
   - Install pre-commit and golangci-lint (using brew, apt, or pip)
   - Set up pre-commit hooks
   - Install required Go tools

## Development Workflow

We use Atmos custom commands for development (dogfooding our own tool). This ensures we experience the same workflows our users do. See [Atmos Custom Commands](https://atmos.tools/core-concepts/custom-commands/) for more details.

### Available Commands

```bash
atmos dev help         # Show all available dev commands
atmos dev check        # Run pre-commit hooks on staged files
atmos dev check-all    # Run pre-commit hooks on all files
atmos dev lint         # Run golangci-lint
atmos dev test         # Run tests
atmos dev build        # Build the Atmos binary
atmos dev quick        # Quick build and test
atmos dev fix          # Auto-fix Go formatting issues
```

### Alternative Make Commands

Traditional make commands are also available:

```bash
make build             # Build the Atmos binary
make test              # Run tests
make lint              # Run golangci-lint on changed files
```

## Pre-commit Hooks

We use pre-commit hooks to ensure code quality. The following hooks run automatically on `git commit`:

### Go-specific Hooks
- **go-fumpt**: Enforces consistent Go formatting (stricter than gofmt)
- **go-build-mod**: Verifies code compiles
- **go-mod-tidy**: Ensures go.mod and go.sum are clean
- **golangci-lint**: Comprehensive linting to prevent massive functions and files

### General Hooks
- **trailing-whitespace**: Removes trailing whitespace
- **end-of-file-fixer**: Ensures files end with a newline
- **check-yaml**: Validates YAML syntax
- **check-added-large-files**: Prevents committing large files (>500KB)

## golangci-lint

golangci-lint is critical for maintaining code quality. It runs both locally (via pre-commit) and in CI. The linter enforces:

- Function complexity limits
- File size limits
- Code style consistency
- Bug detection
- Performance improvements

Configuration is in `.golangci.yml`.

## CI/CD

Pull requests trigger the pre-commit workflow which:
1. Runs all pre-commit hooks on changed files only (not the entire codebase)
2. Ensures code quality before merge

## Troubleshooting

### Pre-commit Hooks Not Running

If hooks aren't running on commit:
```bash
pre-commit install
```

### golangci-lint Issues

If golangci-lint fails with Go version issues:
```bash
# Reinstall with brew (recommended)
brew upgrade golangci-lint

# Or use the installer
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin
```

### Manual Pre-commit Control

```bash
# Skip hooks for a single commit (use sparingly)
git commit --no-verify -m "message"

# Run hooks manually
pre-commit run --all-files

# Update hooks to latest versions
pre-commit autoupdate
```

## Configuration Files

- `.pre-commit-config.yaml` - Pre-commit hooks configuration
- `.golangci.yml` - golangci-lint rules
- `.github/workflows/pre-commit.yml` - CI workflow for pre-commit
- `.atmos.d/dev.yaml` - Atmos custom development commands
