# Vendor Update Product Requirements Document

## Executive Summary

The `atmos vendor update` command provides automated version management for vendored components in Atmos configurations. It checks upstream sources for newer versions and updates version references in vendor configuration files while preserving YAML structure, comments, and anchors.

## Problem Statement

Currently, maintaining up-to-date versions of vendored components requires:
- Manual checking of upstream repositories for new versions
- Manual editing of vendor configuration files
- Risk of breaking YAML structure (anchors, comments, formatting)
- No visibility into available updates across multiple components
- Tedious process when managing many components with imports

## Goals

### Primary Goals
- Automate checking for component version updates from upstream sources
- Update version references in vendor configuration files programmatically
- Preserve YAML structure including anchors, comments, and formatting
- Support both `vendor.yaml` and `component.yaml` formats
- Handle vendor config imports recursively

### Non-Goals (Current Scope)
- Semantic version range support (requires lock files)
- OCI registry version checking
- S3/GCS bucket version checking
- Automatic major version upgrades (requires compatibility checking)

## Solution Overview

The `atmos vendor update` command will:
1. Read vendor configurations including all imports
2. Check upstream sources for newer versions
3. Update version references in the appropriate config files
4. Optionally pull the updated components

## Supported Upstream Sources

| Source Type | Version Detection | Support Status | Example |
|-------------|------------------|----------------|---------|
| Git (GitHub, GitLab, Bitbucket) | Tags & Commits | ✅ Implemented | `github.com/cloudposse/terraform-aws-components.git` |
| OCI Registries | Registry API | ⏳ Future | `oci://ghcr.io/cloudposse/components` |
| HTTP/HTTPS Direct Files | N/A | ❌ Not Applicable | `https://raw.githubusercontent.com/...` |
| Local File System | N/A | ❌ Not Applicable | `./components/vpc` |
| Amazon S3 | Object Metadata | ⏳ Future | `s3://bucket/components/` |
| Google GCS | Object Metadata | ⏳ Future | `gs://bucket/components/` |

## User Interface

### Command Structure

```bash
# Check for available updates (dry-run)
atmos vendor update --check

# Update version references in config files
atmos vendor update

# Update versions and pull components
atmos vendor update --pull

# Update specific component
atmos vendor update --component vpc

# Update components with specific tags
atmos vendor update --tags terraform,networking
```

### Flags

- `--check` - Check for updates without modifying files (dry-run mode)
- `--pull` - Update version references AND pull the new component versions
- `--component <name>` - Update specific component only
- `--tags <tags>` - Update components with specific tags (comma-separated)
- `--type <type>` - Component type: terraform or helmfile (default: terraform)

### Terminal UI Experience

```
Checking for vendor updates...

[=========>] 9/10 Checking terraform-aws-ecs...

✓ terraform-aws-vpc (1.323.0 → 1.372.0)
✓ terraform-aws-s3-bucket (4.1.0 → 4.2.0)
✓ terraform-aws-eks (2.1.0 - up to date)
⚠ custom-module (skipped - templated version {{.Version}})
⚠ terraform-aws-ecs (skipped - OCI registry not yet supported)
✓ terraform-aws-rds (5.0.0 → 5.1.0)

Found 3 updates available. Use 'atmos vendor update' to update the configuration files.
```

## Technical Design

### Architecture

1. **Command Layer** (`cmd/vendor_update.go`)
   - Parse command-line flags
   - Initialize configuration
   - Call execution layer

2. **Execution Layer** (`internal/exec/vendor_update.go`)
   - Process vendor configurations and imports
   - Check for version updates
   - Update configuration files
   - Optionally trigger vendor pull

3. **Version Checking** (`internal/exec/vendor_version_check.go`)
   - Git repository tag/commit checking
   - Version comparison logic
   - Template detection and skipping

4. **YAML Preservation** (`internal/exec/vendor_yaml_updater.go`)
   - Parse YAML while preserving structure
   - Update specific version fields
   - Maintain anchors, comments, and formatting

### Implementation Phases

#### Phase 1: Core Functionality (Current)
- Git repository version checking
- Basic YAML update with structure preservation
- Import chain handling
- TUI integration
- Mock-based unit tests

#### Phase 2: Enhanced Sources (Future)
- OCI registry support
- S3/GCS bucket support
- GitHub/GitLab API optimization
- Rate limit handling

#### Phase 3: Advanced Features (Future)
- Semantic version ranges
- Lock file generation
- Compatibility checking
- Rollback capabilities
- Update policies (major/minor/patch)

## Testing Requirements

### Unit Tests (Mock-Based)
- Version comparison logic
- YAML preservation including anchors
- Template detection
- Import chain processing
- File update operations

### Integration Tests
- End-to-end update workflow
- Real Git repository checking
- Multi-file import scenarios
- Both config format support

### Test Coverage
- Minimum 80% coverage for new code
- All error conditions tested
- All flag combinations tested

## Success Metrics

- Zero YAML structure corruption (anchors, comments preserved)
- Version checking accuracy > 99%
- Performance: < 1 second per component check
- User satisfaction with TUI experience
- Reduced time spent on manual version updates

## Migration Path

### From `vendor diff`
The existing `vendor diff` command will be deprecated in favor of `vendor update --check`. Users will be guided to use the new command through deprecation notices.

### Backward Compatibility
- All existing vendor configurations will work without changes
- Templated versions will be automatically skipped
- Import chains will be fully supported

## Security Considerations

- No credentials stored in configuration files
- Git operations use existing SSH/HTTPS credentials
- No automatic major version updates (breaking changes)
- Audit trail through Git commits of config changes

## Documentation Requirements

1. CLI command documentation (`website/docs/cli/commands/vendor/vendor-update.mdx`)
2. Migration guide from `vendor diff`
3. Examples for common use cases
4. Supported sources matrix
5. Troubleshooting guide

## Future Enhancements

1. **Semantic Version Ranges**
   - Support `~>` and `^` operators
   - Generate lock files for reproducibility

2. **Update Policies**
   - Configure per-component update strategies
   - Automatic patch updates, manual major updates

3. **Compatibility Checking**
   - Analyze breaking changes
   - Integration with component test suites

4. **Notifications**
   - Webhook integration for new versions
   - Email/Slack notifications

## Conclusion

The `atmos vendor update` command will significantly improve the developer experience for maintaining vendored components. By automating version checking and updates while preserving YAML structure, it reduces manual effort and potential errors in configuration management.
