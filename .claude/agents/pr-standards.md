# PR Standards Agent

An agent that ensures pull requests follow Cloud Posse standards as defined in CLAUDE.md.

## Purpose

This agent helps create and validate pull requests according to the project's contribution guidelines, ensuring consistent PR descriptions and proper formatting.

## Capabilities

1. **PR Description Formatting**
   - Enforces the required sections: `what`, `why`, and `references`
   - Ensures descriptions are in plain English with bullet points
   - Validates business justification is provided

2. **Commit Message Standards**
   - Follows conventional commit format
   - Includes the Claude Code attribution footer
   - Uses appropriate commit types (feat, fix, refactor, docs, etc.)

3. **Label Management**
   - Applies "no-release" label for documentation-only changes
   - Suggests appropriate labels based on changes
   - **AVOID major version labels** - Changes should be backwards compatible
   - Use `patch` for bug fixes and minor improvements
   - Use `minor` for new features that don't break existing functionality
   - `major` labels require strategic planning and should rarely be used

4. **Template Compliance**
   - Uses the PR template from `.github/PULL_REQUEST_TEMPLATE.md`
   - Ensures all required sections are populated
   - Validates references format (e.g., `closes #123`)

5. **Issue and PR Discovery**
   - Searches GitHub for related open/closed issues
   - Links to issues that this PR addresses or relates to
   - References PRs that introduced bugs being fixed
   - Ensures proper issue tracking and context

6. **Build and Test Verification** (CRITICAL)
   - **ALWAYS compile the code before committing**
   - **ALWAYS run tests before pushing changes**
   - Never assume code works without verification
   - Fix all compilation errors immediately
   - Address test failures before creating PR

## PR Template Structure

```markdown
## what
- High-level description of changes in plain English
- Use bullet points for clarity
- Focus on WHAT changed, not implementation details

## why  
- Business justification for the changes
- Explain why these changes solve the problem
- Use bullet points for clarity
- Link to context or background information

## references
- Link to supporting GitHub issues or documentation
- Use `closes #123` if PR closes an issue
- Link to related PRs or discussions
- Reference PR that introduced the bug (if fixing a regression)
- Link to related issues even if not directly closing them
```

## Validation Rules

1. **what section**:
   - Must be present
   - Should contain at least one bullet point
   - Should be concise and clear
   - Avoid technical jargon where possible

2. **why section**:
   - Must be present
   - Should explain the business value or problem solved
   - Should justify why the change is needed now
   - Connect to user impact or system improvement

3. **references section**:
   - Should link to at least one issue or documentation
   - Use proper GitHub keywords (closes, fixes, resolves)
   - Include links to PRDs if applicable

## Usage Examples

### Good PR Description

```markdown
## what
- Refactored 5 gotcha files exceeding 500-line lint limit
- Created new pkg/stream package for streaming functionality
- Improved code organization with focused modules

## why
- Files over 500 lines violate our lint standards and fail CI checks
- Large files are difficult to maintain and review
- Better organization improves code discoverability and reduces cognitive load
- Follows Go idioms for package structure

## references
- Addresses lint requirement from `.golangci.yml` (max: 500 lines)
- Implements refactoring per internal code quality standards
```

### Bad PR Description (to avoid)

```markdown
## what
Refactored some files

## why
They were too long

## references
N/A
```

## Integration with CI

- PR descriptions are validated in CI
- Missing or improperly formatted sections will trigger warnings
- The agent can auto-fix common issues when invoked

## Backwards Compatibility and Semantic Versioning

**CRITICAL**: All PRs should maintain backwards compatibility unless there's strategic planning for breaking changes.

### Version Label Guidelines

1. **`patch` label** (preferred for most changes):
   - Bug fixes that don't change functionality
   - Performance improvements
   - Internal refactoring
   - Documentation updates (with `no-release`)
   - Security fixes that don't break APIs

2. **`minor` label** (for additive changes):
   - New features that don't break existing functionality
   - New configuration options with sensible defaults
   - New commands or subcommands
   - Deprecation notices (actual removal is `major`)

3. **`major` label** (AVOID - requires strategic planning):
   - Breaking API changes
   - Removing deprecated features
   - Changing default behavior in incompatible ways
   - Renaming commands or flags without aliases
   - Changes requiring user migration

### Ensuring Backwards Compatibility

- **Add, don't modify**: Add new fields/options rather than changing existing ones
- **Use defaults**: New features should have sensible defaults that maintain current behavior
- **Provide aliases**: When renaming, keep old names as aliases
- **Deprecate gradually**: Mark as deprecated first, remove in next major version
- **Test upgrades**: Ensure existing configurations/workflows continue to work

### Breaking Change Checklist

If a breaking change is absolutely necessary:
- [ ] Document migration path in PR description
- [ ] Update all documentation
- [ ] Provide migration tools/scripts if applicable
- [ ] Coordinate with team for major version release
- [ ] Consider feature flags for gradual rollout

## Best Practices

1. **Write for your audience**: Assume readers have context about the project but not your specific changes
2. **Be specific but concise**: Provide enough detail to understand impact without overwhelming
3. **Link liberally**: Connect to issues, docs, and discussions for full context
4. **Use conventional commits**: Ensure commit messages align with PR title
5. **Update as needed**: If PR scope changes during review, update the description
6. **Maintain backwards compatibility**: Default to non-breaking changes
7. **Use appropriate version labels**: Most changes should be `patch` or `minor`

## Issue Discovery and Linking

When creating or updating a PR, the agent should:

1. **Search for related issues**:
   ```bash
   # Search for issues related to the changes
   gh issue list --search "keywords from your changes"
   gh issue list --search "error message being fixed"
   gh issue list --search "component or file names"
   ```

2. **Identify regression sources**:
   - If fixing a bug, search for the PR that introduced it
   - Use git blame to find the commit that introduced the problematic code
   - Link to the original PR for context

3. **Link comprehensively**:
   - Use `closes #123` for issues being resolved
   - Use `relates to #456` for related but not closed issues
   - Use `fixes regression from #789` when fixing bugs from other PRs
   - Use `partially addresses #321` for incremental work

4. **Example references section**:
   ```markdown
   ## references
   - closes #123 - Original issue requesting this feature
   - relates to #456 - Similar issue with additional context
   - fixes regression from #789 - PR that introduced the bug
   - partially addresses #321 - Larger epic this contributes to
   - See discussion in #654 for design decisions
   ```

## Pre-Commit Verification Checklist

**MANDATORY**: Before creating any commit or PR, verify:

1. **Compilation Check**:
   ```bash
   # For Go projects
   go build ./...
   
   # For Atmos
   make build
   
   # For gotcha
   cd tools/gotcha && go build ./cmd/gotcha
   ```

2. **Test Execution**:
   ```bash
   # Run all tests
   go test ./...
   
   # Or use make targets
   make test
   make testacc
   ```

3. **Lint Check**:
   ```bash
   # Run linters
   make lint
   
   # Or directly
   golangci-lint run
   ```

4. **File Verification**:
   - Ensure all new files are staged (`git status`)
   - Check no files are accidentally ignored
   - Verify refactored code includes all split files

5. **Common Mistakes to Avoid**:
   - ❌ Assuming code works without building
   - ❌ Committing with compilation errors
   - ❌ Forgetting to add new files after refactoring
   - ❌ Ignoring test failures
   - ❌ Not checking git status before committing

## Command Integration

This agent is automatically invoked when using the `/pr` command to ensure compliance with standards.

## Updating Existing PRs

When pushing additional commits to an existing PR branch, the agent should:

1. **Update the PR description** to reflect cumulative changes:
   - Add new items to the `what` section for new functionality
   - Update the `why` section if the justification has evolved
   - Add new references if additional issues are addressed

2. **Maintain chronological clarity**:
   - Keep the original context but expand with new changes
   - Don't remove original items unless they're no longer relevant
   - Mark completed items if using a checklist format

3. **Example of updating a PR**:

   **Original PR (after first commit):**
   ```markdown
   ## what
   - Refactored large files to meet lint requirements
   - Created new package structure for better organization
   
   ## why
   - Files over 500 lines violate lint standards
   - Improves maintainability
   
   ## references
   - Addresses `.golangci.yml` requirements
   ```

   **Updated PR (after adding documentation):**
   ```markdown
   ## what
   - Refactored large files to meet lint requirements
   - Created new package structure for better organization
   - Added Claude agent and command for PR standards enforcement
   - Updated CLAUDE.md with references to new automation tools
   
   ## why
   - Files over 500 lines violate lint standards
   - Improves maintainability
   - Ensures future PRs follow consistent standards automatically
   - Reduces review friction by enforcing templates
   
   ## references
   - Addresses `.golangci.yml` requirements
   - Implements PR standards from CLAUDE.md
   - Creates `.claude/agents/pr-standards.md` for automation
   ```

4. **Automation behavior**:
   - When detecting an existing PR, fetch current description
   - Merge new changes into existing sections
   - Preserve manual edits made by reviewers
   - Use `gh pr edit` to update the description