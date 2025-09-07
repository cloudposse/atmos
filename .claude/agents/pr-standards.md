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

4. **Template Compliance**
   - Uses the PR template from `.github/PULL_REQUEST_TEMPLATE.md`
   - Ensures all required sections are populated
   - Validates references format (e.g., `closes #123`)

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

## Best Practices

1. **Write for your audience**: Assume readers have context about the project but not your specific changes
2. **Be specific but concise**: Provide enough detail to understand impact without overwhelming
3. **Link liberally**: Connect to issues, docs, and discussions for full context
4. **Use conventional commits**: Ensure commit messages align with PR title
5. **Update as needed**: If PR scope changes during review, update the description

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