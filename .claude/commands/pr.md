# PR Command

Create a pull request following Cloud Posse standards.

## Usage

```
/pr [options]
```

## Options

- `--base <branch>`: Target branch for the PR (default: main)
- `--title <title>`: PR title (will be auto-generated if not provided)
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

## What this command does

1. Stages and commits all changes with a proper commit message
2. Pushes the branch to origin
3. Creates a PR with properly formatted description following the template:
   - **what**: High-level description of changes
   - **why**: Business justification
   - **references**: Links to issues or documentation
4. Applies appropriate version labels based on semantic versioning
5. Ensures the PR follows Cloud Posse standards from CLAUDE.md

## Version Label Guidelines

**IMPORTANT**: Most PRs should be backwards compatible. Avoid breaking changes.

- **`patch`** (default): Bug fixes, refactoring, performance improvements
- **`minor`**: New features that don't break existing functionality
- **`major`**: Breaking changes (requires strategic planning - AVOID)
- **`no-release`**: Documentation-only changes

When in doubt, use `patch`. The goal is to maintain backwards compatibility.

## See also

- `.claude/agents/pr-standards.md` - Agent that enforces PR standards and version labeling