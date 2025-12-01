# Product Requirements Document: Atmos Command Merging

## Overview

This PRD documents the requirements and implementation for Atmos command merging functionality, which enables teams to maintain centralized command definitions that projects can import, extend, and optionally override.

## Problem Statement

Organizations using Atmos need to:
1. Define common commands in centralized configuration repositories
2. Import these commands into multiple projects
3. Extend imported commands with project-specific additions
4. Override imported commands when local customization is needed
5. Support complex import chains for multi-level organizational structures

Previously, imported commands were either completely replaced or lost during the configuration loading process, breaking workflows that depended on command inheritance.

## Goals

- **Enable command inheritance**: Projects should inherit all commands from imported configurations
- **Support local overrides**: Local configurations should be able to override imported commands
- **Maintain simplicity**: The merging behavior should be intuitive and predictable
- **Preserve backward compatibility**: Existing configurations should continue to work

## Non-Goals

- Deep merging of individual command properties (only name-based replacement)
- Conditional command inclusion based on environment or context
- Command versioning or deprecation mechanisms

## Requirements

### Functional Requirements

#### FR1: Command Collection from All Sources
The system MUST collect commands from all configuration sources in the following order:
1. Embedded defaults (built into Atmos)
2. `.atmos.d/` directories (both `atmos.d/` and `.atmos.d/`)
3. Explicitly imported configurations (via `import:` field)
4. Local configuration file

#### FR2: Command Merging Behavior
- **FR2.1**: Commands from all sources MUST be combined into a single list
- **FR2.2**: When command names duplicate, the last occurrence MUST take precedence
- **FR2.3**: The precedence order MUST be: defaults < `.atmos.d/` < imports < local

#### FR3: Import Processing
- **FR3.1**: The system MUST support file imports via the `import:` field
- **FR3.2**: The system MUST support glob patterns in import paths (e.g., `commands/*.yaml`)
- **FR3.3**: The system MUST support nested imports (imports within imported files)
- **FR3.4**: The system MUST support at least 4 levels of import nesting

#### FR4: Local Override Capability
- **FR4.1**: Local commands with the same name as imported commands MUST override them
- **FR4.2**: The override MUST be complete (entire command replaced, not merged)
- **FR4.3**: Override behavior MUST be consistent across all import methods

#### FR5: Command Preservation
- **FR5.1**: All unique commands from imports MUST be preserved
- **FR5.2**: Command order MUST be deterministic and reproducible
- **FR5.3**: Complex command structures (steps, env, arguments, flags) MUST be preserved

### Technical Requirements

#### TR1: Configuration Processing
- **TR1.1**: The system MUST handle Viper's non-overwriting behavior for arrays
- **TR1.2**: The system MUST use temporary Viper instances to extract commands from imports
- **TR1.3**: The system MUST maintain the same code path for all import types

#### TR2: Performance
- **TR2.1**: Import processing MUST complete within reasonable time (<1 second for typical configs)
- **TR2.2**: The system MUST handle at least 100 imported commands efficiently
- **TR2.3**: Memory usage MUST scale linearly with number of commands

#### TR3: Error Handling
- **TR3.1**: The system MUST continue processing if individual imports fail
- **TR3.2**: Failed imports MUST be logged but not halt configuration loading
- **TR3.3**: Circular imports MUST be detected and prevented

## Use Cases

### UC1: Centralized Organization Commands
**Actor**: DevOps Team Lead
**Scenario**: CloudPosse maintains a central repository with standard commands
1. Central repo defines 10 common commands (lint, test, deploy, etc.)
2. Project imports the central configuration
3. Project adds 1 project-specific command
4. Result: Project has access to all 11 commands

### UC2: Command Customization
**Actor**: Developer
**Scenario**: Project needs to customize an organization command
1. Organization defines a `deploy` command with standard steps
2. Project imports the organization config
3. Project defines its own `deploy` command with custom steps
4. Result: Project's `deploy` command completely replaces the imported one

### UC3: Multi-Level Organization
**Actor**: Enterprise Architect
**Scenario**: Large organization with department and team-level configs
1. Company-wide config defines baseline commands
2. Department config imports company config and adds department commands
3. Team config imports department config and adds team commands
4. Project imports team config and adds project commands
5. Result: Project has access to all commands from all levels

### UC4: Modular Command Libraries
**Actor**: Platform Engineer
**Scenario**: Commands organized by category in separate files
1. `commands/terraform.yaml` defines Terraform-related commands
2. `commands/kubernetes.yaml` defines Kubernetes-related commands
3. `commands/testing.yaml` defines testing commands
4. Project imports `commands/*.yaml` using glob pattern
5. Result: All commands from all files are available

## Implementation Details

### Command Merging Algorithm

```go
function mergeCommands(sources []CommandSource) []Command {
    commandMap := map[string]Command{}
    orderedNames := []string{}

    for each source in sources (in precedence order) {
        for each command in source {
            if command.name not in commandMap {
                orderedNames.append(command.name)
            }
            commandMap[command.name] = command  // Override if exists
        }
    }

    result := []Command{}
    for each name in orderedNames {
        result.append(commandMap[name])
    }
    return result
}
```

### Precedence Chain

```text
Embedded Defaults
    ↓ (lowest precedence)
.atmos.d/ directories
    ↓
Explicit imports (in order listed)
    ↓
Local configuration
    ↓ (highest precedence)
Final merged commands
```

### Configuration Example

```yaml
# Organization config (https://github.com/cloudposse/.github/atmos/commands.yaml)
commands:
  - name: lint
    description: "Organization standard linting"
    steps:
      - golangci-lint run

  - name: test
    description: "Organization standard testing"
    steps:
      - go test ./...

# Project config (atmos.yaml)
import:
  - https://github.com/cloudposse/.github/atmos/commands.yaml

commands:
  - name: lint
    description: "Project-specific linting with custom rules"
    steps:
      - golangci-lint run --config=.golangci.local.yml

  - name: build
    description: "Project build command"
    steps:
      - go build ./...

# Result: 3 commands available
# - lint (project version - overrides organization)
# - test (from organization)
# - build (project-specific)
```

## Testing Requirements

### Test Scenarios

1. **Basic Merging**: Verify imported + local commands = all commands
2. **Override Behavior**: Verify local overrides imported with same name
3. **Deep Nesting**: Verify 4+ level import chains work
4. **Empty Imports**: Verify empty imports don't affect other commands
5. **Glob Patterns**: Verify glob imports include all matched files
6. **Complex Structures**: Verify command properties are preserved
7. **Duplicate Names**: Verify deduplication works correctly
8. **Order Preservation**: Verify commands appear in consistent order

### Acceptance Criteria

- [ ] All imported commands are accessible via `atmos <command> --help`
- [ ] Local commands with duplicate names completely replace imported ones
- [ ] Import chains of 4+ levels work without errors
- [ ] 10 imported + 1 local = 11 total commands
- [ ] No regression in existing functionality
- [ ] Performance remains acceptable (<1 second load time)

## Rollout Plan

1. **Phase 1**: Implement core merging logic with comprehensive tests
2. **Phase 2**: Validate with CloudPosse's actual configurations
3. **Phase 3**: Document behavior in Atmos documentation
4. **Phase 4**: Release as patch version (backward compatible)

## Success Metrics

- Zero regression reports related to command availability
- Successful adoption by CloudPosse and similar organizations
- Reduced configuration duplication across projects
- Improved command standardization across teams

## Open Questions

None - all requirements have been implemented and tested.

## Decision Log

| Date | Decision | Rationale |
|------|----------|-----------|
| 2025-09-26 | Use name-based override instead of deep merge | Simpler mental model, predictable behavior |
| 2025-09-26 | Process `.atmos.d/` before explicit imports | Follows precedence principle: more specific overrides less specific |
| 2025-09-26 | Support glob patterns in imports | Enables modular command organization |
| 2025-09-26 | Same code path for all import types | Ensures consistent behavior, easier maintenance |

## References

- [Original Issue](https://github.com/cloudposse/atmos/issues/1447)
- [First Fix Attempt](https://github.com/cloudposse/atmos/issues/1489)
- [Atmos Configuration Documentation](https://atmos.tools/configuration)
- [CloudPosse Reference Architecture](https://github.com/cloudposse/reference-architectures)
