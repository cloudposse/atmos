# PR Command

Create a pull request following Cloud Posse standards.

## Usage

```
/pr [options]
```

## Options

- `--base <branch>`: Target branch for the PR (default: main)
- `--title <title>`: PR title focusing on value/outcome (auto-generated if not provided)
- `--no-release`: Add the no-release label for documentation-only changes
- `--patch`: Add patch label for bug fixes and minor improvements (default for most changes)
- `--minor`: Add minor label for new features that don't break compatibility
- `--major`: Add major label for breaking changes (AVOID - requires strategic planning)

## Examples

```
/pr --base testacc-job-summary --patch
/pr --title "feat: add new feature" --base main --minor
/pr --no-release
/pr --title "fix: resolve bug in parser" --patch
```

## PR Title Best Practices

When using `--title` or when the command generates a title:

**Focus on value, not compliance:**
- ✅ Good: `improve code organization with focused modules`
- ❌ Bad: `split files to meet lint requirement`

**Describe the benefit:**
- ✅ Good: `reduce memory usage in large file processing`
- ❌ Bad: `fix memory issue found in testing`

**Use active voice:**
- ✅ Good: `add SSO support for enterprise users`
- ❌ Bad: `adding SSO as requested`

## What this command does

1. **Verifies code builds and tests pass** (CRITICAL FIRST STEP)
   - Runs compilation check (`go build ./...` or `make build`)
   - Executes test suite (`go test ./...` or `make test`)
   - Runs linters (`make lint` or `golangci-lint run`)
   - Stops if any verification fails
2. Searches GitHub for related issues and PRs
   - Looks for open/closed issues related to the changes
   - Identifies PRs that may have introduced bugs being fixed
   - Gathers context from related discussions
3. Stages and commits all changes with a proper commit message
   - Verifies all new files are included
   - Checks nothing is accidentally ignored
4. Pushes the branch to origin
5. Creates a PR with properly formatted description following the template:
   - **what**: High-level description of changes
   - **why**: Business justification
   - **references**: Links to discovered issues, related PRs, and documentation
6. Applies appropriate version labels based on semantic versioning
7. Ensures the PR follows Cloud Posse standards from CLAUDE.md

## Version Label Guidelines

**IMPORTANT**: Most PRs should be backwards compatible. Avoid breaking changes.

- **`patch`** (default): Bug fixes, refactoring, performance improvements
- **`minor`**: New features that don't break existing functionality
- **`major`**: Breaking changes (requires strategic planning - AVOID)
- **`no-release`**: Documentation-only changes

When in doubt, use `patch`. The goal is to maintain backwards compatibility.

## Issue Linking Best Practices

The command will automatically search for and link:
- **Related issues**: Issues that mention similar keywords, file names, or error messages
- **Regression sources**: PRs that introduced bugs being fixed (found via git blame)
- **Partial work**: Issues that this PR partially addresses as part of larger work
- **Discussions**: Related PRs and issues with important context

Use these keywords in references:
- `closes #123` - Issue will be closed when PR merges
- `fixes #456` - Alternative to closes
- `relates to #789` - Related but not closed
- `fixes regression from #321` - Links to PR that introduced bug
- `partially addresses #654` - Contributes to larger issue

## See also

- `.claude/agents/pr-standards.md` - Agent that enforces PR standards and version labeling