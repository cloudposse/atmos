# Configuration Design Options for Secret Masking

**Status**: Draft - Not Implementing
**Created**: 2025-11-02

> **⚠️ NOTE:** This document is a draft design exploration. These configuration designs are **not** being implemented at this time. The document is kept for reference and future consideration.

This document explores alternative configuration designs for the masking system that support multiple pattern libraries while maintaining a clean, intuitive structure.

## Current Design Issues

**Problem with nested `patterns.library` approach:**
```yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"  # Only supports ONE library
        custom:              # Creates deep nesting
          - id: "my-pattern"
            regex: '...'
```

**Issues:**
- Only supports one library at a time (can't mix Gitleaks + TruffleHog)
- `custom` nested under `patterns` feels awkward
- No clear path to support multiple libraries
- Library configuration tied to pattern configuration

---

## Design Option 1: Libraries as Siblings to Patterns

**Concept:** Pattern libraries are siblings to custom patterns, not nested within them.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Custom patterns defined by user
      patterns:
        - id: "company-api-key"
          description: "Company API Key"
          regex: 'ACME_[A-Z0-9]{32}'
        - id: "employee-id"
          regex: 'EMP-\d{6}'

      # Pattern libraries (can enable multiple)
      libraries:
        gitleaks:
          enabled: true
          categories:
            aws: true
            github: true
            generic: false
        trufflehog:
          enabled: false
          detectors:
            - aws
            - github
        secrets-patterns-db:
          enabled: false

      # Literal values to mask
      literals:
        - "my-hardcoded-secret"

      # Environment variables to auto-mask
      env_vars:
        - AWS_SECRET_ACCESS_KEY
        - GITHUB_TOKEN
```

**Pros:**
- ✅ Clear separation: libraries vs. custom patterns
- ✅ Can enable multiple libraries simultaneously
- ✅ No deep nesting - everything at same level
- ✅ Each library can have its own configuration structure
- ✅ Easy to add new libraries without breaking existing config

**Cons:**
- ❌ More top-level keys under `mask`
- ❌ Library-specific config (categories vs detectors) varies

**Example - Using Multiple Libraries:**
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
        trufflehog:
          enabled: true
          detectors:
            - slack
            - stripe
```

---

## Design Option 2: Flat Sources List

**Concept:** All pattern sources (libraries, custom, builtin) are equal items in a list.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      sources:
        # Pattern library
        - type: library
          name: gitleaks
          categories:
            aws: true
            github: true
            generic: false

        # Another pattern library
        - type: library
          name: trufflehog
          detectors:
            - slack
            - stripe

        # Custom patterns
        - type: pattern
          id: "company-api-key"
          regex: 'ACME_[A-Z0-9]{32}'

        # Literal value
        - type: literal
          value: "my-hardcoded-secret"

        # Environment variable
        - type: env
          variable: COMPANY_API_KEY
```

**Pros:**
- ✅ Extremely flexible - everything is a source
- ✅ Easy to reorder sources (precedence)
- ✅ Uniform structure across all source types
- ✅ Can mix and match freely

**Cons:**
- ❌ Verbose - lots of `type:` fields
- ❌ Less intuitive - have to understand "source" concept
- ❌ Harder to find specific config (everything in one list)
- ❌ Difficult to provide defaults for each source type

---

## Design Option 3: Registry Pattern

**Concept:** Pattern libraries register themselves, config just enables/disables them.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Simple enable/disable for known libraries
      use_libraries:
        - gitleaks
        - trufflehog

      # Per-library configuration
      gitleaks:
        categories:
          aws: true
          github: true
          generic: false

      trufflehog:
        detectors:
          - slack
          - stripe

      # Custom patterns (not a library)
      patterns:
        - id: "company-api-key"
          regex: 'ACME_[A-Z0-9]{32}'

      # Literal values
      literals:
        - "my-hardcoded-secret"

      # Environment variables
      env_vars:
        - COMPANY_API_KEY
```

**Pros:**
- ✅ Simple enable/disable with `use_libraries`
- ✅ Each library gets its own top-level config section
- ✅ No nesting confusion
- ✅ Easy to understand and document

**Cons:**
- ❌ Library config at same level as `patterns` (could be confusing)
- ❌ Need to know which keys are libraries vs system keys
- ❌ More top-level keys

---

## Design Option 4: Hierarchical with Library Collections

**Concept:** Group library configs under `libraries`, but keep patterns flat.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Custom patterns (user-defined)
      patterns:
        - id: "company-api-key"
          regex: 'ACME_[A-Z0-9]{32}'
        - id: "employee-id"
          regex: 'EMP-\d{6}'

      # Pattern libraries (third-party)
      libraries:
        - name: gitleaks
          enabled: true
          version: "8.x"  # Optional: lock to specific version
          config:
            categories:
              aws: true
              github: true
              generic: false

        - name: trufflehog
          enabled: true
          config:
            detectors:
              - slack
              - stripe

        - name: secrets-patterns-db
          enabled: false

      # Other masking sources
      literals:
        - "my-hardcoded-secret"

      env_vars:
        - AWS_SECRET_ACCESS_KEY
        - COMPANY_API_KEY
```

**Pros:**
- ✅ Libraries as list items (easier to add/remove)
- ✅ Can specify library version for reproducibility
- ✅ Clear distinction: `patterns` = yours, `libraries` = third-party
- ✅ Each library config nested under `config` key
- ✅ Can enable multiple libraries

**Cons:**
- ❌ Nested `config` key adds depth
- ❌ List format slightly more verbose than map

---

## Design Option 5: Hybrid - Maps for Libraries, List for Patterns (RECOMMENDED)

**Concept:** Best of both worlds - libraries as map (easy lookup), patterns as list (order doesn't matter).

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Pattern libraries (map for easy enable/disable)
      libraries:
        gitleaks:
          enabled: true
          categories:
            aws: true
            github: true
            gitlab: true
            slack: true
            datadog: true
            openai: true
            anthropic: true
            google: true
            azure: true
            generic: false

        trufflehog:
          enabled: false
          detectors:
            - aws
            - github
            - slack

        secrets-patterns-db:
          enabled: false
          categories:
            - passwords
            - api-keys

      # Custom patterns (list - user adds their own)
      patterns:
        - id: "company-api-key"
          description: "Company Internal API Key"
          regex: 'ACME_[A-Z0-9]{32}'
        - id: "employee-id"
          description: "Employee ID"
          regex: 'EMP-\d{6}'

      # Literal values to mask
      literals:
        - "my-hardcoded-secret"

      # Environment variables to auto-mask
      env_vars:
        - AWS_SECRET_ACCESS_KEY
        - GITHUB_TOKEN
        - DATADOG_API_KEY
        - COMPANY_API_KEY
```

**Pros:**
- ✅ Libraries as map = easy to find and toggle specific libraries
- ✅ Each library can have unique config structure (categories vs detectors)
- ✅ Can enable multiple libraries simultaneously
- ✅ Clear separation: `libraries` (third-party) vs `patterns` (custom)
- ✅ No deep nesting - only 2 levels max
- ✅ Intuitive: "I want Gitleaks" → set `libraries.gitleaks.enabled: true`
- ✅ Easy to add new libraries without config changes

**Cons:**
- ❌ Slightly more keys than current design
- ✅ (Minor) - Actually this helps organization!

**Comparison to Current:**
```yaml
# CURRENT (limiting)
patterns:
  library: "gitleaks"  # Can only use ONE
  custom: [...]        # Nested under patterns

# RECOMMENDED (flexible)
libraries:
  gitleaks: {enabled: true}
  trufflehog: {enabled: false}
patterns: [...]        # Not nested, equal level
```

---

## Usage Examples with Recommended Design

### Example 1: Default Configuration (Gitleaks Only)

```yaml
settings:
  terminal:
    mask:
      libraries:
        gitleaks:
          enabled: true
```

**Result:** All 120 Gitleaks patterns enabled, all categories active.

### Example 2: Reduce False Positives

```yaml
settings:
  terminal:
    mask:
      libraries:
        gitleaks:
          enabled: true
          categories:
            generic: false  # Disable generic patterns
```

### Example 3: Multiple Libraries

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
            generic: false

        trufflehog:
          enabled: true
          detectors:
            - slack
            - stripe
            - twilio
```

**Result:** AWS/GitHub patterns from Gitleaks + Slack/Stripe/Twilio from TruffleHog.

### Example 4: Custom Patterns Only

```yaml
settings:
  terminal:
    mask:
      libraries:
        gitleaks:
          enabled: false

      patterns:
        - id: "company-api-key"
          regex: 'ACME_[A-Z0-9]{32}'
        - id: "session-token"
          regex: 'SESSION_[a-f0-9]{64}'
```

**Result:** Only 2 custom patterns, no library patterns.

### Example 5: Everything Together

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      libraries:
        gitleaks:
          enabled: true
          categories:
            aws: true
            github: true

        trufflehog:
          enabled: true
          detectors:
            - slack

      patterns:
        - id: "company-api-key"
          regex: 'ACME_[A-Z0-9]{32}'

      literals:
        - "my-test-secret-abc123"

      env_vars:
        - COMPANY_API_KEY
        - INTERNAL_TOKEN
```

**Result:**
- AWS + GitHub patterns from Gitleaks
- Slack patterns from TruffleHog
- 1 custom pattern
- 1 literal value masked
- Values from 2 env vars masked

---

## Migration Path

### From Current Design to Recommended

**Current (limited):**
```yaml
patterns:
  library: "gitleaks"
  categories: {...}
  custom: [...]
```

**Recommended (flexible):**
```yaml
libraries:
  gitleaks:
    enabled: true
    categories: {...}
patterns: [...]
```

**Migration:**
1. Move `patterns.library: "gitleaks"` → `libraries.gitleaks.enabled: true`
2. Move `patterns.categories` → `libraries.gitleaks.categories`
3. Move `patterns.custom` → `patterns` (un-nest)
4. Support both formats during transition period
5. Deprecate old format after 2-3 releases

---

## Implementation Considerations

### Library Registry

```go
// pkg/io/patterns/registry.go
package patterns

type Library interface {
    Name() string
    Load(config map[string]interface{}) ([]Pattern, error)
}

var libraries = map[string]Library{
    "gitleaks":            &GitleaksLibrary{},
    "trufflehog":          &TruffleHogLibrary{},
    "secrets-patterns-db": &SecretsDBLibrary{},
}

func RegisterLibrary(name string, lib Library) {
    libraries[name] = lib
}

func LoadLibraries(config *schema.MaskConfig) ([]Pattern, error) {
    var allPatterns []Pattern

    for name, libConfig := range config.Libraries {
        if !libConfig.Enabled {
            continue
        }

        lib, exists := libraries[name]
        if !exists {
            return nil, fmt.Errorf("unknown pattern library: %s", name)
        }

        patterns, err := lib.Load(libConfig.Config)
        if err != nil {
            return nil, fmt.Errorf("failed to load %s: %w", name, err)
        }

        allPatterns = append(allPatterns, patterns...)
    }

    return allPatterns, nil
}
```

### Schema Updates

```go
// pkg/schema/atmos_configuration.go
type MaskConfig struct {
    Enabled     bool                       `yaml:"enabled" json:"enabled"`
    Replacement string                     `yaml:"replacement" json:"replacement"`
    Libraries   map[string]LibraryConfig   `yaml:"libraries" json:"libraries"`
    Patterns    []CustomPattern            `yaml:"patterns" json:"patterns"`
    Literals    []string                   `yaml:"literals" json:"literals"`
    EnvVars     []string                   `yaml:"env_vars" json:"env_vars"`
}

type LibraryConfig struct {
    Enabled bool                   `yaml:"enabled" json:"enabled"`
    Config  map[string]interface{} `yaml:",inline"`  // Library-specific config
}

type CustomPattern struct {
    ID          string `yaml:"id" json:"id"`
    Description string `yaml:"description" json:"description"`
    Regex       string `yaml:"regex" json:"regex"`
}
```

---

## Recommendation

**Use Design Option 5: Hybrid - Maps for Libraries, List for Patterns**

**Rationale:**
1. **Extensible:** Easy to add new libraries (TruffleHog, Secrets-DB, custom) without breaking changes
2. **Intuitive:** Clear mental model - libraries (third-party) vs patterns (yours)
3. **No Deep Nesting:** Max 2 levels (libraries.gitleaks.categories)
4. **Multiple Libraries:** Can enable Gitleaks + TruffleHog + others simultaneously
5. **Flexible:** Each library can have unique config structure
6. **Future-Proof:** Registry pattern allows plugin-style library additions

**Quick Win:**
Most users just do:
```yaml
libraries:
  gitleaks:
    enabled: true
```

**Power Users:**
Can mix multiple libraries, custom patterns, literals, env vars all together.

**Implementation Priority:**
1. Phase 1: Support `libraries.gitleaks` with categories (replaces current `patterns.library`)
2. Phase 2: Add `patterns` list for custom patterns (replaces `patterns.custom`)
3. Phase 3: Add TruffleHog support under `libraries.trufflehog`
4. Phase 4: Add plugin/extension system for custom libraries
