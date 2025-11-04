# Future Considerations for Secret Masking

**Status**: Draft - Not Implementing
**Created**: 2025-11-02

> **⚠️ NOTE:** This document explores future enhancements that are **not** being implemented at this time. The current implementation uses 8 hardcoded patterns which are sufficient for current needs.

## Overview

This document consolidates exploration of future enhancements to Atmos's secret masking system, specifically around integrating third-party pattern libraries like Gitleaks, TruffleHog, and Secrets-Patterns-DB.

## Current State

**What We Have:**
- 8 hardcoded patterns in `pkg/io/global.go`
- Public API for dynamic patterns: `io.RegisterPattern(regex)`, `io.RegisterValue(literal)`, `io.RegisterSecret(value)`
- Configuration schema defined: `settings.terminal.mask.patterns` and `settings.terminal.mask.literals`
- Automatic masking via global writers (`io.Data`, `io.UI`)
- CLI control via `--mask` flag
- Thread-safe masking engine
- Format-aware masking (base64, URL-encoded, hex)

**What's Missing:**
- **Configuration loading** - Schema exists but patterns/literals from atmos.yaml not loaded into masker
- Environment variable registration from config (currently only hardcoded env vars)

**Why This Is Sufficient:**
- Covers most common secrets (AWS, GitHub, OpenAI, generic tokens)
- Developers can register patterns programmatically: `io.RegisterPattern("ACME_[A-Z0-9]{32}")`
- Zero library dependencies
- Fast initialization (<50ms)
- Simple to maintain and test

## Pattern Library Options

### Option 1: Gitleaks Integration

**Overview:** Gitleaks provides 120+ battle-tested regex patterns organized into categories.

**Benefits:**
- Comprehensive coverage (AWS, GitHub, GitLab, Slack, Datadog, etc.)
- Battle-tested patterns from security community
- Regular updates for new secret formats
- Category-based control (aws, github, generic, etc.)

**Challenges:**
- External dependency (gitleaks TOML file)
- Increased complexity (rule engine, allowlists, entropies)
- Slower initialization (parse 120+ patterns)
- May increase false positives (generic patterns)

**Proposed Configuration:**
```yaml
settings:
  terminal:
    mask:
      libraries:
        gitleaks:
          enabled: true
          categories:
            aws: true
            github: true
            generic: false  # Reduce false positives
```

**Implementation Approach:**
1. Embed gitleaks.toml in binary using `//go:embed`
2. Parse TOML at initialization
3. Convert rules to regex patterns
4. Register with masker engine
5. Add category-level enable/disable

**See:** Original exploration in `gitleaks-integration-design.md`

---

### Option 2: TruffleHog Integration

**Overview:** TruffleHog uses detector-based approach with entropy analysis.

**Benefits:**
- High-confidence detection (uses entropy + patterns)
- Fewer false positives than pure regex
- Detector architecture more extensible
- Active community and updates

**Challenges:**
- More complex integration (detector interface, not just regex)
- Higher CPU cost (entropy calculations)
- External dependency on detector definitions
- May need to implement custom detector interface

**Proposed Configuration:**
```yaml
settings:
  terminal:
    mask:
      libraries:
        trufflehog:
          enabled: true
          detectors:
            - aws
            - github
            - slack
            - stripe
```

**Implementation Approach:**
1. Define detector interface compatible with TruffleHog
2. Implement entropy checker
3. Register detectors by name
4. Add confidence threshold configuration

---

### Option 3: Secrets-Patterns-DB

**Overview:** Community-maintained database of secret patterns.

**Benefits:**
- Curated by security community
- Regularly updated
- JSON format (easier to parse)
- Organized by provider/service

**Challenges:**
- Less mature than Gitleaks/TruffleHog
- External dependency
- May need custom validation
- Unclear update frequency

**Proposed Configuration:**
```yaml
settings:
  terminal:
    mask:
      libraries:
        secrets-patterns-db:
          enabled: true
          categories:
            - passwords
            - api-keys
            - certificates
```

---

### Option 4: Custom Pattern Library Format

**Overview:** Define our own simple JSON/YAML format for pattern libraries.

**Benefits:**
- Full control over format
- Optimized for Atmos use case
- No conversion needed
- Community can contribute patterns

**Challenges:**
- Need to maintain pattern quality
- More work to build initial library
- Need update mechanism
- Compete with established libraries

**Example Format:**
```yaml
# patterns.yaml
version: 1.0
patterns:
  - id: aws-access-key
    name: AWS Access Key ID
    regex: 'AKIA[0-9A-Z]{16}'
    category: aws
    severity: high

  - id: github-pat
    name: GitHub Personal Access Token
    regex: 'ghp_[A-Za-z0-9]{36}'
    category: github
    severity: high
```

---

## Configuration Design Options

### Challenge: Avoiding Deep Nesting

**Problem:** Initial proposal had 7 levels of nesting:
```yaml
settings:          # 1
  terminal:        # 2
    mask:          # 3
      libraries:   # 4
        gitleaks:  # 5
          enabled: # 6
          categories: # 6
            aws:   # 7  ← TOO DEEP!
```

User feedback: "That's just insane at that point."

### Design Option A: Simplified Two-Level (Recommended)

**Depth:** 4 levels maximum

```yaml
settings:
  terminal:
    mask:
      enabled: true

      # Libraries (can enable multiple)
      gitleaks: true         # Enable all patterns

      # Or be selective
      trufflehog:
        - slack
        - stripe

      # Custom patterns
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'

      literals:
        - "my-secret"

      env_vars:
        - COMPANY_API_KEY
```

**Benefits:**
- Only 4 levels deep
- Simple boolean enable: `gitleaks: true`
- Category list: `gitleaks: [aws, github]`
- Each library is just a list
- Uniform structure

**Type Handling:**
```go
type LibraryConfig interface{}  // Can be bool or []string

// Parse:
// - true = enable all categories
// - false = disable all
// - ["aws", "github"] = enable only those
```

---

### Design Option B: Tag-Based Selection

**Depth:** 4 levels maximum

```yaml
settings:
  terminal:
    mask:
      enabled: true

      # Select what to include (tags)
      include:
        - gitleaks         # All Gitleaks patterns
        - aws              # AWS patterns from all libraries
        - github           # GitHub patterns from all libraries
        - slack            # Slack patterns from all libraries

      # Exclude specific things
      exclude:
        - generic          # Exclude generic patterns

      # Custom patterns (tagged)
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'
          tags: [company, api-key]
```

**Benefits:**
- 4 levels deep
- Works across libraries (include: [aws] gets AWS from all)
- Can exclude categories
- Powerful selection model

**Challenges:**
- Need to establish tag taxonomy
- Potential conflicts if library name = category name
- Less explicit about source library

---

### Design Option C: Prefix-Based Flat Structure

**Depth:** 4 levels maximum

```yaml
settings:
  terminal:
    mask:
      enabled: true

      # Library enable/disable
      lib-gitleaks: true
      lib-trufflehog: false

      # Category configuration
      gitleaks-aws: true
      gitleaks-github: true
      gitleaks-generic: false

      trufflehog-slack: true
      trufflehog-stripe: true

      # Custom patterns
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'
```

**Benefits:**
- Completely flat - everything at same level
- Prefixes provide visual grouping
- Easy to enable: `lib-gitleaks: true`
- Easy to configure: `gitleaks-aws: true`

**Challenges:**
- Many top-level keys
- Prefix convention needs consistency
- Less structured than nested approach

---

### Design Option D: Ultra-Flat String Arrays

**Depth:** 3 levels only!

```yaml
settings:
  terminal:
    mask:
      enabled: true

      # Just list what you want
      enable:
        - gitleaks:aws
        - gitleaks:github
        - trufflehog:slack
        - pattern:company-key
        - literal:my-secret
        - env:COMPANY_API_KEY

      # Define custom patterns separately
      patterns:
        company-key: 'ACME_[A-Z0-9]{32}'
        employee-id: 'EMP-\d{6}'
```

**Benefits:**
- Flattest possible (only 3 levels!)
- One list to rule them all
- Super simple config

**Challenges:**
- String parsing required everywhere
- Pattern definitions separate from usage
- Less structured
- Harder to validate

---

### Comparison Table

| Design | Max Depth | Library Enable | Category Control | Readability | Extensibility |
|--------|-----------|----------------|------------------|-------------|---------------|
| **A: Two-Level** ⭐ | 4 | `gitleaks: true` | `gitleaks: [aws]` | Excellent | Good |
| **B: Tag-Based** | 4 | `include: [gitleaks]` | `include: [aws]` | Excellent | Excellent |
| **C: Prefix-Based** | 4 | `lib-gitleaks: true` | `gitleaks-aws: true` | OK | Excellent |
| **D: Ultra-Flat** | 3 | `- gitleaks:*` | `- gitleaks:aws` | Good | OK |

**Recommended:** Option A (Simplified Two-Level) for balance of simplicity and flexibility.

---

## Implementation Strategy

### Phase 1: Foundation (Current - Implemented)
- ✅ Global writers pattern (`io.Data`, `io.UI`)
- ✅ Masking engine with thread safety
- ✅ 8 hardcoded patterns
- ✅ CLI `--mask` flag
- ✅ Format-aware masking
- ✅ Test coverage

### Phase 2: Load Custom Patterns from Configuration (High Priority)

**Status**: Schema exists, loading NOT implemented

**What's Already Built:**
- ✅ Schema defined: `settings.terminal.mask.patterns` ([]string) and `literals` ([]string)
- ✅ Public API: `io.RegisterPattern()`, `io.RegisterValue()`
- ✅ Validation and thread safety in masker

**What's Missing:**
- ❌ Load patterns/literals from atmos.yaml during initialization
- ❌ Validate regex patterns at config load time
- ❌ Error handling for invalid patterns

**Implementation:**
```go
// In cmd/root.go PersistentPreRun, after io.Initialize():
for _, pattern := range atmosConfig.Settings.Terminal.Mask.Patterns {
    if err := io.RegisterPattern(pattern); err != nil {
        // Log warning, don't fail
    }
}
for _, literal := range atmosConfig.Settings.Terminal.Mask.Literals {
    io.RegisterValue(literal)
}
```

**Example Configuration:**
```yaml
settings:
  terminal:
    mask:
      enabled: true
      patterns:
        - 'ACME_[A-Z0-9]{32}'           # Company API keys
        - 'api_key=[A-Za-z0-9]+'        # API key parameters
      literals:
        - "my-hardcoded-secret"
```

**Priority**: HIGH - This is the reason we're not implementing pattern libraries yet.

### Phase 3: Pattern Library Integration (Future)
- Implement library registry pattern
- Add Gitleaks as first library
- Embed gitleaks.toml in binary
- Add category-level control
- Update configuration schema

**Example:**
```yaml
settings:
  terminal:
    mask:
      gitleaks: true  # Enable all
      # Or selective:
      # gitleaks: [aws, github]
```

### Phase 4: Multi-Library Support (Future)
- Add TruffleHog support
- Add Secrets-Patterns-DB support
- Allow multiple libraries simultaneously
- Handle pattern conflicts/duplicates

**Example:**
```yaml
settings:
  terminal:
    mask:
      gitleaks: [aws, github]
      trufflehog: [slack, stripe]
```

---

## Technical Considerations

### Performance Impact

**Current (8 patterns):**
- Initialization: <50ms
- Per-operation: <3μs (no secrets)
- With secrets: <16μs per operation

**With Gitleaks (120+ patterns):**
- Initialization: ~150-300ms (parse TOML + compile regex)
- Per-operation: ~50-100μs (more patterns to check)
- Still negligible for terminal output

**Optimization Strategies:**
- Lazy compilation (compile regex on first use)
- Pattern caching
- Category-based filtering (only load enabled categories)
- Parallel regex matching for large outputs

### Memory Impact

**Current:** ~100KB (8 compiled patterns)

**With Gitleaks:** ~2-5MB (120+ patterns + metadata)

**Mitigation:**
- Only load enabled categories
- Share compiled patterns across goroutines
- Use sync.Pool for temporary buffers

### Maintenance Burden

**Current (8 patterns):**
- Updates: Manual, infrequent
- Testing: Simple, fast
- Debugging: Easy to trace

**With Libraries:**
- Updates: Need to track upstream changes
- Testing: More complex (test each library)
- Debugging: Harder to identify which pattern matched
- Versioning: Need to handle library version conflicts

### Dependency Management

**Current:** Zero external dependencies

**With Libraries:**
- Need to vendor pattern files (embed in binary)
- Track upstream versions
- Handle breaking changes
- Provide update mechanism

**Options:**
1. **Embed at compile time** - Use `//go:embed` to include pattern files
2. **Download at runtime** - Fetch from GitHub releases (requires network)
3. **Ship separately** - Patterns as separate package (complicates install)

**Recommendation:** Embed at compile time for zero-dependency experience.

---

## Security Considerations

### False Negatives (Missing Secrets)

**Current Risk:** Low (8 patterns cover most common cases)

**With Libraries:** Lower (comprehensive coverage)

**Mitigation:**
- Regular pattern updates
- Community contributions
- User-defined custom patterns

### False Positives (Over-Masking)

**Current Risk:** Low (conservative patterns)

**With Libraries:** Medium-High (generic patterns like "password=...")

**Mitigation:**
- Disable generic categories by default
- Allow per-pattern disable
- Provide unmask list for known non-secrets

### Performance-Based DoS

**Risk:** Malicious input triggers catastrophic backtracking in regex

**Mitigation:**
- Use RE2 engine (linear time, no backtracking)
- Set timeout for pattern matching
- Limit input size per operation

### Pattern Disclosure

**Risk:** Exposing patterns reveals what we DON'T mask

**Mitigation:**
- Don't log matched patterns in production
- Careful with error messages
- Patterns embedded in binary (not external config)

---

## Migration Path

### From Current to Custom Patterns

**Step 1:** Add `patterns` section support
```yaml
settings:
  terminal:
    mask:
      enabled: true
      patterns:
        - id: custom-1
          regex: '...'
```

**Step 2:** Keep hardcoded patterns as defaults, allow override

**Step 3:** Deprecate hardcoded patterns in favor of default pattern library

### From Custom Patterns to Libraries

**Step 1:** Add library support alongside custom patterns
```yaml
settings:
  terminal:
    mask:
      gitleaks: true
      patterns:  # Still supported
        - id: custom-1
          regex: '...'
```

**Step 2:** Users can opt-in to libraries

**Step 3:** Eventually make libraries default, keep custom patterns for overrides

---

## Decision: Why Not Now?

**Primary Reason:**
**We support user-defined patterns, but haven't hooked up config loading yet.** Pattern libraries would be premature optimization when users can already define custom patterns via:
1. Public API: `io.RegisterPattern("ACME_[A-Z0-9]{32}")`
2. Configuration (schema defined, loading not implemented):
   ```yaml
   settings:
     terminal:
       mask:
         patterns: ['ACME_[A-Z0-9]{32}']
   ```

**Secondary Reasons:**
1. **8 hardcoded patterns cover most common cases** - AWS, GitHub, OpenAI, generic tokens
2. **Complexity burden** - Pattern libraries add significant complexity (parsing TOML, managing categories)
3. **Maintenance overhead** - Need to track upstream changes, handle breaking changes
4. **Performance impact** - 120+ patterns vs 8 (3-5x slower)
5. **Diminishing returns** - Most users need <20 patterns total
6. **Zero dependencies** - Keep Atmos self-contained

**Next Steps (Before Pattern Libraries):**
1. **Implement config loading** - Hook up `mask.patterns` and `mask.literals` to masker
2. **Document pattern API** - Show users how to register custom patterns
3. **Gather usage data** - See what patterns users actually need
4. **Evaluate need** - Only add pattern libraries if custom patterns aren't sufficient

**When to Reconsider Pattern Libraries:**
- Users consistently need 50+ custom patterns (indicates library would help)
- Security audit requires comprehensive coverage (e.g., SOC2, FedRAMP)
- Community demand (multiple feature requests)
- Custom patterns prove insufficient (too much maintenance burden on users)

---

## Appendix: Pattern Library Comparison

| Library | Patterns | Categories | Format | Maintenance | Stars |
|---------|----------|------------|--------|-------------|-------|
| **Gitleaks** | 120+ | 13 | TOML | Active | 16k+ |
| **TruffleHog** | 700+ | N/A | Go code | Active | 14k+ |
| **Secrets-Patterns-DB** | 150+ | 20+ | JSON | Moderate | 2k+ |
| **Atmos (current)** | 8 | N/A | Go code | Us | N/A |

**Verdict:** Gitleaks is best balance of comprehensiveness and simplicity if we integrate.

---

## References

- [Gitleaks GitHub](https://github.com/gitleaks/gitleaks)
- [TruffleHog GitHub](https://github.com/trufflesecurity/trufflehog)
- [Secrets-Patterns-DB GitHub](https://github.com/mazen160/secrets-patterns-db)
- Current implementation: `pkg/io/global.go`
- Configuration schema: `pkg/schema/atmos_configuration.go`
- Testing: `pkg/io/global_test.go`

---

## See Also

- [README.md](README.md) - Overview and current implementation
- [implementation.md](implementation.md) - Technical implementation details
