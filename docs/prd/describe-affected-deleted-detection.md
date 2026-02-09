# PRD: Detect Deleted Components and Stacks in `describe affected`

**Date**: 2026-02-08

## Status

Implemented.

## Problem Statement

### Current Behavior

The `atmos describe affected` command detects components and stacks that have been **modified** between two Git
commits (HEAD vs. BASE). However, it does **not** detect components or stacks that have been **deleted** in HEAD compared
to BASE.

Currently, `describe affected` iterates over the stacks in HEAD (current branch) and compares them to BASE (target
branch). This means:

1. **Components/stacks that exist only in BASE are invisible** - If a component was removed in HEAD, it won't appear in
   the affected output
2. **No way to trigger destroy workflows** - CI/CD pipelines have no automated way to know which resources need
   `terraform destroy`

### Impact

1. **Manual destruction required**: Users must manually identify and destroy removed components
2. **Resource leaks**: Deleted stack configurations may leave orphaned cloud resources
3. **Incomplete CI/CD**: Pipelines can't fully automate infrastructure lifecycle
4. **Audit gaps**: No automated tracking of what was removed

## Proposed Solution

### Overview

Extend `atmos describe affected` to detect and report:

1. **Deleted components** - Components that exist in BASE but not in HEAD
2. **Deleted stacks** - Entire stacks that exist in BASE but not in HEAD
3. **Removed component instances** - Component removed from a specific stack (but may exist in other stacks)

### New Affected Reasons

Add new affected reason values:

| Reason              | Description                                               |
|---------------------|-----------------------------------------------------------|
| `deleted`           | Component/stack was deleted (exists in BASE, not in HEAD) |
| `deleted.component` | The component definition was removed from the stack       |
| `deleted.stack`     | The entire stack file/configuration was removed           |

### New Output Fields

Add new fields to the affected output schema:

```json
{
  "component": "vpc",
  "component_type": "terraform",
  "stack": "dev-us-east-1",
  "affected": "deleted",
  "deleted": true,
  "deletion_type": "component"
}
```

| Field           | Type    | Description                                                     |
|-----------------|---------|-----------------------------------------------------------------|
| `deleted`       | boolean | `true` if this component/stack was deleted                      |
| `deletion_type` | string  | Type of deletion: `component`, `stack`, or `component_instance` |

### Algorithm Changes

**Current algorithm:**

```text
for each stack in HEAD:
    for each component in stack:
        compare with BASE
        if different: add to affected
```

**New algorithm:**

```text
for each stack in HEAD:
    for each component in stack:
        compare with BASE
        if different: add to affected

# NEW: Also check BASE for deletions
for each stack in BASE:
    if stack not in HEAD:
        add all components as deleted (deletion_type: stack)
    else:
        for each component in BASE stack:
            if component not in HEAD stack:
                add as deleted (deletion_type: component)
```

Users can filter the output using `--query` to separate modified vs deleted components.

## Use Cases

### Use Case 1: Component Removed from Stack

**Scenario**: User removes the `monitoring` component from `prod-us-east-1` stack.

**BASE (main branch):**

```yaml
# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"
    monitoring:
      vars:
        enabled: true
```

**HEAD (PR branch):**

```yaml
# stacks/prod/us-east-1.yaml
components:
  terraform:
    vpc:
      vars:
        cidr: "10.0.0.0/16"
    # monitoring component removed
```

**Output:**

```json
[
  {
    "component": "monitoring",
    "component_type": "terraform",
    "stack": "prod-us-east-1",
    "stack_slug": "prod-us-east-1-monitoring",
    "affected": "deleted",
    "deleted": true,
    "deletion_type": "component"
  }
]
```

### Use Case 2: Entire Stack Removed

**Scenario**: User deletes the entire `staging-us-west-2` stack.

**BASE:** Stack file exists at `stacks/staging/us-west-2.yaml`
**HEAD:** Stack file deleted

**Output:**

```json
[
  {
    "component": "vpc",
    "component_type": "terraform",
    "stack": "staging-us-west-2",
    "affected": "deleted.stack",
    "deleted": true,
    "deletion_type": "stack"
  },
  {
    "component": "eks",
    "component_type": "terraform",
    "stack": "staging-us-west-2",
    "affected": "deleted.stack",
    "deleted": true,
    "deletion_type": "stack"
  }
]
```

### Use Case 3: CI/CD Pipeline Integration

**GitHub Actions workflow:**

```yaml
jobs:
  detect-changes:
    runs-on: ubuntu-latest
    outputs:
      modified: ${{ steps.affected.outputs.modified }}
      deleted: ${{ steps.affected.outputs.deleted }}
    steps:
      - uses: actions/checkout@v4
        with:
          fetch-depth: 0

      - name: Detect affected
        id: affected
        run: |
          # Get all affected components
          atmos describe affected --format json > affected.json

          # Filter modified components (for apply)
          jq '[.[] | select(.deleted != true)]' affected.json > modified.json
          echo "modified=$(cat modified.json | jq -c)" >> $GITHUB_OUTPUT

          # Filter deleted components (for destroy)
          jq '[.[] | select(.deleted == true)]' affected.json > deleted.json
          echo "deleted=$(cat deleted.json | jq -c)" >> $GITHUB_OUTPUT

  apply:
    needs: detect-changes
    if: needs.detect-changes.outputs.modified != '[]'
    strategy:
      matrix:
        include: ${{ fromJson(needs.detect-changes.outputs.modified) }}
    steps:
      - run: atmos terraform apply ${{ matrix.component }} -s ${{ matrix.stack }}

  destroy:
    needs: detect-changes
    if: needs.detect-changes.outputs.deleted != '[]'
    strategy:
      matrix:
        include: ${{ fromJson(needs.detect-changes.outputs.deleted) }}
    steps:
      - run: atmos terraform destroy ${{ matrix.component }} -s ${{ matrix.stack }} --auto-approve
```

### Use Case 4: Query for Deleted Only

Users can use the `--query` flag to filter:

```shell
# Get only deleted components
atmos describe affected --query '[.[] | select(.deleted == true)]'

# Get only modified (not deleted) components
atmos describe affected --query '[.[] | select(.deleted != true)]'

# Get deleted components in specific stack
atmos describe affected --query '[.[] | select(.deleted == true and .stack == "prod-us-east-1")]'
```

## Implementation

### Phase 1: Core Detection

1. **Add deletion detection logic** in `internal/exec/describe_affected_utils.go`:
   ```go
   func detectDeletedComponents(
       baseStacks map[string]any,
       headStacks map[string]any,
       atmosConfig *schema.AtmosConfiguration,
   ) ([]schema.Affected, error) {
       var deleted []schema.Affected

       for stackName, baseStack := range baseStacks {
           headStack, existsInHead := headStacks[stackName]

           if !existsInHead {
               // Entire stack deleted
               // Add all components with deletion_type: "stack"
           } else {
               // Check for deleted components within stack
               // Add missing components with deletion_type: "component"
           }
       }

       return deleted, nil
   }
   ```

2. **Update `schema.Affected`** in `pkg/schema/schema.go`:
   ```go
   type Affected struct {
       // ... existing fields ...
       Deleted      bool   `json:"deleted,omitempty" yaml:"deleted,omitempty"`
       DeletionType string `json:"deletion_type,omitempty" yaml:"deletion_type,omitempty"`
   }
   ```

### Phase 2: Documentation

1. Update `website/docs/cli/commands/describe/describe-affected.mdx`:

- Add `deleted` and `deletion_type` to output schema
- Document `deleted` affected reason
- Add examples for deletion detection
- Add CI/CD workflow examples for destroy pipelines

2. Update GitHub Actions documentation for destroy workflows

### Phase 3: Testing

1. Add test fixtures for:

- Component deleted from stack
- Entire stack deleted
- Multiple deletions
- Mixed modifications and deletions

2. Add unit tests:

- `TestDetectDeletedComponents`
- `TestDetectDeletedStacks`
- `TestDescribeAffectedWithDeleted`

## Edge Cases

### 1. Abstract Components

Abstract components (`metadata.type: abstract`) should not be reported as deleted since they are not provisioned.

### 2. Disabled Components

Components with `metadata.enabled: false` in BASE should still be reported if removed from HEAD (user may want to clean
up the disabled resource).

### 3. Component Renamed

If a component is renamed (old name removed, new name added):

- Old name appears as `deleted`
- New name appears as modified (new component)

Users should handle this case in their pipelines (may need manual intervention to avoid destroying and recreating).

### 4. Stack File Moved

If a stack file is moved (but results in the same logical stack):

- Need to compare by stack name, not file path
- May require additional logic to detect moves vs deletes

### 5. Locked Components

Components with `metadata.locked: true` in BASE that are deleted in HEAD should:

- Still be reported as deleted (with a warning?)
- Or be excluded if `--exclude-locked` is passed?

**Recommendation**: Report them as deleted but add a `was_locked: true` field to alert the user.

## Security Considerations

1. **Destroy requires explicit action**: Atmos only reports deletions; destruction requires separate pipeline step with
   explicit `terraform destroy` command
2. **Clear identification**: Deleted components are clearly marked with `deleted: true` field
3. **Audit trail**: Deleted components are logged with full context for audit

## Success Criteria

1. Deleted components are automatically detected and included in output
2. Deleted components have clear `affected: deleted` reason and `deleted: true` field
3. CI/CD pipelines can separate apply and destroy workflows using `--query` or jq filtering
4. Existing workflows continue to work (new fields are additive, not breaking)
5. Documentation covers destruction workflow patterns
6. All edge cases are handled or documented

## Future Extensions

### 1. Destroy Plan Generation

```shell
atmos describe affected --include-deleted --generate-destroy-plans
```

Generate `terraform plan -destroy` for each deleted component.

### 2. Dependency-Aware Destruction

When deleting components with dependents:

- Warn about dependent components
- Suggest destruction order (dependents first)

### 3. Soft Delete Detection

Detect components marked as `metadata.enabled: false` (soft delete) vs completely removed (hard delete).

### 4. State Orphan Detection

Compare stack configuration with actual Terraform state to detect:

- Resources in state but not in config (orphans)
- Config without state (never applied)

## References

- [GitHub Actions: Affected Stacks](https://atmos.tools/integrations/github-actions/affected-stacks)
- [Terraform Destroy](https://developer.hashicorp.com/terraform/cli/commands/destroy)
