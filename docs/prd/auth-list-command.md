# PRD: `atmos auth list` Command

## Overview

**Feature**: `atmos auth list` command for listing authentication providers and identities
**Status**: Design
**Created**: 2025-01-15
**Author**: Claude Code
**Target Release**: TBD

## Executive Summary

The `atmos auth list` command provides users with a comprehensive view of all configured authentication providers and identities in their Atmos configuration. It supports multiple output formats (table, tree, JSON, YAML) and flexible filtering options to help users understand their authentication infrastructure, visualize complex identity chains, and troubleshoot authentication issues.

## Problem Statement

### Current State

Users currently have no easy way to:
1. View all configured authentication providers and identities at a glance
2. Understand the relationships between providers and identities
3. Visualize complex authentication chains involving multiple role assumptions
4. Inspect authentication configuration without manually parsing YAML files
5. Export authentication configuration for documentation or automation purposes

### Pain Points

1. **Lack of Visibility**: Users must manually read `atmos.yaml` to understand what providers and identities are available
2. **Complex Chains**: Multi-level role assumption chains are difficult to understand from YAML configuration alone
3. **Troubleshooting**: When authentication fails, users cannot easily see the full chain of dependencies
4. **Documentation**: No programmatic way to export authentication configuration for documentation
5. **Discovery**: New team members struggle to understand available authentication options

### User Impact

- DevOps engineers spend time manually tracing authentication chains
- Security auditors need visibility into authentication configurations
- New team members face a steep learning curve
- Troubleshooting authentication issues is time-consuming and error-prone

## Goals and Non-Goals

### Goals

1. **Visibility**: Provide clear visibility into all configured providers and identities
2. **Chain Visualization**: Show complete authentication chains from provider through all role assumptions
3. **Multiple Formats**: Support table, tree, JSON, and YAML output formats
4. **Flexible Filtering**: Allow filtering by provider name(s), identity name(s), or type
5. **User Experience**: Follow Atmos CLI conventions and styling
6. **Test Coverage**: Achieve 85-90% test coverage
7. **Documentation**: Comprehensive user documentation with examples

### Non-Goals

1. **Modification**: This command only lists/views configuration, does not modify it
2. **Credential Display**: Does not show actual credentials or secrets
3. **Interactive Selection**: Not an interactive picker (use `atmos auth login` for that)
4. **Validation**: Does not validate configuration (use `atmos auth validate` for that)
5. **Real-time Status**: Does not show if credentials are currently valid/expired (use `atmos auth whoami`)

## User Stories

### US-1: View All Authentication Configuration
**As a** DevOps engineer
**I want to** view all configured providers and identities
**So that** I can understand what authentication options are available

**Acceptance Criteria**:
- Command runs without arguments and shows all providers and identities
- Output is clear and well-formatted
- Default format (table) works without additional flags
- Both providers and identities are displayed with key metadata

### US-2: Understand Authentication Chains
**As a** security engineer
**I want to** visualize the complete authentication chain for an identity
**So that** I can understand the full path of role assumptions

**Acceptance Criteria**:
- Tree format shows complete chain: provider → identity1 → identity2 → target
- Chain is displayed for each identity
- Chain handles arbitrary depth (no limit on role assumptions)
- Broken chains are handled gracefully with error messages

### US-3: Filter by Provider
**As a** platform engineer
**I want to** view configuration for a specific provider
**So that** I can focus on relevant authentication settings

**Acceptance Criteria**:
- `--providers aws-sso` shows only aws-sso provider
- `--providers aws-sso,okta` shows multiple specific providers
- `--providers` (no value) shows all providers
- Non-existent provider names show helpful error message

### US-4: Filter by Identity
**As a** developer
**I want to** view specific identities and their chains
**So that** I can understand how to authenticate for my use case

**Acceptance Criteria**:
- `--identities admin` shows only admin identity with its chain
- `--identities admin,dev,prod` shows multiple identities
- `--identities` (no value) shows all identities
- Shows complete chain even when filtering

### US-5: Export for Documentation
**As a** technical writer
**I want to** export authentication configuration as JSON/YAML
**So that** I can generate documentation programmatically

**Acceptance Criteria**:
- `--format json` produces valid JSON output
- `--format yaml` produces valid YAML output
- Output matches schema of `atmos.yaml` auth configuration
- Can pipe output to files or other tools

### US-6: Quick Provider Lookup
**As a** system administrator
**I want to** quickly see details of a single provider
**So that** I can verify configuration without reading YAML files

**Acceptance Criteria**:
- `atmos auth list --providers aws-sso --format tree` shows detailed provider info
- Shows all relevant attributes (region, URLs, session config, etc.)
- Output is concise and easy to read

## Functional Requirements

### FR-1: Command Structure

**Requirement**: Command follows standard Cobra CLI patterns

```bash
atmos auth list [flags]
```

**Flags**:
- `--format`, `-f`: Output format (table, tree, json, yaml) - default: table
- `--providers [name]`: Show only providers, optionally filter by name(s)
- `--identities [name]`: Show only identities, optionally filter by name(s)
- `--identity`, `-i`: Filter by identity (inherited from auth parent)
- `--profile`: Filter by profile (inherited from auth parent)

**Validation**:
- `--providers` and `--identities` are mutually exclusive
- Invalid format values show error with valid options
- Non-existent provider/identity names show helpful message

### FR-2: Table Format Output

**Requirement**: Default table format shows providers and identities clearly

**Providers Table Columns**:
- NAME: Provider name
- KIND: Provider type (aws/iam-identity-center, aws/saml, github/oidc, etc.)
- REGION: AWS region (for AWS providers)
- START URL / URL: Authentication endpoint
- DEFAULT: Marker (✓) for default provider

**Identities Table Columns**:
- NAME: Identity name
- KIND: Identity type (aws/permission-set, aws/assume-role, aws/user, etc.)
- VIA PROVIDER: Direct provider reference
- VIA IDENTITY: Parent identity reference (for chained identities)
- DEFAULT: Marker (✓) for default identity
- ALIAS: Alternative name

**Implementation**:
- Use `github.com/charmbracelet/bubbles/table`
- Apply Atmos theme colors from `pkg/ui/theme/colors.go`
- Truncate long URLs with ellipsis
- Handle missing fields with "-"
- Sort alphabetically by name

### FR-3: Tree Format Output

**Requirement**: Tree format shows hierarchical relationships and chains

**Structure**:
```
Authentication Configuration

Providers
├─ provider-name (kind) [DEFAULT]
│  ├─ Attribute: value
│  └─ Attribute: value

Identities
├─ identity-name (kind) [DEFAULT] [ALIAS: alias-name]
│  ├─ Via Provider: provider-name
│  ├─ Chain: provider → identity1 → identity2 → target
│  ├─ Principal:
│  │  ├─ Key: value
│  │  └─ Key: value
│  └─ Credentials:
│     └─ Key: value
```

**Implementation**:
- Use `github.com/charmbracelet/lipgloss` tree utilities
- Build chain by calling `manager.buildAuthenticationChain(identity)`
- Show full chain with arrows: `→`
- Display principal and credentials as nested nodes
- Apply theme colors for hierarchy
- Handle errors in chain building gracefully

### FR-4: JSON/YAML Format Output

**Requirement**: Structured output matching configuration schema

**Output Structure**:
```json
{
  "providers": {
    "provider-name": {
      "kind": "aws/iam-identity-center",
      "region": "us-east-1",
      "start_url": "https://...",
      "default": true,
      ...
    }
  },
  "identities": {
    "identity-name": {
      "kind": "aws/permission-set",
      "default": true,
      "via": {
        "provider": "provider-name"
      },
      "principal": {...},
      ...
    }
  }
}
```

**Implementation**:
- Use standard Go JSON/YAML marshaling
- Output matches `schema.AuthConfig` structure
- Pretty-print with indentation
- Support piping to files or other tools

### FR-5: Filtering

**Requirement**: Support flexible filtering by provider and identity

**Filter Behavior**:

| Flag | Value | Behavior |
|------|-------|----------|
| (none) | - | Show all providers and identities |
| `--providers` | (empty) | Show all providers only |
| `--providers` | `aws-sso` | Show aws-sso provider only |
| `--providers` | `aws-sso,okta` | Show aws-sso and okta providers only |
| `--identities` | (empty) | Show all identities only |
| `--identities` | `admin` | Show admin identity only |
| `--identities` | `admin,dev` | Show admin and dev identities only |

**Validation**:
- Parse comma-separated values
- Trim whitespace from names
- Case-sensitive exact matching
- Show error for non-existent names (or empty result with message)
- Prevent `--providers` and `--identities` together

### FR-6: Chain Visualization

**Requirement**: Show complete authentication chains for all identities

**Chain Building**:
1. For each identity, call `manager.buildAuthenticationChain(identityName)`
2. Chain format: `[provider, identity1, identity2, ..., targetIdentity]`
3. Display format: `provider → identity1 → identity2 → target`

**Error Handling**:
- Circular references: Show error message
- Missing via reference: Show error message
- Broken chain: Show partial chain with error indicator

**Display**:
- Table format: Show immediate parent in VIA columns
- Tree format: Show complete chain as metadata node
- JSON/YAML: No chain (raw configuration only)

### FR-7: Integration with Auth Manager

**Requirement**: Use existing AuthManager interfaces

**Dependencies**:
- `types.AuthManager.GetProviders()` - Get all provider configurations
- `types.AuthManager.GetIdentities()` - Get all identity configurations
- `types.AuthManager.ListProviders()` - Get provider names
- `types.AuthManager.ListIdentities()` - Get identity names
- Internal: `manager.buildAuthenticationChain(identity)` - Get full chain

**Implementation**:
- Load auth config using `config.InitCliConfig()`
- Create auth manager using existing factory
- No new manager methods needed
- May need to access internal chain building (or expose it)

## Non-Functional Requirements

### NFR-1: Performance

**Requirement**: Command executes quickly even with many providers/identities

**Targets**:
- < 100ms for typical configurations (5-10 providers, 20-30 identities)
- < 500ms for large configurations (20+ providers, 100+ identities)
- No blocking network calls (read from local config only)

**Implementation**:
- All data from in-memory config
- No authentication performed (read-only operation)
- Efficient chain building with memoization if needed

### NFR-2: Test Coverage

**Requirement**: Achieve 85-90% test coverage

**Test Categories**:
1. **Command Tests** (`cmd/auth_list_test.go`): 85% coverage
   - Flag parsing and validation
   - Format routing
   - Filter logic
   - Error handling
   - Mock auth manager integration

2. **Formatter Tests** (`pkg/auth/list/formatter_test.go`): 90% coverage
   - Table generation
   - Tree generation
   - JSON/YAML output
   - Chain visualization
   - Edge cases

3. **Integration Tests**:
   - End-to-end command execution
   - Real config parsing
   - Output validation

**Test Fixtures**:
- Multiple provider types (AWS SSO, SAML, GitHub OIDC)
- Various identity kinds (permission-set, assume-role, user)
- Identity chains of varying depth (1-5 levels)
- Edge cases (circular refs, missing refs, empty configs)

### NFR-3: Code Quality

**Requirement**: Follow Atmos coding conventions

**Conventions** (from `CLAUDE.md`):
- All comments end with periods
- Three-part import organization (stdlib, 3rd-party, atmos)
- Performance tracking with `defer perf.Track()`
- Error wrapping with static errors from `errors/errors.go`
- File organization: one command per file
- Prefer many small files over few large files
- Use existing utilities (theme, formatting, etc.)

**Linting**:
- Pass `golangci-lint` with all rules
- Pass pre-commit hooks
- No `--no-verify` commits

### NFR-4: User Experience

**Requirement**: Follow Atmos CLI conventions

**Consistency**:
- Help text follows pattern of existing commands
- Error messages are clear and actionable
- Output follows Atmos styling/theming
- Works well in both TTY and non-TTY (piped) contexts

**Accessibility**:
- Table format works in narrow terminals (graceful wrapping/truncation)
- Tree format handles deep nesting
- Colors only in TTY (plain text when piped)
- Screen reader friendly (semantic structure)

### NFR-5: Documentation

**Requirement**: Comprehensive user documentation

**Documentation Components**:
1. **Usage Examples** (`cmd/markdown/atmos_auth_list_usage.md`)
   - Basic usage
   - All flag combinations
   - Common workflows

2. **Docusaurus Docs** (`website/docs/cli/commands/auth/list.mdx`)
   - Command overview
   - Detailed flag descriptions
   - Multiple examples
   - Output format descriptions
   - Links to related concepts
   - Related commands

3. **Help Text**:
   - Short description
   - Long description with context
   - Flag descriptions
   - Examples embedded in command

**Quality**:
- Clear and concise
- Covers all use cases
- Includes screenshots/examples
- Links to core concepts

## Technical Design

### Architecture

```
cmd/auth_list.go
├─ Flag parsing and validation
├─ AuthManager loading
├─ Filter application
└─ Format routing
    ├─ Table formatter
    ├─ Tree formatter
    ├─ JSON formatter
    └─ YAML formatter

pkg/auth/list/
├─ formatter.go
│  ├─ RenderTable()
│  ├─ RenderTree()
│  ├─ RenderJSON()
│  ├─ RenderYAML()
│  └─ Helper functions
└─ formatter_test.go
```

### Data Flow

```
1. Parse command flags
   ↓
2. Validate flag combinations
   ↓
3. Load Atmos config
   ↓
4. Create AuthManager
   ↓
5. Get providers and identities
   ↓
6. Apply filters (if any)
   ↓
7. Build chains for identities
   ↓
8. Route to appropriate formatter
   ↓
9. Render output
   ↓
10. Print to stdout/stderr
```

### Key Functions

```go
// cmd/auth_list.go

// Execute the list command.
func executeAuthListCommand(cmd *cobra.Command, args []string) error

// Parse and validate filter flags.
func parseFilterFlags(cmd *cobra.Command) (*filterConfig, error)

// Apply filters to providers and identities.
func applyFilters(providers, identities, filters) (filtered, error)

// pkg/auth/list/formatter.go

// Render providers and identities as table.
func RenderTable(providers, identities, options) (string, error)

// Render as tree with chains.
func RenderTree(providers, identities, options) (string, error)

// Render as JSON.
func RenderJSON(providers, identities) (string, error)

// Render as YAML.
func RenderYAML(providers, identities) (string, error)

// Build and format chain for display.
func formatChain(manager, identity) (string, error)

// Create table using bubbles/table.
func createProvidersTable(providers) table.Model

// Create table for identities.
func createIdentitiesTable(identities, chains) table.Model

// Build tree node for provider.
func buildProviderNode(provider) tree.Node

// Build tree node for identity with chain.
func buildIdentityNode(identity, chain) tree.Node
```

### Error Handling

**Error Types**:
- `ErrInvalidAuthConfig`: Configuration errors
- `ErrInvalidFormat`: Invalid format flag
- `ErrMutuallyExclusiveFlags`: --providers and --identities both set
- `ErrInvalidChain`: Circular or broken chain
- `ErrFilterNotFound`: Requested provider/identity not found

**Error Behavior**:
- Validation errors: Exit with helpful message
- Chain errors: Show partial result with warning
- Filter not found: Show empty result with message
- Config errors: Exit with config path and error details

### Dependencies

**Existing**:
- `github.com/spf13/cobra` - CLI framework
- `github.com/spf13/viper` - Config management
- `github.com/charmbracelet/bubbles/table` - Table rendering
- `github.com/charmbracelet/lipgloss` - Styling and tree
- `github.com/cloudposse/atmos/pkg/auth` - Auth management
- `github.com/cloudposse/atmos/pkg/config` - Config loading
- `github.com/cloudposse/atmos/pkg/ui/theme` - Theme colors

**New**:
- None! All required dependencies already exist

## Implementation Plan

### Phase 1: Core Functionality (Table Format)
**Estimated Effort**: 4-6 hours

**Tasks**:
1. Create `cmd/auth_list.go` with basic structure
2. Implement flag parsing and validation
3. Integrate with AuthManager
4. Implement table formatter for providers
5. Implement table formatter for identities
6. Basic error handling
7. Write unit tests for command

**Deliverable**: Working `atmos auth list` with table format

### Phase 2: Tree Format and Chains
**Estimated Effort**: 4-6 hours

**Tasks**:
1. Create `pkg/auth/list/formatter.go`
2. Implement chain building integration
3. Implement tree formatter for providers
4. Implement tree formatter for identities with chains
5. Handle chain visualization edge cases
6. Write formatter unit tests
7. Write chain building tests

**Deliverable**: Tree format with complete chain visualization

### Phase 3: Filtering and Additional Formats
**Estimated Effort**: 3-4 hours

**Tasks**:
1. Implement filter parsing (comma-separated)
2. Implement filter application logic
3. Add JSON formatter
4. Add YAML formatter
5. Write filter tests
6. Write JSON/YAML tests

**Deliverable**: All formats and filtering working

### Phase 4: Polish and Testing
**Estimated Effort**: 4-5 hours

**Tasks**:
1. Comprehensive test coverage (target 85-90%)
2. Integration tests
3. Edge case handling
4. Error message improvements
5. Code review and refactoring
6. Performance optimization if needed
7. Run linter and fix issues

**Deliverable**: Production-ready code with high test coverage

### Phase 5: Documentation
**Estimated Effort**: 2-3 hours

**Tasks**:
1. Create `cmd/markdown/atmos_auth_list_usage.md`
2. Create `website/docs/cli/commands/auth/list.mdx`
3. Update help text with examples
4. Add screenshots if needed
5. Build website and verify
6. Review documentation for clarity

**Deliverable**: Complete documentation

**Total Estimated Effort**: 17-24 hours

## Testing Strategy

### Unit Tests

**Command Tests** (`cmd/auth_list_test.go`):
```go
- TestExecuteAuthListCommand_AllFormats
- TestExecuteAuthListCommand_ProvidersFilter
- TestExecuteAuthListCommand_IdentitiesFilter
- TestExecuteAuthListCommand_SpecificProviders
- TestExecuteAuthListCommand_SpecificIdentities
- TestExecuteAuthListCommand_MutuallyExclusiveFlags
- TestExecuteAuthListCommand_InvalidFormat
- TestExecuteAuthListCommand_EmptyConfig
- TestExecuteAuthListCommand_NonExistentProvider
- TestExecuteAuthListCommand_NonExistentIdentity
```

**Formatter Tests** (`pkg/auth/list/formatter_test.go`):
```go
- TestRenderTable_Providers
- TestRenderTable_Identities
- TestRenderTable_Empty
- TestRenderTree_ProvidersOnly
- TestRenderTree_IdentitiesOnly
- TestRenderTree_FullConfig
- TestRenderTree_ChainVisualization
- TestRenderTree_LongChains
- TestRenderTree_BrokenChains
- TestRenderJSON_ValidOutput
- TestRenderYAML_ValidOutput
- TestFormatChain_SingleLevel
- TestFormatChain_MultiLevel
- TestFormatChain_Circular
- TestCreateProvidersTable_AllTypes
- TestCreateIdentitiesTable_AllKinds
- TestBuildProviderNode_AllAttributes
- TestBuildIdentityNode_WithChain
```

### Integration Tests

```go
- TestAuthListCommand_EndToEnd
- TestAuthListCommand_WithRealConfig
- TestAuthListCommand_PipedOutput
- TestAuthListCommand_TTYDetection
```

### Test Coverage Targets

| Package | Target Coverage |
|---------|----------------|
| `cmd/auth_list.go` | 85% |
| `pkg/auth/list/formatter.go` | 90% |
| Overall | 85-90% |

### Test Fixtures

**Provider Types**:
- AWS IAM Identity Center (SSO)
- AWS SAML
- GitHub OIDC

**Identity Types**:
- aws/permission-set
- aws/assume-role
- aws/user

**Chain Scenarios**:
- Single level: provider → identity
- Two levels: provider → identity1 → identity2
- Three levels: provider → id1 → id2 → id3
- Long chain: 5+ levels
- Standalone: identity with no via
- Circular: id1 → id2 → id1 (error case)
- Broken: via references non-existent entity (error case)

## Acceptance Criteria

### Functional

- [ ] Command executes successfully with no arguments
- [ ] Table format displays providers and identities clearly
- [ ] Tree format shows hierarchical relationships and chains
- [ ] JSON format produces valid, parseable JSON
- [ ] YAML format produces valid, parseable YAML
- [ ] `--providers` flag filters to show only providers
- [ ] `--providers aws-sso` shows only aws-sso provider
- [ ] `--providers aws-sso,okta` shows multiple providers
- [ ] `--identities` flag filters to show only identities
- [ ] `--identities admin` shows only admin identity
- [ ] `--identities admin,dev` shows multiple identities
- [ ] `--providers` and `--identities` together shows error
- [ ] Authentication chains display correctly in tree format
- [ ] Long chains (5+ levels) display correctly
- [ ] Broken chains show helpful error messages
- [ ] Non-existent filter names show helpful messages
- [ ] Empty configs show appropriate messages

### Technical

- [ ] Test coverage ≥ 85% overall
- [ ] Test coverage ≥ 90% for formatters
- [ ] All tests pass
- [ ] `make lint` passes with no errors
- [ ] Pre-commit hooks pass
- [ ] Code follows all CLAUDE.md conventions
- [ ] Comments end with periods
- [ ] Imports organized correctly
- [ ] Performance tracking added to public functions
- [ ] Error wrapping uses static errors
- [ ] No compilation warnings or errors

### Documentation

- [ ] Usage markdown created with all examples
- [ ] Docusaurus documentation created
- [ ] Help text clear and comprehensive
- [ ] Website builds successfully
- [ ] No broken links in documentation
- [ ] Examples cover all major use cases
- [ ] Flag descriptions are clear

### User Experience

- [ ] Output is clear and readable
- [ ] Colors applied consistently
- [ ] Works in TTY and non-TTY contexts
- [ ] Error messages are actionable
- [ ] Long URLs truncate gracefully
- [ ] Tables fit in standard terminal widths
- [ ] Tree format handles deep nesting

## Risks and Mitigations

### Risk 1: Complex Chain Building
**Impact**: High
**Probability**: Medium
**Mitigation**:
- Reuse existing `buildAuthenticationChain()` logic from manager
- Comprehensive testing of chain scenarios
- Graceful error handling for broken chains
- May need to expose internal chain building method

### Risk 2: Performance with Large Configs
**Impact**: Medium
**Probability**: Low
**Mitigation**:
- No network calls (read from local config)
- Chain building is fast (in-memory graph traversal)
- Add performance tests if needed
- Optimize if issues arise

### Risk 3: Test Coverage Requirements
**Impact**: Medium
**Probability**: Low
**Mitigation**:
- Write tests alongside implementation
- Use table-driven tests for comprehensive coverage
- Mock auth manager for unit tests
- Integration tests for end-to-end validation

### Risk 4: Breaking Changes to Auth Interfaces
**Impact**: Low
**Probability**: Low
**Mitigation**:
- Use existing public interfaces (no changes needed)
- If internal access needed, discuss with team
- Version compatibility considerations

## Success Metrics

### Usage Metrics
- Number of invocations of `atmos auth list`
- Most common format used (table vs tree vs json/yaml)
- Filter usage patterns (providers vs identities)

### Quality Metrics
- Test coverage: ≥ 85%
- Zero critical bugs in first 30 days
- Documentation completeness: 100%
- Linter pass rate: 100%

### User Satisfaction
- Clear documentation (measured by support tickets)
- Intuitive UX (measured by user feedback)
- Helpful error messages (measured by repeat invocations)

## Future Enhancements

**Out of Scope for Initial Release**:

1. **Interactive Mode**: TUI for selecting providers/identities
2. **Graphviz Export**: Generate visual diagrams of chains
3. **Diff Mode**: Compare authentication configs across environments
4. **Validation**: Integrate with `atmos auth validate`
5. **Credential Status**: Show which credentials are cached/valid
6. **Search**: Fuzzy search across providers and identities
7. **Watch Mode**: Auto-refresh when config changes
8. **Export Formats**: CSV, Markdown table, HTML

## References

- [Atmos Authentication Documentation](https://atmos.tools/cli/commands/auth/)
- [CLAUDE.md](../../CLAUDE.md) - Atmos coding conventions
- [Error Handling Strategy](./error-handling-strategy.md)
- [Testing Strategy](./testing-strategy.md)
- [Charmbracelet Bubbles Table](https://github.com/charmbracelet/bubbles/tree/master/table)
- [Charmbracelet Lipgloss](https://github.com/charmbracelet/lipgloss)

## Appendix

### Example Output

#### Table Format (Default)
```
PROVIDERS
┌──────────┬─────────────────────────────┬────────────┬─────────────────────────┬─────────┐
│ NAME     │ KIND                        │ REGION     │ START URL / URL         │ DEFAULT │
├──────────┼─────────────────────────────┼────────────┼─────────────────────────┼─────────┤
│ aws-sso  │ aws/iam-identity-center     │ us-east-1  │ https://d-abc.awsapps…  │ ✓       │
│ okta     │ aws/saml                    │ us-west-2  │ https://company.okta…   │         │
│ github   │ github/oidc                 │ -          │ -                       │         │
└──────────┴─────────────────────────────┴────────────┴─────────────────────────┴─────────┘

IDENTITIES
┌──────────────┬──────────────────────┬──────────────────┬──────────────┬─────────┬──────────┐
│ NAME         │ KIND                 │ VIA PROVIDER     │ VIA IDENTITY │ DEFAULT │ ALIAS    │
├──────────────┼──────────────────────┼──────────────────┼──────────────┼─────────┼──────────┤
│ base         │ aws/permission-set   │ aws-sso          │ -            │ ✓       │ -        │
│ team-admin   │ aws/assume-role      │ -                │ base         │         │ -        │
│ project-dev  │ aws/assume-role      │ -                │ team-admin   │         │ -        │
│ ci           │ aws/user             │ aws-user         │ -            │         │ -        │
└──────────────┴──────────────────────┴──────────────────┴──────────────┴─────────┴──────────┘
```

#### Tree Format
```
Authentication Configuration

Providers
├─ aws-sso (aws/iam-identity-center) [DEFAULT]
│  ├─ Region: us-east-1
│  └─ Start URL: https://d-abc123.awsapps.com/start
├─ okta (aws/saml)
│  ├─ Region: us-west-2
│  └─ URL: https://company.okta.com/app/amazon_aws/123/sso/saml
└─ github (github/oidc)

Identities
├─ base (aws/permission-set) [DEFAULT]
│  ├─ Via Provider: aws-sso
│  ├─ Chain: aws-sso → base
│  └─ Principal:
│     ├─ Account: 123456789012
│     └─ Permission Set: AdministratorAccess
├─ team-admin (aws/assume-role)
│  ├─ Via Identity: base
│  ├─ Chain: aws-sso → base → team-admin
│  └─ Principal:
│     └─ Role ARN: arn:aws:iam::123456789012:role/TeamAdmin
├─ project-dev (aws/assume-role)
│  ├─ Via Identity: team-admin
│  ├─ Chain: aws-sso → base → team-admin → project-dev
│  └─ Principal:
│     └─ Role ARN: arn:aws:iam::999999999999:role/ProjectDeveloper
└─ ci (aws/user)
   ├─ Standalone Identity (no provider)
   └─ Chain: ci
```

#### JSON Format
```json
{
  "providers": {
    "aws-sso": {
      "kind": "aws/iam-identity-center",
      "region": "us-east-1",
      "start_url": "https://d-abc123.awsapps.com/start",
      "default": true
    },
    "okta": {
      "kind": "aws/saml",
      "region": "us-west-2",
      "url": "https://company.okta.com/app/amazon_aws/123/sso/saml"
    },
    "github": {
      "kind": "github/oidc"
    }
  },
  "identities": {
    "base": {
      "kind": "aws/permission-set",
      "default": true,
      "via": {
        "provider": "aws-sso"
      },
      "principal": {
        "account": {
          "id": "123456789012"
        },
        "name": "AdministratorAccess"
      }
    },
    "team-admin": {
      "kind": "aws/assume-role",
      "via": {
        "identity": "base"
      },
      "principal": {
        "assume_role": "arn:aws:iam::123456789012:role/TeamAdmin"
      }
    },
    "project-dev": {
      "kind": "aws/assume-role",
      "via": {
        "identity": "team-admin"
      },
      "principal": {
        "assume_role": "arn:aws:iam::999999999999:role/ProjectDeveloper"
      }
    },
    "ci": {
      "kind": "aws/user"
    }
  }
}
```

### Configuration Example

```yaml
# atmos.yaml
auth:
  providers:
    aws-sso:
      kind: aws/iam-identity-center
      region: us-east-1
      start_url: https://d-abc123.awsapps.com/start
      default: true

    okta:
      kind: aws/saml
      region: us-west-2
      url: https://company.okta.com/app/amazon_aws/123/sso/saml

    github:
      kind: github/oidc

  identities:
    base:
      kind: aws/permission-set
      default: true
      via:
        provider: aws-sso
      principal:
        account:
          id: "123456789012"
        name: AdministratorAccess

    team-admin:
      kind: aws/assume-role
      via:
        identity: base
      principal:
        assume_role: arn:aws:iam::123456789012:role/TeamAdmin

    project-dev:
      kind: aws/assume-role
      via:
        identity: team-admin
      principal:
        assume_role: arn:aws:iam::999999999999:role/ProjectDeveloper

    ci:
      kind: aws/user
```

---

**Document Version**: 1.0
**Last Updated**: 2025-01-15
**Status**: Ready for Implementation
