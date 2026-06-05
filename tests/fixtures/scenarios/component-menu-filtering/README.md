# Component Menu Filtering Test Fixture

This fixture tests the component menu filtering functionality in Atmos, specifically:

1. **Abstract component filtering**: Components with `metadata.type: abstract` should not appear in interactive component selection menus
2. **Disabled component filtering**: Components with `metadata.enabled: false` should not appear in interactive component selection menus
3. **Stack-scoped filtering**: When `--stack` flag is provided, only components deployed in that specific stack should appear

## Background

When users run Atmos terraform commands interactively (without specifying a component), they are presented with a menu to select a component. This menu should:

- Filter out abstract components (base configurations meant for inheritance only)
- Filter out disabled components (components explicitly marked as not deployable)
- When a stack is specified via `--stack`, show only components that exist in that stack

## Fixture Structure

```
component-menu-filtering/
├── README.md
├── atmos.yaml
├── components/
│   └── terraform/
│       ├── vpc/main.tf      # VPC component
│       ├── eks/main.tf      # EKS component
│       └── rds/main.tf      # RDS component
└── stacks/
    ├── catalog/
    │   ├── vpc.yaml         # Abstract base VPC (type: abstract)
    │   ├── eks.yaml         # EKS base configuration
    │   └── rds.yaml         # RDS base configuration
    └── deploy/
        ├── dev.yaml         # Has: vpc, eks (disabled)
        ├── staging.yaml     # Has: vpc, rds
        └── prod.yaml        # Has: vpc, eks, rds
```

## Component Configuration Summary

| Component | dev stack | staging stack | prod stack | Notes |
|-----------|-----------|---------------|------------|-------|
| vpc-base  | N/A       | N/A           | N/A        | Abstract (catalog only) |
| vpc       | Yes       | Yes           | Yes        | Deployable |
| eks       | Disabled  | No            | Yes        | Disabled in dev |
| rds       | No        | Yes           | Yes        | Not in dev |

## Manual Testing Commands

### Test 1: Verify data setup with describe stacks

```bash
# Change to fixture directory
cd tests/fixtures/scenarios/component-menu-filtering

# List all stacks
atmos describe stacks --format json | jq 'keys'
# Expected: ["dev", "prod", "staging"]

# Show components with their metadata per stack
atmos describe stacks --format json | jq 'to_entries[] | {stack: .key, components: [.value.components.terraform | to_entries[] | {name: .key, type: .value.metadata.type, enabled: .value.metadata.enabled}]}'
# Expected:
# dev: vpc-base (abstract), vpc (enabled), eks (enabled: false)
# staging: vpc-base (abstract), vpc, rds
# prod: vpc-base (abstract), vpc, eks, rds

# Verify vpc-base is abstract
atmos describe component vpc-base --stack dev | grep -A3 "^metadata:"
# Expected: type: abstract

# Verify eks is disabled in dev
atmos describe component eks --stack dev | grep -A3 "^metadata:"
# Expected: enabled: false
```

### Test 2: Verify shell completion filters correctly (Non-TTY testable)

Shell completion uses the same filtering logic as the interactive selector. These tests verify the filtering works correctly:

```bash
cd tests/fixtures/scenarios/component-menu-filtering

# Test all components completion (no --stack)
atmos __complete terraform plan "" 2>/dev/null | grep -v "^:"
# Expected: eks, rds, vpc
# NOTE: vpc-base (abstract) should NOT appear

# Test dev stack - should show only vpc (eks is disabled, rds doesn't exist)
atmos __complete terraform plan --stack dev "" 2>/dev/null | grep -v "^:"
# Expected: vpc

# Test staging stack - should show vpc and rds (no eks in staging)
atmos __complete terraform plan --stack staging "" 2>/dev/null | grep -v "^:"
# Expected: rds, vpc

# Test prod stack - should show all three deployable components
atmos __complete terraform plan --stack prod "" 2>/dev/null | grep -v "^:"
# Expected: eks, rds, vpc
```

### Test 3: Verify interactive selector (TTY only)

The interactive selector requires a TTY environment. Test these manually in an interactive terminal:

```bash
cd tests/fixtures/scenarios/component-menu-filtering

# Without component - should show interactive selector
# In TTY: Shows selector with vpc, eks, rds (not vpc-base)
# In non-TTY: Returns error "stack is required"
atmos terraform plan

# With --stack but no component - runs multi-component execution
# (Runs all deployable components in the stack)
atmos terraform plan --stack dev
# In TTY or non-TTY: Runs vpc component (only deployable component in dev)

# Verify a specific component works
atmos terraform plan vpc --stack dev
# Should run terraform plan for vpc in dev stack
```

### Test 4: Verify tab completion in interactive shell

In a bash/zsh shell with Atmos completions installed:

```bash
cd tests/fixtures/scenarios/component-menu-filtering

# Type and press TAB twice
atmos terraform plan <TAB><TAB>
# Expected: eks, rds, vpc (no vpc-base)

atmos terraform plan --stack dev <TAB><TAB>
# Expected: vpc only

atmos terraform plan --stack staging <TAB><TAB>
# Expected: rds, vpc

atmos terraform plan --stack prod <TAB><TAB>
# Expected: eks, rds, vpc
```

## Expected Behavior Summary

1. **Abstract components (`metadata.type: abstract`)**: Never appear in menus or completions
2. **Disabled components (`metadata.enabled: false`)**: Never appear in menus or completions
3. **Stack filtering**: When `--stack` is provided, only components in that stack appear
4. **All stacks mode**: Without `--stack`, shows union of all deployable components across all stacks

## Related Code

- `cmd/terraform/shared/prompt.go`: Contains the component filtering logic
- `isComponentDeployable()`: Checks if a component is abstract or disabled
- `filterDeployableComponents()`: Filters out non-deployable components
- `listTerraformComponentsForStack()`: Lists components for a specific stack
- `listTerraformComponents()`: Lists all deployable components across all stacks
