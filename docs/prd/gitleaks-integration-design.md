# PRD: Gitleaks Integration for Secret Masking

**Status**: Proposed
**Created**: 2025-11-01
**Owner**: Engineering Team

## Executive Summary

This document proposes integrating Gitleaks pattern library (~120 regex patterns) into Atmos's existing masking system. The integration will enhance secret detection coverage from 8 patterns to 120+ while maintaining configurability and performance.

## Problem Statement

Currently, Atmos has only 8 hardcoded patterns in `pkg/io/global.go`. We need comprehensive secret detection without:
- Manually maintaining hundreds of patterns
- Breaking existing functionality
- Degrading performance
- Losing configurability

## Solution: Gitleaks Pattern Library Integration

### Why Gitleaks Works for Masking

**YES - Gitleaks is perfect for our masking use case:**

1. **Pattern-Based Detection**: Gitleaks uses regex patterns to detect secrets - exactly what our masker needs
2. **Comprehensive Coverage**: 120+ patterns for AWS, GitHub, GitLab, Slack, Datadog, OpenAI, etc.
3. **License Compatible**: MIT license (compatible with Apache-2.0)
4. **No Network Calls**: Pure regex matching (no verification attempts)
5. **Simple Format**: TOML configuration file easy to parse and embed
6. **Community Maintained**: Active project with regular pattern updates

**How It Maps to Our Masker:**

```go
// Gitleaks TOML
[[rules]]
id = "aws-access-key"
description = "AWS Access Key"
regex = '''AKIA[0-9A-Z]{16}'''
keywords = ["AKIA"]

// Maps directly to our masker
masker.RegisterPattern(`AKIA[0-9A-Z]{16}`)
```

### Enable/Disable Configuration

Gitleaks can be enabled/disabled at three levels:

#### 1. Global Enable/Disable

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      enabled: true              # Master switch for all masking

      patterns:
        library: "gitleaks"      # Options: "gitleaks", "builtin", "none"
```

**Behavior:**
- `library: "gitleaks"` - Load all 120+ Gitleaks patterns
- `library: "builtin"` - Only use 8 hardcoded patterns
- `library: "none"` - No pattern library, only env vars and explicit registrations

#### 2. Category-Level Control

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      enabled: true

      patterns:
        library: "gitleaks"

        # Enable/disable specific secret categories
        categories:
          aws: true              # AWS access keys, session tokens
          github: true           # GitHub PATs, OAuth tokens
          gitlab: true           # GitLab PATs
          slack: true            # Slack tokens
          datadog: true          # Datadog API keys
          openai: true           # OpenAI API keys
          anthropic: true        # Anthropic API keys
          google: true           # Google API keys, GCP tokens
          azure: true            # Azure connection strings
          generic: true          # Generic high-entropy strings
          all: true              # Enable all categories (default)
```

**Implementation:**
- Gitleaks rules have `id` field (e.g., "aws-access-key", "github-pat")
- We map rule IDs to categories
- Skip rules from disabled categories during initialization

#### 3. Rule-Level Control (Advanced)

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"

        # Disable specific rules by ID
        disabled_rules:
          - "generic-api-key"           # Too many false positives
          - "slack-webhook-url"         # We don't use Slack

        # Enable only specific rules (whitelist mode)
        enabled_rules:
          - "aws-access-key"
          - "github-pat"
          - "datadog-api-key"
```

**Behavior:**
- If `enabled_rules` is set: ONLY load those rules (whitelist)
- If `disabled_rules` is set: Load all EXCEPT those rules (blacklist)
- Cannot use both at the same time (config validation error)

#### 4. CLI Flag Override

```bash
# Completely disable masking for this command
atmos terraform plan --mask=false

# Enable masking (respects atmos.yaml patterns config)
atmos terraform plan --mask=true
```

### Configuration Schema

```yaml
# atmos.yaml (complete example)
settings:
  terminal:
    mask:
      # Global masking control
      enabled: true                         # Default: true
      replacement: "***MASKED***"           # What to replace secrets with

      # Pattern library configuration
      patterns:
        library: "gitleaks"                 # Options: "gitleaks", "builtin", "none"

        # Category controls (when library="gitleaks")
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
          generic: true

        # Rule-level controls (advanced)
        disabled_rules: []                  # Blacklist specific rule IDs
        # OR
        # enabled_rules: []                 # Whitelist specific rule IDs (mutually exclusive)

        # Custom patterns (added to library patterns)
        custom:
          - id: "company-internal-key"
            description: "Company Internal API Key"
            regex: 'ACME_[A-Z0-9]{32}'

      # Literal values to always mask
      literals:
        - "my-hardcoded-secret"

      # Environment variables to auto-mask values
      env_vars:
        - AWS_SECRET_ACCESS_KEY
        - GITHUB_TOKEN
        - DATADOG_API_KEY
        - ANTHROPIC_API_KEY
```

### Runtime Behavior

```go
// Initialization flow
func Initialize() error {
    // 1. Check global flag
    if !viper.GetBool("mask") {
        // Masking completely disabled - use passthrough writers
        Data = os.Stdout
        UI = os.Stderr
        return nil
    }

    // 2. Create context with masking
    ctx, err := NewContext()
    if err != nil {
        return err
    }

    // 3. Load pattern library based on config
    switch atmosConfig.Settings.Terminal.Mask.Patterns.Library {
    case "gitleaks":
        loadGitleaksPatterns(ctx.Masker(), atmosConfig)
    case "builtin":
        registerCommonPatterns(ctx.Masker())
    case "none":
        // No patterns, only env vars
    }

    // 4. Register env vars
    registerCommonSecrets(ctx.Masker())

    // 5. Register custom patterns
    registerCustomPatterns(ctx.Masker(), atmosConfig)

    // 6. Register literal values
    registerLiteralValues(ctx.Masker(), atmosConfig)

    Data = ctx.Streams().Output()
    UI = ctx.Streams().Error()
    return nil
}
```

### Category Mapping

```go
// pkg/io/patterns/categories.go
package patterns

// CategoryMap maps Gitleaks rule IDs to categories.
var CategoryMap = map[string]string{
    // AWS
    "aws-access-key":           "aws",
    "aws-secret-key":           "aws",
    "aws-session-token":        "aws",
    "aws-mws-key":              "aws",

    // GitHub
    "github-pat":               "github",
    "github-oauth":             "github",
    "github-app-token":         "github",
    "github-refresh-token":     "github",

    // GitLab
    "gitlab-pat":               "gitlab",
    "gitlab-pipeline-token":    "gitlab",
    "gitlab-runner-token":      "gitlab",

    // Slack
    "slack-access-token":       "slack",
    "slack-webhook-url":        "slack",

    // Datadog
    "datadog-api-key":          "datadog",
    "datadog-app-key":          "datadog",

    // OpenAI
    "openai-api-key":           "openai",

    // Anthropic
    "anthropic-api-key":        "anthropic",

    // Google/GCP
    "gcp-api-key":              "google",
    "google-oauth":             "google",
    "gcp-service-account":      "google",

    // Azure
    "azure-connection-string":  "azure",
    "azure-storage-key":        "azure",

    // Generic
    "generic-api-key":          "generic",
    "private-key":              "generic",
    "jwt":                      "generic",
}

// GetCategory returns the category for a rule ID.
func GetCategory(ruleID string) string {
    if category, ok := CategoryMap[ruleID]; ok {
        return category
    }
    return "generic"
}

// IsCategoryEnabled checks if a category is enabled in config.
func IsCategoryEnabled(category string, config *schema.AtmosConfiguration) bool {
    categories := config.Settings.Terminal.Mask.Patterns.Categories

    // If categories.all is set, use that
    if all, ok := categories["all"]; ok {
        return all
    }

    // Otherwise check specific category
    if enabled, ok := categories[category]; ok {
        return enabled
    }

    // Default: enabled
    return true
}
```

### Pattern Loading Logic

```go
// pkg/io/patterns/gitleaks.go
package patterns

import (
    _ "embed"
    "github.com/BurntSushi/toml"
)

//go:embed gitleaks.toml
var gitleaksConfig string

type GitleaksRule struct {
    ID          string   `toml:"id"`
    Description string   `toml:"description"`
    Regex       string   `toml:"regex"`
    Keywords    []string `toml:"keywords"`
    SecretGroup int      `toml:"secretGroup"`
    Entropy     float64  `toml:"entropy"`
}

type GitleaksConfig struct {
    Title       string         `toml:"title"`
    Rules       []GitleaksRule `toml:"rules"`
}

// LoadGitleaksPatterns loads Gitleaks patterns based on configuration.
func LoadGitleaksPatterns(masker Masker, config *schema.AtmosConfiguration) error {
    defer perf.Track(nil, "patterns.LoadGitleaksPatterns")()

    var gitleaks GitleaksConfig
    if err := toml.Unmarshal([]byte(gitleaksConfig), &gitleaks); err != nil {
        return fmt.Errorf("failed to parse Gitleaks config: %w", err)
    }

    maskConfig := config.Settings.Terminal.Mask.Patterns

    // Determine which rules to load
    var rulesToLoad []GitleaksRule

    if len(maskConfig.EnabledRules) > 0 {
        // Whitelist mode: only load enabled rules
        rulesToLoad = filterByWhitelist(gitleaks.Rules, maskConfig.EnabledRules)
    } else if len(maskConfig.DisabledRules) > 0 {
        // Blacklist mode: load all except disabled rules
        rulesToLoad = filterByBlacklist(gitleaks.Rules, maskConfig.DisabledRules)
    } else {
        // Category mode: load by enabled categories
        rulesToLoad = filterByCategories(gitleaks.Rules, config)
    }

    // Register patterns with masker
    for _, rule := range rulesToLoad {
        if err := masker.RegisterPattern(rule.Regex); err != nil {
            // Log warning but continue
            log.Warnf("Failed to register pattern %s: %v", rule.ID, err)
        }
    }

    return nil
}

func filterByWhitelist(rules []GitleaksRule, whitelist []string) []GitleaksRule {
    whitelistMap := make(map[string]bool, len(whitelist))
    for _, id := range whitelist {
        whitelistMap[id] = true
    }

    var result []GitleaksRule
    for _, rule := range rules {
        if whitelistMap[rule.ID] {
            result = append(result, rule)
        }
    }
    return result
}

func filterByBlacklist(rules []GitleaksRule, blacklist []string) []GitleaksRule {
    blacklistMap := make(map[string]bool, len(blacklist))
    for _, id := range blacklist {
        blacklistMap[id] = true
    }

    var result []GitleaksRule
    for _, rule := range rules {
        if !blacklistMap[rule.ID] {
            result = append(result, rule)
        }
    }
    return result
}

func filterByCategories(rules []GitleaksRule, config *schema.AtmosConfiguration) []GitleaksRule {
    var result []GitleaksRule
    for _, rule := range rules {
        category := GetCategory(rule.ID)
        if IsCategoryEnabled(category, config) {
            result = append(result, rule)
        }
    }
    return result
}
```

## Performance Considerations

### Lazy Pattern Compilation

```go
// pkg/io/masker.go (enhancement)
type compiledPattern struct {
    regex   *regexp.Regexp
    pattern string
}

type maskerImpl struct {
    mu               sync.RWMutex
    patterns         []string           // Uncompiled patterns
    compiledPatterns []compiledPattern  // Lazily compiled
    compiled         bool               // Whether compilation has happened
}

func (m *maskerImpl) Mask(data []byte) []byte {
    m.mu.Lock()
    if !m.compiled {
        m.compilePatterns()
        m.compiled = true
    }
    m.mu.Unlock()

    // Now use compiled patterns
    // ...
}
```

**Benefits:**
- Startup time not impacted by 120+ regex compilations
- Patterns only compiled on first Mask() call
- One-time compilation cost amortized across all masking operations

### Benchmark Targets

| Metric | Target | Notes |
|--------|--------|-------|
| Initialization | <50ms | Loading + parsing Gitleaks TOML |
| First Mask() | <100ms | Lazy compilation of 120 patterns |
| Subsequent Mask() | <1ms per KB | Using compiled patterns |
| Memory Overhead | <10MB | Compiled regex cache |

## Migration Path

### Phase 1: Minimal Integration (Week 1)

**Goal**: Get Gitleaks patterns loaded and working

1. Download `gitleaks.toml` from official repo
2. Create `pkg/io/patterns/` package
3. Embed Gitleaks TOML with `//go:embed`
4. Add TOML parsing with `github.com/BurntSushi/toml`
5. Register all patterns (no filtering yet)
6. Test with existing masker

**Config:**
```yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"  # New option
```

### Phase 2: Category Filtering (Week 2)

**Goal**: Add category-level control

1. Create category mapping for all Gitleaks rules
2. Implement category filtering logic
3. Update schema to support categories config
4. Add validation
5. Write tests

**Config:**
```yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"
        categories:
          aws: true
          github: true
          generic: false  # Disable generic patterns
```

### Phase 3: Advanced Controls (Week 3)

**Goal**: Rule-level control and performance

1. Implement whitelist/blacklist filtering
2. Add custom pattern support
3. Implement lazy compilation
4. Add performance benchmarks
5. Update documentation

**Config:**
```yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"
        disabled_rules:
          - "generic-api-key"
        custom:
          - id: "company-key"
            regex: 'ACME_[A-Z0-9]{32}'
```

## User Experience

### Default Behavior

**Out of the box** (no atmos.yaml changes):
- ✅ All masking enabled (120+ patterns)
- ✅ All categories enabled
- ✅ Common env vars masked (AWS, GitHub, etc.)
- ✅ Zero configuration required

**User needs to disable generic patterns** (too many false positives):
```yaml
settings:
  terminal:
    mask:
      patterns:
        categories:
          generic: false
```

**User wants ONLY AWS masking**:
```yaml
settings:
  terminal:
    mask:
      patterns:
        enabled_rules:
          - "aws-access-key"
          - "aws-secret-key"
          - "aws-session-token"
```

**User wants to disable masking for debugging**:
```bash
atmos terraform plan --mask=false
```

## Testing Strategy

### Unit Tests

```go
func TestLoadGitleaksPatterns(t *testing.T) {
    tests := []struct {
        name           string
        config         *schema.AtmosConfiguration
        expectedCount  int
        expectedRules  []string
    }{
        {
            name: "all categories enabled",
            config: &schema.AtmosConfiguration{
                Settings: schema.Settings{
                    Terminal: schema.Terminal{
                        Mask: schema.Mask{
                            Patterns: schema.MaskPatterns{
                                Library: "gitleaks",
                                Categories: map[string]bool{"all": true},
                            },
                        },
                    },
                },
            },
            expectedCount: 120, // All Gitleaks patterns
        },
        {
            name: "only AWS category",
            config: &schema.AtmosConfiguration{
                Settings: schema.Settings{
                    Terminal: schema.Terminal{
                        Mask: schema.Mask{
                            Patterns: schema.MaskPatterns{
                                Library: "gitleaks",
                                Categories: map[string]bool{
                                    "aws": true,
                                    "github": false,
                                },
                            },
                        },
                    },
                },
            },
            expectedCount: 5, // Only AWS patterns
            expectedRules: []string{"aws-access-key", "aws-secret-key"},
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            masker := NewMasker()
            err := LoadGitleaksPatterns(masker, tt.config)
            require.NoError(t, err)

            // Verify pattern count
            assert.Equal(t, tt.expectedCount, masker.PatternCount())

            // Verify specific rules loaded
            for _, ruleID := range tt.expectedRules {
                assert.True(t, masker.HasPattern(ruleID))
            }
        })
    }
}
```

### Integration Tests

```go
func TestGitleaksMasking(t *testing.T) {
    tests := []struct {
        name     string
        input    string
        masked   string
    }{
        {
            name:   "AWS access key",
            input:  "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
            masked: "AWS_ACCESS_KEY_ID=***MASKED***",
        },
        {
            name:   "GitHub PAT",
            input:  "GITHUB_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz",
            masked: "GITHUB_TOKEN=***MASKED***",
        },
        {
            name:   "Multiple secrets",
            input:  "AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE GITHUB_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz",
            masked: "AWS_ACCESS_KEY_ID=***MASKED*** GITHUB_TOKEN=***MASKED***",
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            ctx, err := NewContext()
            require.NoError(t, err)

            config := &schema.AtmosConfiguration{
                Settings: schema.Settings{
                    Terminal: schema.Terminal{
                        Mask: schema.Mask{
                            Patterns: schema.MaskPatterns{
                                Library: "gitleaks",
                            },
                        },
                    },
                },
            }

            LoadGitleaksPatterns(ctx.Masker(), config)

            output := ctx.Masker().Mask([]byte(tt.input))
            assert.Equal(t, tt.masked, string(output))
        })
    }
}
```

### Benchmark Tests

```go
func BenchmarkGitleaksPatterns(b *testing.B) {
    ctx, _ := NewContext()
    config := &schema.AtmosConfiguration{
        Settings: schema.Settings{
            Terminal: schema.Terminal{
                Mask: schema.Mask{
                    Patterns: schema.MaskPatterns{
                        Library: "gitleaks",
                    },
                },
            },
        },
    }

    LoadGitleaksPatterns(ctx.Masker(), config)

    input := []byte("AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE and GITHUB_TOKEN=ghp_1234567890abcdefghijklmnopqrstuvwxyz")

    b.ResetTimer()
    for i := 0; i < b.N; i++ {
        ctx.Masker().Mask(input)
    }
}
```

## Documentation Updates

### 1. CLAUDE.md

Add section under "I/O and UI Usage":

```markdown
### Secret Masking with Gitleaks

Atmos uses Gitleaks pattern library (120+ patterns) for comprehensive secret detection:

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      patterns:
        library: "gitleaks"  # Use Gitleaks patterns (default)
        categories:
          aws: true          # Enable AWS secret detection
          github: true       # Enable GitHub token detection
```

Disable specific categories to reduce false positives:
```yaml
settings:
  terminal:
    mask:
      patterns:
        categories:
          generic: false  # Disable generic patterns
```

Disable masking for debugging:
```bash
atmos terraform plan --mask=false
```
```

### 2. docs/io-and-ui-output.md

Add new section:

```markdown
## Secret Pattern Configuration

Atmos integrates Gitleaks pattern library for comprehensive secret detection. Configure in atmos.yaml:

[Full configuration examples...]
```

### 3. website/docs/cli/configuration/mask.md

New page documenting all masking configuration options.

## Success Criteria

1. ✅ Support 120+ secret patterns from Gitleaks
2. ✅ Configurable at global, category, and rule levels
3. ✅ CLI flag override (`--mask=false`)
4. ✅ Lazy pattern compilation for performance
5. ✅ <100ms first mask operation
6. ✅ <1ms per KB subsequent operations
7. ✅ 90%+ test coverage
8. ✅ Zero breaking changes to existing API

## Open Questions

1. **Should we vendor the Gitleaks TOML or fetch from GitHub on build?**
   - **Recommendation**: Vendor it (commit to repo) for build reproducibility
   - Update periodically via script or manual PR

2. **Should generic patterns be enabled by default?**
   - **Recommendation**: Yes by default, document how to disable if too many false positives
   - Let users opt-out rather than opt-in

3. **Should we support entropy-based detection?**
   - **Recommendation**: Phase 2 enhancement (after basic integration works)
   - Gitleaks has entropy thresholds we could leverage

## References

- [Gitleaks Repository](https://github.com/gitleaks/gitleaks)
- [Gitleaks Configuration](https://github.com/gitleaks/gitleaks/blob/master/config/gitleaks.toml)
- [PRD: Secret Pattern Library Options](./secret-pattern-library-options.md)
- [PRD: I/O Handling Strategy](./io-handling-strategy.md)
