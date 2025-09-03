# Symlink Security for Vendor Operations

## Problem Statement

HashiCorp's go-getter library (used by Atmos for vendoring) has a critical vulnerability (CVE-2025-8959, CVSS 7.5) that allows symlink attacks during subdirectory downloads. Malicious repositories can use symlinks to gain unauthorized read access to files outside the intended vendor boundaries, potentially exposing sensitive system files like `/etc/passwd` or SSH keys.

HashiCorp's fix in go-getter v1.7.9 simply disables all symlinks for git operations, breaking legitimate use cases where repositories contain valid symlinks. This approach is unacceptable for Atmos users who rely on symlinks within their infrastructure repositories.

## Goals

1. **Protect against symlink attacks** - Prevent unauthorized file access outside vendor boundaries
2. **Maintain functionality** - Support legitimate symlinks within repository boundaries  
3. **Provide flexibility** - Allow users to choose their security posture based on trust levels
4. **Ensure backward compatibility** - Don't break existing workflows that depend on symlinks
5. **Enable auditing** - Log security decisions for compliance and debugging

## Non-Goals

1. **Changing go-getter upstream** - We work around their limitations rather than waiting for upstream fixes
2. **Scanning repository content** - We don't analyze what files symlinks point to, only their boundaries
3. **Runtime symlink creation** - This PRD only covers symlinks that exist in source repositories
4. **Other security vulnerabilities** - This PRD is specifically for CVE-2025-8959

## User Stories

### Story 1: Security-Conscious DevOps Engineer
**As a** DevOps engineer vendoring from public repositories  
**I want** protection against malicious symlinks  
**So that** I don't accidentally expose sensitive files from my system

**Acceptance Criteria:**
- Default configuration blocks symlinks that escape repository boundaries
- Clear warnings are logged when symlinks are rejected
- No manual configuration needed for basic protection

### Story 2: Enterprise Security Team
**As a** security team member  
**I want** to enforce maximum security when vendoring from untrusted sources  
**So that** we maintain compliance with security policies

**Acceptance Criteria:**
- Can configure policy to reject ALL symlinks
- Policy can be enforced via `atmos.yaml` configuration
- Audit logs show all symlink decisions

### Story 3: Platform Engineer with Legacy Infrastructure
**As a** platform engineer with existing symlink-dependent infrastructure  
**I want** to continue using symlinks within my trusted repositories  
**So that** I don't have to refactor working infrastructure code

**Acceptance Criteria:**
- Can configure policy to allow all symlinks (legacy behavior)
- Internal repository symlinks continue to work with default policy
- Clear documentation on security implications

## Design Decisions

### Decision 1: Three-Policy Approach
**Choice:** Implement three policies: `allow_safe`, `reject_all`, `allow_all`

**Why:**
- Balances security with functionality
- Provides clear, understandable options
- Covers all identified use cases

**Alternatives Considered:**
- Binary on/off switch - Too limiting, doesn't handle mixed trust scenarios
- Per-repository configuration - Too complex for initial implementation
- Allowlist/blocklist patterns - Adds complexity without clear benefit

### Decision 2: Default to `allow_safe`
**Choice:** Make `allow_safe` the default policy

**Why:**
- Provides immediate protection without breaking most use cases
- Follows security best practices of secure-by-default
- Internal symlinks still work

**Alternatives Considered:**
- Default to `allow_all` - Leaves users vulnerable
- Default to `reject_all` - Too disruptive to existing workflows
- No default (require explicit configuration) - Poor user experience

### Decision 3: Configuration in `atmos.yaml`
**Choice:** Add `vendor.policy.symlinks` configuration

**Why:**
- Centralized configuration
- Consistent with Atmos patterns
- Supports both nested and dot notation

**Alternatives Considered:**
- Environment variables - Less discoverable
- Command-line flags - Requires changes to all vendor commands
- Per-vendor-file config - Too granular for security policy

## Technical Specification

### Configuration Schema
```yaml
vendor:
  policy:
    symlinks: "allow_safe"  # Options: allow_safe, reject_all, allow_all
```

### Security Module API
```go
type SymlinkPolicy string

const (
    PolicyAllowSafe SymlinkPolicy = "allow_safe"  // Default
    PolicyRejectAll SymlinkPolicy = "reject_all"
    PolicyAllowAll  SymlinkPolicy = "allow_all"
)

func CreateSymlinkHandler(baseDir string, policy SymlinkPolicy) func(string) cp.SymlinkAction
func ValidateSymlinks(root string, policy SymlinkPolicy) error
func IsSymlinkSafe(symlink, boundary string) bool
```

### Validation Algorithm
1. Read symlink target
2. Resolve to absolute path
3. Check if path is within boundary:
   - Calculate relative path from boundary to target
   - If relative path starts with `..` → unsafe
   - If relative path is absolute → unsafe
   - Otherwise → safe

### Integration Points
- `vendor_component_utils.go` - Component vendoring
- `vendor_utils.go` - General vendoring utilities
- `vendor_model.go` - Vendor model operations
- `copy_glob.go` - Pattern-based copying
- `git_getter.go` - Git-specific operations

## Implementation Plan

### Phase 1: Core Security Module (Completed)
- [x] Create `pkg/security/symlink_validator.go`
- [x] Implement three policies
- [x] Add boundary validation logic
- [x] Create comprehensive unit tests

### Phase 2: Integration (Completed)
- [x] Update schema to support configuration
- [x] Integrate with copy operations
- [x] Update git operations
- [x] Add integration tests

### Phase 3: Documentation (Completed)
- [x] Document configuration in vendor-pull.mdx
- [x] Add security best practices
- [x] Include troubleshooting guide
- [x] Provide migration examples

### Phase 4: Future Enhancements (Not in scope)
- [ ] Per-source policy configuration
- [ ] Symlink allowlist patterns
- [ ] Metrics on rejected symlinks
- [ ] Integration with security scanning tools

## Success Metrics

1. **Security**: Zero symlink escapes with `allow_safe` or `reject_all` policies
2. **Compatibility**: Existing workflows continue to function with appropriate policy
3. **Performance**: No measurable performance impact on vendor operations
4. **Adoption**: Clear documentation leads to correct policy selection
5. **Auditability**: Security teams can track symlink decisions via logs

## Security Considerations

### Threat Model
- **Attacker**: Malicious repository maintainer
- **Attack Vector**: Symlinks in repository pointing to sensitive files
- **Impact**: Unauthorized read access to system files
- **Mitigation**: Boundary validation prevents escape

### Residual Risks
1. **`allow_all` policy**: Users who explicitly choose this accept the risk
2. **Broken symlinks**: May cause vendor operations to fail (safe failure)
3. **Race conditions**: Symlink target could change between validation and use (minimal risk in vendor context)

## Dependencies

- `github.com/otiai10/copy` v1.14.1 - Provides symlink handling callbacks
- `github.com/hashicorp/go-getter` v1.7.9 - Fixed version with CVE patch
- No additional external dependencies required

## Testing Strategy

### Unit Tests
- Policy parsing and validation
- Boundary checking algorithm
- Each policy behavior
- Edge cases (circular symlinks, broken symlinks)

### Integration Tests
- CVE-2025-8959 specific attack scenarios
- Policy integration with vendor operations
- Cross-platform compatibility (Linux, macOS)
- Performance benchmarks

### Manual Testing
- Vendor operations with various repository structures
- Configuration validation
- Log output verification
- Documentation accuracy

## Documentation Plan

1. **Configuration Guide**: How to set up symlink policies
2. **Security Best Practices**: When to use each policy
3. **Migration Guide**: Moving from vulnerable versions
4. **Troubleshooting**: Common issues and solutions
5. **API Reference**: For developers extending Atmos

## Rollout Strategy

1. **Default Protection**: New installations get `allow_safe` by default
2. **Backward Compatibility**: Existing installations continue to work
3. **Deprecation Notice**: Warn users about security implications
4. **Migration Period**: Give users time to adjust configurations
5. **Full Enforcement**: Consider making `allow_safe` mandatory in future major version

## Alternative Approaches Considered

### Alternative 1: Fork go-getter
**Rejected because:**
- Maintenance burden
- Divergence from upstream
- Difficult to track security updates

### Alternative 2: Implement custom vendoring
**Rejected because:**
- Significant development effort
- Loss of go-getter features
- Increased surface area for bugs

### Alternative 3: Disable symlinks entirely
**Rejected because:**
- Breaks legitimate use cases
- Not acceptable to users with symlink dependencies
- Same limitation as HashiCorp's approach

## Open Questions

1. Should we add per-repository policy overrides in the future?
2. Should we collect telemetry on policy usage?
3. Should we integrate with external security scanners?
4. Should we add a "quarantine" mode for suspicious symlinks?

## References

- [CVE-2025-8959](https://nvd.nist.gov/vuln/detail/CVE-2025-8959)
- [HashiCorp Security Advisory HCSEC-2025-23](https://discuss.hashicorp.com/t/hcsec-2025-23-hashicorp-go-getter-vulnerable-to-arbitrary-read-through-symlink-attack/76242)
- [go-getter v1.7.9 Release Notes](https://github.com/hashicorp/go-getter/releases/tag/v1.7.9)
- [POSIX Symlink Specification](https://pubs.opengroup.org/onlinepubs/9699919799/functions/symlink.html)

## Appendix

### Example Attack Scenario
```bash
# Malicious repository structure
malicious-repo/
├── components/
│   └── terraform/
│       └── vpc/
│           ├── main.tf
│           └── secrets.tf -> /etc/passwd  # Attack symlink
```

### Example Configuration
```yaml
# Maximum security for public sources
vendor:
  policy:
    symlinks: "reject_all"

# Balanced security (default)
vendor:
  policy:
    symlinks: "allow_safe"

# Legacy compatibility
vendor:
  policy:
    symlinks: "allow_all"
```

### Example Log Output
```
WARN Symlink rejected - target outside boundary src=/tmp/vendor/evil.tf boundary=/tmp/vendor target=/etc/passwd
INFO Symlink validated and allowed src=/tmp/vendor/link.tf target=/tmp/vendor/shared/common.tf
```