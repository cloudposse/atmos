# Flat Configuration Design for Secret Masking

**Status**: Draft - Not Implementing
**Created**: 2025-11-02

> **⚠️ NOTE:** This document is a draft design exploration. These flat configuration designs are **not** being implemented at this time. The document is kept for reference and future consideration.

## Problem with Deep Nesting

**Current recommended approach has 7 levels:**
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

**This is insane.** We need a flatter structure.

---

## Design Option A: Libraries as Top-Level List

**Concept:** Use a flat list with inline configuration.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Just a flat list of what to use
      use:
        - gitleaks                    # Use all Gitleaks patterns
        - gitleaks:aws                # Use only AWS category
        - gitleaks:github             # Use only GitHub category
        - trufflehog:slack            # Use TruffleHog Slack detector
        - pattern:company-key         # Use custom pattern by ID

      # Custom patterns (same level as use)
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'

      literals:
        - "my-secret"

      env_vars:
        - COMPANY_API_KEY
```

**Depth:** Max 4 levels (settings → terminal → mask → use)

**Pros:**
- ✅ FLAT! Only 4 levels deep
- ✅ Simple enable/disable: add or remove from list
- ✅ Intuitive: "I want Gitleaks AWS patterns" → `gitleaks:aws`
- ✅ Can mix libraries and custom patterns in one list
- ✅ No nested maps

**Cons:**
- ❌ Can't configure library-specific settings (like disabled_rules)
- ❌ String parsing required (`gitleaks:aws` needs to be split)
- ❌ Less discoverable - need to know library:category syntax

**Examples:**
```yaml
# Use all Gitleaks patterns
use:
  - gitleaks

# Use specific Gitleaks categories
use:
  - gitleaks:aws
  - gitleaks:github
  - gitleaks:gitlab

# Mix multiple libraries
use:
  - gitleaks:aws
  - trufflehog:slack
  - pattern:company-key

# Use Gitleaks but exclude generic
use:
  - gitleaks:aws
  - gitleaks:github
  # Just don't list gitleaks:generic
```

---

## Design Option B: Libraries as Flat Map (Boolean Only)

**Concept:** Dead simple - libraries are just on/off switches.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Super simple: library on/off
      gitleaks: true
      trufflehog: false
      secrets-patterns-db: false

      # Fine-grained control via separate keys
      gitleaks-categories:
        - aws
        - github
        # Omit 'generic' to disable it

      trufflehog-detectors:
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

**Depth:** Max 4 levels (settings → terminal → mask → gitleaks-categories)

**Pros:**
- ✅ FLAT! Only 4 levels
- ✅ Dead simple library enable: `gitleaks: true`
- ✅ No nested maps at all
- ✅ Easy to see what's enabled at a glance

**Cons:**
- ❌ Library-specific keys at same level as system keys (could be confusing)
- ❌ Key names have prefixes (`gitleaks-categories`, `trufflehog-detectors`)
- ❌ Doesn't scale well if library has many config options

**Examples:**
```yaml
# Default: use all Gitleaks patterns
gitleaks: true

# Selective Gitleaks categories
gitleaks: true
gitleaks-categories:
  - aws
  - github

# Multiple libraries
gitleaks: true
trufflehog: true
trufflehog-detectors:
  - slack
```

---

## Design Option C: Prefix-Based Flat Structure

**Concept:** Use prefixes to organize, but keep everything flat.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Library enable/disable (lib- prefix)
      lib-gitleaks: true
      lib-trufflehog: false
      lib-secrets-db: false

      # Gitleaks config (gitleaks- prefix)
      gitleaks-aws: true
      gitleaks-github: true
      gitleaks-gitlab: true
      gitleaks-generic: false

      # TruffleHog config (trufflehog- prefix)
      trufflehog-slack: true
      trufflehog-stripe: true

      # Custom patterns (no prefix needed)
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'

      # Other masking sources
      literals:
        - "my-secret"

      env_vars:
        - COMPANY_API_KEY
```

**Depth:** Max 4 levels (settings → terminal → mask → patterns)

**Pros:**
- ✅ COMPLETELY FLAT - everything at same level
- ✅ Easy to enable library: `lib-gitleaks: true`
- ✅ Easy to configure categories: `gitleaks-aws: true/false`
- ✅ Prefixes provide visual grouping
- ✅ Scalable - can add unlimited library config

**Cons:**
- ❌ Many top-level keys if using multiple libraries
- ❌ Less structured than nested approach
- ❌ Prefix convention needs to be consistent

**Examples:**
```yaml
# Simple: enable Gitleaks
lib-gitleaks: true

# Disable specific categories
lib-gitleaks: true
gitleaks-generic: false

# Multiple libraries
lib-gitleaks: true
gitleaks-aws: true
gitleaks-github: true

lib-trufflehog: true
trufflehog-slack: true
trufflehog-stripe: true
```

---

## Design Option D: Tag-Based Selection (RECOMMENDED)

**Concept:** Use tags/labels to select what you want. Libraries and categories become tags.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Select what to include (tags)
      include:
        - gitleaks              # All Gitleaks patterns
        - aws                   # AWS patterns from all libraries
        - github                # GitHub patterns from all libraries
        - slack                 # Slack patterns from all libraries
        - pattern:company-key   # Specific custom pattern

      # Or exclude specific things
      exclude:
        - generic               # Exclude generic patterns from all libraries

      # Custom patterns (tagged for selection)
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'
          tags: [company, api-key]

      literals:
        - "my-secret"

      env_vars:
        - COMPANY_API_KEY
```

**Depth:** Max 4 levels (settings → terminal → mask → include)

**Pros:**
- ✅ FLAT! Only 4 levels
- ✅ Powerful: select by library OR by category
- ✅ Works across libraries: `aws` gets AWS from Gitleaks + TruffleHog
- ✅ Intuitive: "I want AWS patterns" → add `aws` to include
- ✅ Can exclude specific categories

**Cons:**
- ❌ Need to establish tag taxonomy
- ❌ Potential conflicts if library name = category name
- ❌ Less explicit about which library provides what

**Examples:**
```yaml
# Use all Gitleaks
include:
  - gitleaks

# Use AWS and GitHub from any library
include:
  - aws
  - github

# Use Gitleaks but exclude generic
include:
  - gitleaks
exclude:
  - generic

# Mix libraries and custom
include:
  - gitleaks:aws        # Only AWS from Gitleaks
  - trufflehog:slack    # Only Slack from TruffleHog
  - pattern:company-key # Custom pattern
```

---

## Design Option E: Simplified Two-Level (RECOMMENDED)

**Concept:** Accept ONE level of library nesting, but keep library config flat via lists.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Gitleaks configuration (flat list)
      gitleaks:
        - aws
        - github
        - gitlab
        # Omit 'generic' to disable it

      # TruffleHog configuration (flat list)
      trufflehog:
        - slack
        - stripe

      # Custom patterns (flat list)
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'

      # Other sources (flat lists)
      literals:
        - "my-secret"

      env_vars:
        - COMPANY_API_KEY
```

**Depth:** Max 4 levels (settings → terminal → mask → gitleaks)

**Pros:**
- ✅ Only 4 levels deep
- ✅ Simple list structure - no nested maps
- ✅ Clear which library provides what
- ✅ Easy to enable all: `gitleaks: true` or specific: `gitleaks: [aws, github]`
- ✅ Each library configuration is just a list
- ✅ Uniform structure across all libraries

**Cons:**
- ❌ One level of nesting still exists (but minimal)
- ❌ Can't disable entire library easily without removing key

**Boolean shorthand:**
```yaml
# Enable all Gitleaks patterns
gitleaks: true

# Or be selective
gitleaks:
  - aws
  - github

# Disable Gitleaks
gitleaks: false
# Or just omit the key entirely
```

**Type handling:**
```go
type LibraryConfig interface{}  // Can be bool, string, or []string

// Parse:
// - true = enable all
// - false = disable all
// - "aws" = enable only aws category
// - ["aws", "github"] = enable only those categories
```

---

## Design Option F: Ultra-Flat String Arrays

**Concept:** Everything is just arrays of strings. Dead simple.

```yaml
settings:
  terminal:
    mask:
      enabled: true
      replacement: "***MASKED***"

      # Just list what you want (library:category format)
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

**Depth:** Only 3 levels! (settings → terminal → mask)

**Pros:**
- ✅ FLATTEST POSSIBLE - only 3 levels!!!
- ✅ One list to rule them all
- ✅ Super simple config
- ✅ Easy to add/remove

**Cons:**
- ❌ String parsing required everywhere
- ❌ Pattern definitions separate from usage
- ❌ Less structured
- ❌ Harder to validate

---

## Comparison Table

| Design | Max Depth | Library Enable | Category Control | Readability | Extensibility |
|--------|-----------|----------------|------------------|-------------|---------------|
| **A: Top-Level List** | 4 | `- gitleaks` | `- gitleaks:aws` | Good | Good |
| **B: Flat Map** | 4 | `gitleaks: true` | `gitleaks-categories: [aws]` | Good | Limited |
| **C: Prefix-Based** | 4 | `lib-gitleaks: true` | `gitleaks-aws: true` | OK | Excellent |
| **D: Tag-Based** | 4 | `include: [gitleaks]` | `include: [aws]` | Excellent | Excellent |
| **E: Two-Level** ⭐ | 4 | `gitleaks: true` | `gitleaks: [aws]` | Excellent | Good |
| **F: Ultra-Flat** | 3 | `- gitleaks:*` | `- gitleaks:aws` | Good | OK |

---

## Recommendation: Option E (Simplified Two-Level)

**Why:**
- ✅ Only 4 levels deep (acceptable)
- ✅ Dead simple: `gitleaks: [aws, github]`
- ✅ Clean structure - no key prefixes
- ✅ Easy to understand and document
- ✅ Supports multiple libraries naturally
- ✅ Uniform across all libraries

**Full Example:**
```yaml
settings:
  terminal:
    mask:
      enabled: true

      # Enable all Gitleaks patterns (shorthand)
      gitleaks: true

      # Or be selective with TruffleHog
      trufflehog:
        - slack
        - stripe

      # Or disable entirely
      secrets-patterns-db: false

      # Custom patterns
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'
        - id: employee-id
          regex: 'EMP-\d{6}'

      literals:
        - "my-secret"

      env_vars:
        - COMPANY_API_KEY
```

**Depth Breakdown:**
1. `settings`
2. `terminal`
3. `mask`
4. `gitleaks` (or `patterns`, `literals`, etc.)

**That's it - just 4 levels!**

---

## Alternative Recommendation: Option D (Tag-Based)

If you want even more flexibility with the same depth:

```yaml
settings:
  terminal:
    mask:
      enabled: true

      # Include what you want
      include:
        - gitleaks    # All Gitleaks
        - aws         # AWS from all libraries
        - github      # GitHub from all libraries

      # Exclude specific things
      exclude:
        - generic     # Exclude generic from all

      # Custom patterns
      patterns:
        - id: company-key
          regex: 'ACME_[A-Z0-9]{32}'
          tags: [company]
```

**Depth:** Also just 4 levels, but more powerful cross-library selection.

Which approach resonates more with you?
