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

## Examples

```
/pr --base testacc-job-summary
/pr --title "feat: add new feature" --base main
/pr --no-release
```

## What this command does

1. Stages and commits all changes with a proper commit message
2. Pushes the branch to origin
3. Creates a PR with properly formatted description following the template:
   - **what**: High-level description of changes
   - **why**: Business justification
   - **references**: Links to issues or documentation
4. Ensures the PR follows Cloud Posse standards from CLAUDE.md

## See also

- `.claude/agents/pr-standards.md` - Agent that enforces PR standards