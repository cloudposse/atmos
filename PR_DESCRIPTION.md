# Fix: Merge commands from .atmos.d and imports instead of replacing them

## what
- Fixed regression where commands from `.atmos.d/` directories and explicit imports were being replaced instead of merged with local commands
- Implemented proper command merging behavior that combines commands from all sources (defaults, .atmos.d, imports, local) with correct precedence
- Added comprehensive test coverage validating all command merging scenarios including CloudPosse's real-world use case
- Created Product Requirements Document capturing implementation details and requirements

## why
- Organizations using Atmos need to maintain centralized command definitions that projects can import, extend, and optionally override
- Previous behavior broke workflows where teams define common commands in central repositories (e.g., CloudPosse's `.github` repo) that projects import and customize
- The regression prevented command inheritance, forcing teams to either duplicate all commands locally or lose access to centralized commands
- This fix enables:
  - Command inheritance from organizational repositories
  - Local project customization and overrides
  - Multi-level organizational structures with department/team/project command hierarchies
  - Modular command libraries using glob patterns

## Technical Details

### Root Cause
Viper's `MergeConfig` function doesn't overwrite arrays - it preserves existing array values. This caused imported commands to be ignored when local commands were present.

### Solution
- Modified `pkg/config/load.go` to use temporary Viper instances to extract commands from imported files
- Restructured `processConfigImportsAndReapply` to apply correct precedence order: defaults < .atmos.d < imports < local
- Updated `mergeCommandArrays` to support name-based override behavior where later commands replace earlier ones with the same name

### Command Precedence Order
1. Embedded defaults (lowest precedence)
2. `.atmos.d/` directories
3. Explicit imports (via `import:` field)
4. Local configuration (highest precedence - wins on duplicates)

### Test Coverage
- Basic merging: imported + local = all commands
- Override behavior: local overrides imported with same name
- Deep nesting: 4+ level import chains
- Empty imports: no effect on other commands
- Complex structures: command properties preserved
- Real-world scenario: 10 upstream + 1 local = 11 total commands (CloudPosse use case)

## references
- Related to #1447 and #1489 which attempted to address this issue
- Fixes CloudPosse's workflow for centralized command management
- PRD: `docs/prd/command-merging.md`
