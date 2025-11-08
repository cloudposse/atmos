# Secrets Masking

**Status**: Implemented
**Created**: 2025-11-02
**Last Updated**: 2025-11-02

This directory contains documentation for Atmos's automatic secrets masking system.

## What We Built

Atmos automatically masks sensitive data (API keys, tokens, passwords) in all terminal output to prevent accidental exposure in logs, screenshots, or CI/CD pipelines.

### Architecture

The masking system operates at the I/O layer (`pkg/io/`) with two components:

1. **Global Writers** (`pkg/io/global.go`)
   - `io.Data` - stdout writer with automatic masking
   - `io.UI` - stderr writer with automatic masking
   - Third-party libraries can use these writers directly
   - All output is automatically masked before reaching the terminal

2. **Masking Engine** (`pkg/io/masker.go`)
   - Pattern-based detection (regex)
   - Literal value masking
   - Environment variable auto-detection
   - Format-aware (handles JSON, YAML, URL-encoded, base64, hex)

### Current Implementation

**8 Hardcoded Patterns:**
- GitHub Personal Access Tokens (classic: `ghp_`, new: `github_pat_`)
- GitHub OAuth tokens (`gho_`, `ghu_`, `ghs_`, `ghr_`)
- GitLab Personal Access Tokens (`glpat-`)
- OpenAI API keys (`sk-`)
- AWS Access Key ID (`AKIA`)
- AWS Secret Access Key (40-char base64)
- Bearer tokens

**Auto-Masking:**
- `AWS_ACCESS_KEY_ID`
- `AWS_SECRET_ACCESS_KEY`
- `AWS_SESSION_TOKEN`
- `GITHUB_TOKEN`
- `GITLAB_TOKEN`
- `DATADOG_API_KEY`
- `ANTHROPIC_API_KEY`

### Configuration

```yaml
# atmos.yaml
settings:
  terminal:
    mask:
      enabled: true                    # Default: true
      replacement: "***MASKED***"      # What to replace secrets with
```

**CLI Override:**
```bash
# Disable masking for debugging
atmos terraform plan --mask=false
```

### Usage

**For Command Developers:**

Just use the standard output functions - masking happens automatically:

```go
import (
    "github.com/cloudposse/atmos/pkg/io"
    "github.com/cloudposse/atmos/pkg/ui"
)

// Data output (stdout) - automatically masked
io.Data.Write("API_KEY=sk-abc123...")
// Output: API_KEY=***MASKED***

// UI output (stderr) - automatically masked
ui.Success("Configured with key sk-abc123...")
// Output: ✓ Configured with key ***MASKED***
```

**For Third-Party Libraries:**

Pass global writers directly to libraries that expect `io.Writer`:

```go
import iolib "github.com/cloudposse/atmos/pkg/io"

// Initialize masking
iolib.Initialize()

// Pass to logger (automatically masked)
logger := log.New(iolib.Data, "", 0)
logger.Println("API_KEY=sk-abc123...")  // Automatically masked

// Pass to progress bar
bar := progressbar.NewOptions(100,
    progressbar.OptionSetWriter(iolib.UI),  // Automatically masked
)

// Pass to custom file handle with masking
maskedFile := iolib.MaskWriter(fileHandle)
fmt.Fprintf(maskedFile, "secret: %s", apiKey)  // Automatically masked
```

**Register Custom Secrets:**

```go
import iolib "github.com/cloudposse/atmos/pkg/io"

// Register literal value (masks exact string + encodings)
iolib.RegisterSecret("my-api-key-abc123")

// Register pattern (regex)
iolib.RegisterPattern(`COMPANY_KEY_[A-Z0-9]{32}`)

// Register value from environment variable
iolib.RegisterValue(os.Getenv("CUSTOM_SECRET"))
```

### Files

- `implementation.md` - How the system works (architecture, implementation details)
- `future-considerations.md` - Pattern library integration options for later

## Key Decisions

### Why Not Pattern Libraries (Yet)?

**Decision:** Start with 8 hardcoded patterns instead of integrating Gitleaks/TruffleHog.

**Rationale:**
- Simpler implementation
- No external dependencies
- Faster startup (no pattern compilation overhead)
- 8 patterns cover 90% of use cases
- Can add pattern library later if needed

**Trade-offs:**
- ✅ Simple and fast
- ✅ No licensing concerns
- ✅ Easy to understand and debug
- ❌ Limited coverage (8 patterns vs 120+ in Gitleaks)
- ❌ Manual pattern maintenance

### Why Global Writers?

**Decision:** Provide package-level `io.Data` and `io.UI` writers.

**Rationale:**
- Third-party libraries need `io.Writer` interface
- Global variables = easy to pass to any library
- No need to thread context through call stack
- Matches logging pattern (familiar to developers)

**Trade-offs:**
- ✅ Simple integration with third-party libraries
- ✅ Automatic masking everywhere
- ✅ No context drilling
- ❌ Global state (but acceptable for I/O)

### Why Format-Aware Masking?

**Decision:** Mask secrets in JSON, YAML, URL-encoded, base64, and hex formats.

**Rationale:**
- Secrets appear in multiple encodings
- `{"key": "sk-abc"}` vs `key=sk-abc` vs `sk%2Dabc`
- Need to catch all variants to prevent leakage

**Implementation:**
- Detect literal value → mask it
- Detect URL-encoded variant → mask it
- Detect base64 variant → mask it
- Detect hex variant → mask it

## Documentation

- **User Documentation:** `website/docs/cli/configuration/mask.mdx`
- **Developer Guide:** `docs/io-and-ui-output.md`
- **Architecture PRD:** `docs/prd/io-handling-strategy.md`
- **Examples:** `pkg/io/example_test.go`

## Testing

- **Unit Tests:** `pkg/io/global_test.go` (17 tests, 80% coverage)
- **Integration Tests:** `pkg/io/context_test.go`
- **Snapshot Tests:** `tests/snapshots/*help*.stdout.golden` (42 snapshots)

## See Also

- [I/O Handling Strategy PRD](../io-handling-strategy.md)
- [I/O and UI Output Guide](../../io-and-ui-output.md)
- [Mask Configuration Docs](../../../website/docs/cli/configuration/mask.mdx)
