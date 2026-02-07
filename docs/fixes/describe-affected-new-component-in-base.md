# Issue: describe affected fails when new component exists in BASE but not in HEAD

## Problem Summary

When running `atmos describe affected` in a PR that hasn't been rebased against main, and main (BASE) contains a new
component that doesn't exist in the PR branch (HEAD), the command fails with an error like:

```text
Error: failed to describe component prometheus in stack plat-usw1-staging in
YAML function: !terraform.state prometheus workspace_endpoint invalid component:
Could not find the component prometheus in the stack plat-usw1-staging. Check
that all the context variables are correctly defined in the stack manifests.
Are the component and stack names correct? Did you forget an import?
```

## Scenario

1. **PR1** introduces a new component (e.g., `prometheus`) and merges into main
2. **PR2** was created from main **before** PR1 was merged
3. When PR2 runs `describe affected` against main, it fails because:
  - The new component exists in BASE (main) but not in HEAD (PR2 branch)
  - Stack files in BASE may reference the new component via `!terraform.state`
  - When processing remoteStacks, YAML functions call `ExecuteDescribeComponent` which looks in HEAD (current working
    directory), not in BASE

## Root Cause

The issue is in `internal/exec/describe_affected_utils.go` in the `executeDescribeAffected` function:

1. `ExecuteDescribeStacks` is called for `currentStacks` (HEAD) - works fine
2. `ExecuteDescribeStacks` is called for `remoteStacks` (BASE) with modified paths
3. When processing `remoteStacks`, if a stack file contains `!terraform.state componentName ...`:
  - The YAML function processor calls `GetTerraformState`
  - `GetTerraformState` calls `ExecuteDescribeComponent(component, stack)`
  - `ExecuteDescribeComponent` looks for the component in the **current working directory** (HEAD)
  - If the component only exists in BASE (not in HEAD), the lookup fails

The problem is that YAML function resolution uses the current working directory context, not the modified paths for the
remote repository.

## Current Workaround

Users must rebase their PR branch against main before running `describe affected`:

```bash
git fetch origin main
git rebase origin/main
```

## Proposed Solutions

### Option 1: Improve Error Message (DX Enhancement)

Add context to the error message explaining WHY the component wasn't found:

```
Error: failed to describe component `prometheus` in stack `plat-usw1-staging`
in YAML function: !terraform.state prometheus workspace_endpoint

The component `prometheus` exists in the base branch but not in the head branch.
This typically happens when the base branch (main) has new components that haven't
been rebased into your PR branch yet.

Suggested actions:
  1. Rebase your branch against the base branch: git rebase origin/main
  2. Or use --process-functions=false to skip YAML function evaluation
```

### Option 2: Skip YAML Functions for Non-Existent Components

When processing YAML functions for remoteStacks, if a referenced component doesn't exist in HEAD:

- Log a warning
- Return nil/null for the function result
- Continue processing

This allows `describe affected` to complete and show what's affected, even if some YAML function values can't be
resolved.

### Option 3: Context-Aware Component Resolution

Pass the repository context (local vs remote) through the YAML function resolution chain so that when processing
remoteStacks, component lookups are performed against the remote repo paths, not the current working directory.

This is the most correct solution but requires significant refactoring.

### Option 4: Detect and Report New Components in BASE

Currently, `findAffected` only iterates over `currentStacks` (HEAD). New components that exist only in BASE are never
detected.

Add logic to also iterate over `remoteStacks` and detect components that:

- Exist in BASE but not in HEAD (new components added to main)
- Report these as "new component in base" affected items

## Test Fixtures

Test fixtures have been added to reproduce this issue:

```text
tests/fixtures/scenarios/atmos-describe-affected-new-component-in-base/
├── atmos.yaml
├── stacks/
│   └── deploy/
│       └── staging.yaml              # HEAD state (without prometheus)
├── stacks-with-new-component/
│   └── deploy/
│       └── staging.yaml              # BASE state (with prometheus, no reference)
└── stacks-with-new-component-and-reference/
    └── deploy/
        └── staging.yaml              # BASE state (with prometheus + !terraform.state reference)
```

Tests:

- `TestDescribeAffectedNewComponentInBase` - Tests basic scenario (no YAML functions)
- `TestDescribeAffectedNewComponentInBaseWithYamlFunctions` - Tests the exact error scenario

## Implementation Status

- [x] Issue documented
- [x] Test fixtures created
- [x] Tests added to reproduce the issue
- [ ] Fix implemented
- [ ] Error message improved
