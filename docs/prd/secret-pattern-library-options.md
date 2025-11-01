# PRD: Secret Pattern Library Integration Options

**Status**: Proposed
**Created**: 2025-11-01
**Owner**: Engineering Team

## Executive Summary

This document evaluates Go SDK options for integrating a comprehensive secret pattern library into Atmos's masking system. Currently, Atmos uses a small set of hardcoded regex patterns. By leveraging existing open-source pattern libraries, we can significantly improve secret detection coverage without maintaining our own pattern database.

## Current State

Atmos currently implements basic pattern matching in `pkg/io/global.go`:

```go
func registerCommonPatterns(masker Masker) {
    patterns := []string{
        `ghp_[A-Za-z0-9]{36}`,                        // GitHub PAT
        `gho_[A-Za-z0-9]{36}`,                        // GitHub OAuth
        `github_pat_[A-Za-z0-9]{22}_[A-Za-z0-9]{59}`, // New GitHub PAT format
        `glpat-[A-Za-z0-9\-_]{20}`,                   // GitLab PAT
        `sk-[A-Za-z0-9]{48}`,                         // OpenAI API key
        `Bearer [A-Za-z0-9\-._~+/]+=*`,               // Bearer tokens
        `AKIA[0-9A-Z]{16}`,                           // AWS Access Key ID
        `[A-Za-z0-9/+=]{40}`,                         // AWS Secret Access Key
    }
}
```

**Limitations:**
- Only 8 patterns
- Manually maintained
- No entropy analysis
- No verification capabilities
- Missing patterns for 100+ common services

## Requirements

1. **Comprehensive Coverage**: Support 100+ secret types (AWS, GitHub, GitLab, Datadog, Anthropic, etc.)
2. **Low Maintenance**: Leverage community-maintained pattern databases
3. **Go Native**: Pure Go implementation for easy integration
4. **Performant**: Minimal impact on masking performance
5. **Configurable**: Allow users to enable/disable specific patterns
6. **License Compatible**: Must be compatible with Atmos's Apache-2.0 license

## Option 1: TruffleHog v3 Detector Library

**Repository**: https://github.com/trufflesecurity/trufflehog
**Go Package**: `github.com/trufflesecurity/trufflehog/v3/pkg/detectors`
**License**: AGPL-3.0 ⚠️
**Patterns**: ~790 detectors

### Overview

TruffleHog v3 is a complete rewrite in Go with extensive detector support. It provides a structured `Detector` interface with verification capabilities and entropy analysis.

### Architecture

```go
type Detector interface {
    FromData(ctx context.Context, verify bool, data []byte) ([]Result, error)
    Keywords() []string
    Type() detectorspb.DetectorType
    Description() string
}
```

### Key Features

**✅ Pros:**
- Most comprehensive pattern library (790+ detectors)
- Active community and frequent updates
- Verification capabilities (can test if secrets are valid)
- Entropy analysis built-in (`StringShannonEntropy()`)
- False positive filtering (`FilterKnownFalsePositives()`)
- HTTP client utilities for verification
- Well-structured Go API
- 865 known importers (proven track record)

**❌ Cons:**
- **AGPL-3.0 license** - Incompatible with Apache-2.0 (requires derivative works to be AGPL)
- API stability warning: "Currently, trufflehog is in heavy development and no guarantees can be made on the stability of the public APIs"
- Heavy dependency (full scanning engine, not just patterns)
- Verification features require network access (security/privacy concerns)
- May be overkill for simple pattern matching

### Integration Approach

```go
import (
    "github.com/trufflesecurity/trufflehog/v3/pkg/detectors"
    "github.com/trufflesecurity/trufflehog/v3/pkg/detectors/aws"
    "github.com/trufflesecurity/trufflehog/v3/pkg/detectors/github"
)

func loadTruffleHogPatterns(masker Masker) {
    detectors := []detectors.Detector{
        aws.Scanner{},
        github.Scanner{},
        // ... 790+ more
    }

    for _, detector := range detectors {
        // Extract keywords and patterns
        for _, keyword := range detector.Keywords() {
            masker.RegisterPattern(keyword)
        }
    }
}
```

### Recommendation

**❌ NOT RECOMMENDED** due to AGPL-3.0 license incompatibility with Atmos's Apache-2.0 license.

---

## Option 2: Gitleaks Rule Library

**Repository**: https://github.com/gitleaks/gitleaks
**Go Package**: `github.com/zricethezav/gitleaks/v8`
**License**: MIT ✅
**Patterns**: ~120 rules

### Overview

Gitleaks is a lightweight, fast secret scanner written in Go. It uses a TOML configuration file format with regex patterns for detection.

### Architecture

```toml
[[rules]]
id = "github-pat"
description = "GitHub Personal Access Token"
regex = '''ghp_[0-9a-zA-Z]{36}'''
keywords = ["ghp_"]

[[rules]]
id = "aws-access-key"
description = "AWS Access Key"
regex = '''AKIA[0-9A-Z]{16}'''
keywords = ["AKIA"]
```

### Key Features

**✅ Pros:**
- MIT license - Compatible with Apache-2.0
- Lightweight and fast (built for performance)
- Simple TOML configuration format
- ~120 well-maintained rules
- Pure regex approach (no network calls)
- Entropy checks available
- Easy to parse configuration
- Can import rules directly

**❌ Cons:**
- Fewer patterns than TruffleHog (120 vs 790)
- No built-in verification
- Less active community than TruffleHog
- Rules in external TOML file (requires parsing)

### Integration Approach

**Option 2a: Embed Gitleaks Configuration**

```go
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
}

type GitleaksConfig struct {
    Rules []GitleaksRule `toml:"rules"`
}

func loadGitleaksPatterns(masker Masker) error {
    var config GitleaksConfig
    if err := toml.Unmarshal([]byte(gitleaksConfig), &config); err != nil {
        return err
    }

    for _, rule := range config.Rules {
        if err := masker.RegisterPattern(rule.Regex); err != nil {
            // Log warning but continue
        }
    }
    return nil
}
```

**Option 2b: Use Gitleaks as Library**

```go
import "github.com/zricethezav/gitleaks/v8/config"

func loadGitleaksPatterns(masker Masker) error {
    cfg, err := config.NewConfig("gitleaks.toml")
    if err != nil {
        return err
    }

    for _, rule := range cfg.Rules {
        masker.RegisterPattern(rule.Regex)
    }
    return nil
}
```

### Recommendation

**✅ RECOMMENDED** - Best balance of features, license compatibility, and maintainability.

---

## Option 3: Secrets Patterns DB

**Repository**: https://github.com/mazen160/secrets-patterns-db
**Format**: YAML database
**License**: CC-BY-SA-4.0 ✅
**Patterns**: 1600+ regex patterns

### Overview

Secrets Patterns DB is a comprehensive open-source database of regex patterns for secret detection. It's designed to be tool-agnostic and can be converted to TruffleHog, Gitleaks, or custom formats.

### Architecture

```yaml
# rules-stable.yml
rules:
  - id: aws-access-key
    description: AWS Access Key
    pattern: 'AKIA[0-9A-Z]{16}'
    confidence: high
    keywords:
      - AKIA

  - id: github-token
    description: GitHub Token
    pattern: 'ghp_[a-zA-Z0-9]{36}'
    confidence: high
    keywords:
      - ghp_
```

### Key Features

**✅ Pros:**
- Most comprehensive (1600+ patterns)
- CC-BY-SA-4.0 license - Compatible with Apache-2.0
- Format-agnostic (YAML source)
- Confidence levels for patterns
- ReDoS (Regular Expression Denial of Service) tested
- Conversion scripts for TruffleHog/Gitleaks formats
- Community-driven contributions

**❌ Cons:**
- No Go library (just data files)
- Requires custom parsing
- No verification capabilities
- Python-based conversion scripts
- Beta status
- Need to vendor YAML file

### Integration Approach

**Option 3a: Embed YAML and Parse**

```go
import (
    _ "embed"
    "gopkg.in/yaml.v3"
)

//go:embed rules-stable.yml
var secretsDB string

type SecretPattern struct {
    ID          string   `yaml:"id"`
    Description string   `yaml:"description"`
    Pattern     string   `yaml:"pattern"`
    Confidence  string   `yaml:"confidence"`
    Keywords    []string `yaml:"keywords"`
}

type SecretsDatabase struct {
    Rules []SecretPattern `yaml:"rules"`
}

func loadSecretsPatternsDB(masker Masker) error {
    var db SecretsDatabase
    if err := yaml.Unmarshal([]byte(secretsDB), &db); err != nil {
        return err
    }

    for _, rule := range db.Rules {
        // Only register high-confidence patterns to avoid noise
        if rule.Confidence == "high" {
            masker.RegisterPattern(rule.Pattern)
        }
    }
    return nil
}
```

**Option 3b: Convert to Gitleaks Format**

Use the provided conversion script to generate a Gitleaks TOML file, then embed it:

```bash
./scripts/convert-rules.py --db ./db/rules-stable.yml --type gitleaks --export gitleaks-rules.toml
```

Then use Option 2a to embed the generated TOML.

### Recommendation

**✅ RECOMMENDED (Alternative)** - Best for maximum coverage if willing to parse YAML.

---

## Option 4: Custom Minimal Library

**Approach**: Extract patterns from Secrets Patterns DB or Gitleaks, manually curate
**License**: Apache-2.0 (our own)
**Patterns**: ~50-100 (curated)

### Overview

Create a minimal, hand-curated set of patterns based on the most common secrets and highest-confidence detections from existing databases.

### Architecture

```go
// pkg/io/patterns/patterns.go
package patterns

type Pattern struct {
    ID          string
    Description string
    Regex       string
    Provider    string
}

var CommonPatterns = []Pattern{
    {
        ID:          "aws-access-key",
        Description: "AWS Access Key ID",
        Regex:       `AKIA[0-9A-Z]{16}`,
        Provider:    "AWS",
    },
    {
        ID:          "github-pat",
        Description: "GitHub Personal Access Token",
        Regex:       `ghp_[a-zA-Z0-9]{36}`,
        Provider:    "GitHub",
    },
    // ... 50-100 more
}
```

### Key Features

**✅ Pros:**
- Full control over patterns
- No external dependencies
- Apache-2.0 license (our own code)
- Optimized for Atmos use cases
- No parsing overhead
- Easy to test and maintain
- Clear provenance

**❌ Cons:**
- Requires manual curation
- Limited coverage compared to full databases
- Ongoing maintenance burden
- Slower to add new patterns
- Need to track upstream changes

### Integration Approach

```go
import "github.com/cloudposse/atmos/pkg/io/patterns"

func registerCuratedPatterns(masker Masker) {
    for _, pattern := range patterns.CommonPatterns {
        if err := masker.RegisterPattern(pattern.Regex); err != nil {
            log.Warnf("Failed to register pattern %s: %v", pattern.ID, err)
        }
    }
}
```

### Recommendation

**✅ RECOMMENDED (Conservative)** - Best for maintaining full control and minimal dependencies.

---

## Comparison Matrix

| Criteria | TruffleHog v3 | Gitleaks | Secrets Patterns DB | Custom |
|----------|---------------|----------|---------------------|--------|
| **Patterns** | 790+ | 120 | 1600+ | 50-100 |
| **License** | ❌ AGPL-3.0 | ✅ MIT | ✅ CC-BY-SA-4.0 | ✅ Apache-2.0 |
| **Go Native** | ✅ Yes | ✅ Yes | ❌ YAML only | ✅ Yes |
| **Verification** | ✅ Yes | ❌ No | ❌ No | ❌ No |
| **Maintenance** | ✅ High | ✅ Medium | ⚠️ Beta | ❌ Manual |
| **Dependencies** | ❌ Heavy | ✅ Light | ⚠️ Parse YAML | ✅ None |
| **Performance** | ⚠️ Medium | ✅ Fast | ⚠️ Parse overhead | ✅ Fast |
| **Integration Effort** | ⚠️ Medium | ✅ Easy | ⚠️ Medium | ✅ Easy |

---

## Recommendations

### Primary Recommendation: **Gitleaks (Option 2)**

**Why:**
1. ✅ MIT license - Compatible with Apache-2.0
2. ✅ Good balance of coverage (~120 patterns) vs complexity
3. ✅ Lightweight and performant
4. ✅ Active maintenance by Gitleaks community
5. ✅ Simple TOML format, easy to embed
6. ✅ Can be used as library or embedded config
7. ✅ No network calls (privacy/security)

**Implementation Strategy:**

1. **Phase 1**: Embed Gitleaks TOML configuration
   - Download latest `gitleaks.toml` from official repo
   - Embed as `//go:embed` in `pkg/io/patterns/gitleaks.toml`
   - Parse at initialization, register patterns with masker
   - **Effort**: 2-4 hours
   - **Risk**: Low

2. **Phase 2**: Configuration support
   - Add `settings.terminal.mask.patterns` in atmos.yaml
   - Allow enabling/disabling specific pattern categories
   - Support custom user patterns
   - **Effort**: 4-8 hours
   - **Risk**: Low

3. **Phase 3**: Performance optimization
   - Cache compiled regex patterns
   - Lazy load patterns on demand
   - Add pattern matching benchmarks
   - **Effort**: 4-6 hours
   - **Risk**: Medium

### Secondary Recommendation: **Secrets Patterns DB (Option 3)**

Use this if maximum coverage is critical and you're willing to:
- Parse YAML at runtime (or convert to Go at build time)
- Filter patterns by confidence level
- Maintain vendored YAML file

### Conservative Alternative: **Custom Library (Option 4)**

Use this if you prefer:
- Full control over patterns
- Minimal dependencies
- Clear code provenance
- Willingness to manually maintain patterns

---

## Configuration Design

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Pattern library configuration
      patterns:
        # Use built-in Gitleaks patterns (recommended)
        library: "gitleaks"  # Options: "gitleaks", "custom", "none"

        # Enable/disable specific categories
        categories:
          aws: true
          github: true
          gitlab: true
          slack: true
          datadog: true
          openai: true
          generic: true

        # Custom patterns (added to library patterns)
        custom:
          - id: "company-api-key"
            regex: 'COMPANY_[A-Z0-9]{32}'
            description: "Company API Key"

      # Literal values (always masked)
      literals:
        - "my-secret-value"

      # Environment variables to auto-mask
      env_vars:
        - AWS_SECRET_ACCESS_KEY
        - GITHUB_TOKEN
        - DATADOG_API_KEY
```

---

## Implementation Plan

### Milestone 1: Basic Integration (Week 1)

1. Download latest Gitleaks configuration
2. Create `pkg/io/patterns/` package
3. Embed Gitleaks TOML with `//go:embed`
4. Add TOML parsing logic
5. Register patterns with masker at initialization
6. Write tests for pattern loading
7. Update documentation

**Deliverables:**
- `pkg/io/patterns/patterns.go`
- `pkg/io/patterns/gitleaks.toml` (embedded)
- `pkg/io/patterns/patterns_test.go`
- Updated `docs/io-and-ui-output.md`

### Milestone 2: Configuration Support (Week 2)

1. Add configuration schema to `pkg/schema/schema_terminal.go`
2. Implement category filtering
3. Support custom user patterns
4. Add validation for user regex patterns
5. Write integration tests
6. Update PRD and developer guide

**Deliverables:**
- Configuration schema changes
- Category filtering logic
- Custom pattern support
- Integration tests
- Updated documentation

### Milestone 3: Optimization (Week 3)

1. Add regex compilation caching
2. Benchmark pattern matching performance
3. Optimize for common cases
4. Add performance tests
5. Document performance characteristics

**Deliverables:**
- Performance optimizations
- Benchmark suite
- Performance documentation

---

## Success Criteria

1. ✅ Support 100+ secret patterns (Gitleaks provides 120)
2. ✅ License compatible (MIT is compatible with Apache-2.0)
3. ✅ Minimal performance impact (<10ms overhead per mask operation)
4. ✅ Configurable by users
5. ✅ No network calls (privacy/security)
6. ✅ Well-documented and tested
7. ✅ Easy to update patterns (just update embedded TOML)

---

## Alternative: Future Enhancement

**If we later need TruffleHog's verification capabilities**, we can:

1. Keep Gitleaks for pattern matching (MIT license)
2. Add optional TruffleHog integration as a **plugin**
3. Make it opt-in via configuration
4. Clearly document AGPL-3.0 implications
5. Ship TruffleHog as separate binary/plugin

This approach maintains Apache-2.0 license for core Atmos while allowing power users to optionally use TruffleHog verification.

---

## References

- [Gitleaks Repository](https://github.com/gitleaks/gitleaks)
- [Gitleaks Configuration](https://github.com/gitleaks/gitleaks/blob/master/config/gitleaks.toml)
- [Secrets Patterns DB](https://github.com/mazen160/secrets-patterns-db)
- [TruffleHog v3](https://github.com/trufflesecurity/trufflehog)
- [Secret Scanner Comparison](https://www.jit.io/resources/appsec-tools/trufflehog-vs-gitleaks-a-detailed-comparison-of-secret-scanning-tools)
