# Atmos Global `--ai` Flag - Product Requirements Document

**Status:** Implemented
**Version:** 2.0
**Last Updated:** 2026-03-10

---

## Executive Summary

The global `--ai` flag enables AI-powered analysis of command output across all Atmos CLI commands. When enabled, command output (stdout and stderr) is automatically captured, sent to the configured AI provider for analysis, and the AI response is rendered after command execution. For errors, the AI explains what went wrong and how to fix it. For successful output, it provides a concise summary.

## Motivation

Users frequently need to interpret complex command output (e.g., Terraform plan diffs, stack descriptions, validation results). By providing a global `--ai` flag, users can opt into AI-assisted analysis on any command without changing their workflow.

**Key Benefits:**
- Zero-friction AI integration — just add `--ai` to any command
- Works with ALL existing commands (terraform, helmfile, describe, validate, list, etc.)
- Error explanation with actionable fix instructions
- Environment variable support (`ATMOS_AI`) for CI/CD and scripting
- Follows standard Atmos flag precedence: CLI > ENV > config > defaults

## Requirements

### Functional Requirements

1. **Global Persistent Flag**: The `--ai` flag MUST be registered as a persistent flag on RootCmd, inherited by all subcommands.
2. **Boolean Flag**: The `--ai` flag MUST be a boolean flag (default: `false`).
3. **Environment Variable**: The flag MUST support the `ATMOS_AI` environment variable.
4. **Flag Precedence**: Standard Atmos precedence MUST apply: CLI flag > `ATMOS_AI` env var > config file > default (`false`).
5. **ParseGlobalFlags Integration**: The `AI` field MUST be parsed in `ParseGlobalFlags()` and available in `global.Flags.AI`.
6. **Output Capture**: When `--ai` is enabled, ALL command output (stdout and stderr) MUST be captured, including subprocess output (terraform, helmfile, packer).
7. **Tee Pattern**: Captured output MUST still be displayed to the user in real-time (tee to terminal and buffer).
8. **Error Analysis**: When a command fails with an error, the AI MUST explain the error and provide actionable steps to fix it.
9. **Success Analysis**: When a command succeeds, the AI MUST provide a concise summary of the output.
10. **AI Configuration Validation**: If `--ai` is used but AI is not configured in `atmos.yaml`, MUST show a helpful error with configuration hints.
11. **AI Command Exclusion**: The `--ai` flag MUST NOT trigger analysis for `atmos ai` commands (to avoid double AI processing).

### Non-Functional Requirements

1. **Zero Overhead When Disabled**: When `--ai` is `false` (default), there MUST be no performance impact (no pipes, no buffers).
2. **Backward Compatible**: Adding the flag MUST NOT break existing commands or workflows.
3. **Cross-Platform**: Output capture using `os.Pipe()` works on Linux, macOS, and Windows.
4. **Output Truncation**: Large outputs MUST be truncated to prevent exceeding AI token limits (50KB max).
5. **Timeout**: AI analysis requests MUST have a configurable timeout (default: 120s).
6. **Documentation**: The flag MUST be documented in global-flags.mdx with examples and environment variable reference.

## Architecture

### Component Overview

```
┌─────────────────────────────────────────────────────────────┐
│  cmd/root.go Execute()                                      │
│  ┌─────────────────────────────────────────────────────┐    │
│  │ 1. Parse --ai from os.Args (before Cobra parses)   │    │
│  │ 2. Validate AI config (fail fast with hints)        │    │
│  │ 3. Start output capture (os.Pipe + tee goroutines)  │    │
│  │ 4. Run command (internal.Execute)                   │    │
│  │ 5. Stop capture, send output to AI provider         │    │
│  │ 6. Render AI analysis as markdown                   │    │
│  └─────────────────────────────────────────────────────┘    │
│                                                             │
│  pkg/ai/analyze/                                            │
│  ├── capture.go    - CaptureSession (os.Pipe + tee)        │
│  ├── analyze.go    - ValidateAIConfig, AnalyzeOutput        │
│  └── providers.go  - AI provider registration imports       │
└─────────────────────────────────────────────────────────────┘
```

### Output Capture (capture.go)

Uses `os.Pipe()` to intercept both Go-level writes and subprocess writes:

```go
// CaptureSession replaces os.Stdout/os.Stderr with pipes.
// Tee goroutines read from pipes and write to both:
// 1. Original terminal (user sees output in real-time)
// 2. Buffer (captured for AI analysis)

session, _ := analyze.StartCapture()
// ... command runs, output flows to terminal AND buffer ...
stdout, stderr := session.Stop()  // Restores original streams
```

### AI Analysis (analyze.go)

```go
// ValidateAIConfig checks configuration before command execution:
// - AI enabled in atmos.yaml?
// - Provider configured?
// - API key present?
// Returns helpful error with configuration hints if not.

// AnalyzeOutput sends captured output to AI:
// - Builds prompt with command name, output, error status
// - Uses systemPrompt with infrastructure expertise
// - Truncates large output (50KB max)
// - Renders response as markdown
```

### Execution Flow in cmd/root.go

```go
func Execute() error {
    // ... config loading, setup ...

    aiEnabled := hasAIFlag() && !isAICommand()

    if aiEnabled {
        if err := analyze.ValidateAIConfig(&atmosConfig); err != nil {
            return err  // Helpful error with config hints
        }
        captureSession, _ = analyze.StartCapture()
    }

    cmd, err := internal.Execute(RootCmd)

    if aiEnabled && captureSession != nil {
        stdout, stderr := captureSession.Stop()
        analyze.AnalyzeOutput(&atmosConfig, commandName, stdout, stderr, err)
    }

    // ... telemetry, error handling ...
}
```

## Flag Infrastructure

### Flag Registration (pkg/flags/)

```go
// In global/flags.go:
AI bool // Enable AI-powered analysis of command output (--ai).

// In global_builder.go:
func (b *GlobalOptionsBuilder) registerAIFlags(defaults *global.Flags) {
    b.options = append(b.options, WithBoolFlag("ai", "", defaults.AI, "Enable AI-powered analysis of command output"))
    b.options = append(b.options, WithEnvVars("ai", "ATMOS_AI"))
}

// In global_registry.go ParseGlobalFlags:
AI: v.GetBool("ai"),
```

### Early Flag Parsing (cmd/root.go)

The `--ai` flag is parsed from `os.Args` before Cobra runs (like `--chdir` and `--use-version`):
- `hasAIFlagInternal(args)` — checks for `--ai` or `--ai=true`
- `isAICommandInternal(args)` — detects `atmos ai` commands to skip
- Respects `--` end-of-flags delimiter

## Error Handling

When `--ai` is used but AI is not configured:

```
Error: AI features are not enabled

Explanation:
  The --ai flag requires AI to be enabled in your atmos.yaml configuration.

Hints:
  - Add the following to your atmos.yaml:
    ai:
      enabled: true
      default_provider: anthropic
      providers:
        anthropic:
          model: claude-sonnet-4-5-20250514
          api_key: !env ANTHROPIC_API_KEY

  - See https://atmos.tools/cli/configuration/ai for full configuration options.
```

Uses Atmos error builder pattern with:
- Sentinel errors: `ErrAINotEnabled`, `ErrAIUnsupportedProvider`, `ErrAIAPIKeyNotFound`
- `WithExplanation()` for context
- `WithHint()` for actionable steps with example YAML configuration

## Usage Examples

```bash
# Analyze terraform plan output with AI
atmos --ai terraform plan vpc -s prod

# AI explains errors
atmos --ai terraform apply vpc -s prod
# If apply fails, AI explains the error and suggests fixes

# Enable via environment variable for CI/CD
export ATMOS_AI=true
atmos terraform plan vpc -s prod

# Works with any command
atmos --ai describe stacks
atmos --ai validate stacks
atmos --ai list components
atmos --ai aws security analyze
```

## Testing

### Unit Tests

| Test File | Tests |
|-----------|-------|
| `pkg/flags/global_builder_test.go` | AI flag registration on commands |
| `pkg/flags/global_registry_test.go` | AI flag parsing (default, CLI, env var) |
| `pkg/ai/analyze/capture_test.go` | Output capture (stdout, stderr, both, restore, empty) |
| `pkg/ai/analyze/analyze_test.go` | Config validation (not enabled, no provider, no key, valid, defaults), prompt building (success, error, empty, stderr only, whitespace), truncation |
| `cmd/root_test.go` | `hasAIFlagInternal` (present, absent, =true, =false, after --, similar flags), `isAICommandInternal` (ai commands, non-ai, flags before ai) |

### Coverage

- `pkg/ai/analyze/`: ~74% (AnalyzeOutput requires real AI client — integration test territory)
- `pkg/flags/`: Full coverage for AI flag registration and parsing
- `cmd/`: Full coverage for flag and command detection helpers

## Dependencies

- Requires AI provider configuration in `atmos.yaml` (provider, model, API key)
- Uses `pkg/ai` factory and registry for client creation
- Uses `pkg/utils` for markdown rendering
- See `docs/prd/atmos-ai.md` for the full AI integration PRD

## Future Considerations

- Command-specific AI prompts for different output types (plan, apply, describe)
- Streaming AI response for faster perceived latency
- AI provider auto-detection from environment
- Cost estimation and token usage reporting
- Integration with AI sessions for follow-up questions
