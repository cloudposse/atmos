# Atmos Global `--ai` Flag - Product Requirements Document

**Status:** In Progress (v3.0 — `--skill` flag)
**Version:** 3.0
**Last Updated:** 2026-03-11

---

## Executive Summary

The global `--ai` flag enables AI-powered analysis of command output across all Atmos CLI commands. When enabled, command output (stdout and stderr) is automatically captured, sent to the configured AI provider for analysis, and the AI response is rendered after command execution. For errors, the AI explains what went wrong and how to fix it. For successful output, it provides a concise summary.

The companion `--skill <name>` flag provides domain-specific context to the AI by loading a skill's system prompt. This gives the AI deep knowledge of a particular Atmos subsystem (e.g., Terraform, stacks, validation) for more accurate and actionable analysis.

## Motivation

Users frequently need to interpret complex command output (e.g., Terraform plan diffs, stack descriptions, validation results). By providing a global `--ai` flag, users can opt into AI-assisted analysis on any command without changing their workflow.

**Key Benefits:**
- Zero-friction AI integration — just add `--ai` to any command
- Works with ALL existing commands (terraform, helmfile, describe, validate, list, etc.)
- Error explanation with actionable fix instructions
- Domain-specific analysis with `--skill` — the AI uses skill knowledge for deeper insights
- Environment variable support (`ATMOS_AI`, `ATMOS_SKILL`) for CI/CD and scripting
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
12. **Skill Flag**: The `--skill <name>` flag MUST be a global persistent string flag that specifies a skill to use for AI analysis context.
13. **Skill Requires AI**: If `--skill` is used without `--ai`, MUST show a helpful error explaining that `--skill` requires `--ai`.
14. **Skill Validation**: The specified skill MUST exist in the skill registry (marketplace-installed or custom). If not found, MUST show an error listing available skills.
15. **Skill Context Injection**: When a valid skill is specified, the skill's system prompt MUST be prepended to the analysis prompt, giving the AI domain-specific expertise.
16. **Skill Environment Variable**: The flag MUST support the `ATMOS_SKILL` environment variable.
17. **Skill Flag Precedence**: Standard Atmos precedence MUST apply: CLI flag > `ATMOS_SKILL` env var > config file > default (empty string).

### Non-Functional Requirements

1. **Zero Overhead When Disabled**: When `--ai` is `false` (default), there MUST be no performance impact (no pipes, no buffers).
2. **Backward Compatible**: Adding the flag MUST NOT break existing commands or workflows.
3. **Cross-Platform**: Output capture using `os.Pipe()` works on Linux, macOS, and Windows.
4. **Output Truncation**: Large outputs MUST be truncated to prevent exceeding AI token limits (50KB max).
5. **Timeout**: AI analysis requests MUST have a configurable timeout (default: 120s).
6. **Documentation**: The flag MUST be documented in global-flags.mdx with examples and environment variable reference.

## Architecture

### Component Overview

```text
┌──────────────────────────────────────────────────────────────────┐
│  cmd/root.go Execute()                                           │
│  ┌──────────────────────────────────────────────────────────┐    │
│  │ 1. Parse --ai and --skill from os.Args (before Cobra)    │    │
│  │ 2. Validate --skill requires --ai                        │    │
│  │ 3. Validate AI config (fail fast with hints)             │    │
│  │ 4. If --skill: load skill registry, validate skill name  │    │
│  │ 5. Start output capture (os.Pipe + tee goroutines)       │    │
│  │ 6. Run command (internal.Execute)                        │    │
│  │ 7. Stop capture, send output + skill context to AI       │    │
│  │ 8. Render AI analysis as markdown                        │    │
│  └──────────────────────────────────────────────────────────┘    │
│                                                                  │
│  pkg/ai/analyze/                                                 │
│  ├── capture.go    - CaptureSession (os.Pipe + tee)              │
│  ├── analyze.go    - ValidateAIConfig, AnalyzeOutput             │
│  └── providers.go  - AI provider registration imports            │
│                                                                  │
│  pkg/ai/skills/                                                  │
│  ├── skill.go      - Skill struct (Name, SystemPrompt, etc.)     │
│  ├── registry.go   - Registry (Get, Has, List)                   │
│  └── loader.go     - LoadSkills (marketplace + custom)           │
└──────────────────────────────────────────────────────────────────┘
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
// - If skillPrompt is provided, prepends it to the system prompt for domain expertise
// - Uses systemPrompt with infrastructure expertise
// - Truncates large output (50KB max)
// - Renders response as markdown
```

### Skill Prompt Injection

When `--skill <name>` is specified, the skill's full markdown content (its `SystemPrompt` field) is sent to the AI provider as part of the analysis prompt. The prompt is assembled in this order:

```text
┌─────────────────────────────────────────────────┐
│ 1. Skill system prompt (full markdown content)  │  ← domain expertise (e.g., Terraform knowledge)
│    ───────── \n\n---\n\n separator ──────────   │
│ 2. General analysis system prompt               │  ← infrastructure/DevOps expertise
│    ───────── \n\n---\n\n separator ──────────   │
│ 3. Command context                              │  ← command name, success/failure status
│ 4. Captured stdout (code block)                 │  ← command's standard output
│ 5. Captured stderr (code block)                 │  ← command's standard error
│ 6. Analysis instructions                        │  ← error-specific or success-specific guidance
└─────────────────────────────────────────────────┘
```

**Without `--skill`**, the skill layer (1) is omitted and the prompt starts with the general system prompt (2).

**Data flow:**

1. `parseSkillFlag()` extracts the skill name from `os.Args` (e.g., `"atmos-terraform"`)
2. `setupAIAnalysis()` → `loadAndValidateSkill()` → loads skills from marketplace (`~/.atmos/skills/`) → `registry.Get(skillName)` → returns `*skills.Skill`
3. `skill.SystemPrompt` (the full markdown content of the skill file) is stored as `skillPrompt string`
4. `runAIAnalysis()` passes `skillPrompt` in `AnalysisInput{SkillPrompt: skillPrompt}`
5. `buildAnalysisPrompt()` prepends the skill prompt before the general system prompt with a `\n\n---\n\n` separator
6. The complete prompt is sent to the AI provider via `client.SendMessage(ctx, prompt)`

This means the AI receives the skill's entire domain knowledge (e.g., Terraform best practices, Atmos stack conventions) alongside the command output, enabling significantly more accurate and actionable analysis.

### Execution Flow in cmd/root.go

```go
func Execute() error {
    // ... config loading, setup ...

    aiEnabled := hasAIFlag() && !isAICommand()
    skillName := parseSkillFlag()  // Parse --skill from os.Args
    var captureSession *analyze.CaptureSession
    var skillPrompt string

    // Validate --skill requires --ai.
    if skillName != "" && !aiEnabled {
        return errUtils.Build(errUtils.ErrAISkillRequiresAIFlag).
            WithExplanation("...").WithHintf("...").Err()
    }

    if aiEnabled {
        // setupAIAnalysis validates config, loads skill, starts capture.
        var setupErr error
        captureSession, skillPrompt, setupErr = setupAIAnalysis(&atmosConfig, skillName)
        if setupErr != nil {
            return setupErr
        }
        if captureSession == nil {
            aiEnabled = false  // Capture failed, disable AI
        }
    }

    cmd, err := internal.Execute(RootCmd)

    telemetry.CaptureCmd(cmd, err)

    // Stop capture, print error (if any), then run AI analysis.
    if aiEnabled && captureSession != nil {
        runAIAnalysis(&atmosConfig, captureSession, err, skillPrompt)
        if err != nil {
            errUtils.Exit(errUtils.GetExitCode(err))  // Error already printed
        }
        return nil
    }

    return err
}
```

**Error Handling with `--ai`**:

1. **Error Propagation**: Command functions (e.g., `executeSingleComponent`, `terraformRunWithOptions`) return errors through Cobra's `RunE` instead of calling `errUtils.CheckErrorPrintAndExit()` / `os.Exit()`. This ensures errors flow back to `Execute()` where AI analysis can process them.

2. **Output Ordering**: `runAIAnalysis()` prints the formatted error to stderr BEFORE sending output to the AI provider. This ensures the user sees: error message → AI explanation (not the reverse).

3. **Exit Handling**: After AI analysis, `Execute()` calls `errUtils.Exit(code)` directly to prevent `main.go` from re-printing the error. For successful commands, it returns `nil`.

4. **UI Output**: All AI output (spinner, markdown response, status messages) goes to stderr via `ui.Writeln()` and `ui.MarkdownMessage()`, keeping stdout clean for piping. `ui.ReinitFormatter()` is called after capture stops to restore color detection.

## Flag Infrastructure

### Flag Registration (pkg/flags/)

```go
// In global/flags.go:
AI    bool   // Enable AI-powered analysis of command output (--ai).
Skill string // Specify skill for AI analysis context (--skill).

// In global_builder.go:
func (b *GlobalOptionsBuilder) registerAIFlags(defaults *global.Flags) {
    b.options = append(b.options, WithBoolFlag("ai", "", defaults.AI, "Enable AI-powered analysis of command output"))
    b.options = append(b.options, WithEnvVars("ai", "ATMOS_AI"))
    b.options = append(b.options, WithStringFlag("skill", "", defaults.Skill, "Specify skill for AI analysis context (requires --ai)"))
    b.options = append(b.options, WithEnvVars("skill", "ATMOS_SKILL"))
}

// In global_registry.go:
registry.Register(&StringFlag{
    Name:        "skill",
    Default:     "",
    Description: "Specify skill for AI analysis context (requires --ai)",
    EnvVars:     []string{"ATMOS_SKILL"},
})

// In ParseGlobalFlags:
AI:    v.GetBool("ai"),
Skill: v.GetString("skill"),
```

### Early Flag Parsing (cmd/root.go)

Both `--ai` and `--skill` are parsed from `os.Args` before Cobra runs (like `--chdir` and `--use-version`):
- `hasAIFlagInternal(args)` — checks for `--ai` or `--ai=true`
- `parseSkillFlagInternal(args)` — extracts skill name from `--skill <name>` or `--skill=<name>`
- `isAICommandInternal(args)` — detects `atmos ai` commands to skip
- Respects `--` end-of-flags delimiter

## Error Handling

When `--ai` is used but AI is not configured:

```text
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
- Sentinel errors: `ErrAINotEnabled`, `ErrAIUnsupportedProvider`, `ErrAIAPIKeyNotFound`, `ErrAISkillRequiresAIFlag`, `ErrAISkillNotFound`
- `WithExplanation()` for context
- `WithHint()` for actionable steps with example YAML configuration

### `--skill` without `--ai`

```text
Error: --skill flag requires --ai

Explanation:
  The --skill flag provides domain-specific context for AI analysis, but AI analysis
  is not enabled. Use --skill together with --ai.

Hints:
  - Add --ai to enable AI analysis:
    atmos --ai --skill atmos-terraform terraform plan vpc -s prod

  - Or use ATMOS_AI=true with ATMOS_SKILL:
    ATMOS_AI=true ATMOS_SKILL=atmos-terraform atmos terraform plan vpc -s prod
```

### Invalid skill name

```text
Error: AI skill not found: "my-skill"

Explanation:
  The skill "my-skill" is not installed or configured. Available skills are loaded
  from marketplace installations (~/.atmos/skills/) and custom skills in atmos.yaml.

Hints:
  - Available skills: atmos-ansible, atmos-auth, atmos-components, atmos-config,
    atmos-custom-commands, atmos-design-patterns, atmos-devcontainer, atmos-gitops,
    atmos-helmfile, atmos-introspection, atmos-packer, atmos-schemas, atmos-stacks,
    atmos-stores, atmos-templates, atmos-terraform, atmos-toolchain, atmos-validation,
    atmos-vendoring, atmos-workflows, atmos-yaml-functions

  - Install skills: atmos ai skill install cloudposse/atmos

  - See https://atmos.tools/ai/agent-skills for more information.
```

## Usage Examples

```bash
# Analyze terraform plan output with AI
atmos --ai terraform plan vpc -s prod

# AI explains errors
atmos --ai terraform apply vpc -s prod
# If apply fails, AI explains the error and suggests fixes

# Use a skill for domain-specific AI analysis
atmos --ai --skill atmos-terraform terraform plan vpc -s prod
atmos --ai --skill atmos-stacks describe stacks
atmos --ai --skill atmos-validation validate stacks

# Enable via environment variable for CI/CD
export ATMOS_AI=true
atmos terraform plan vpc -s prod

# Enable with skill via environment variables
ATMOS_AI=true ATMOS_SKILL=atmos-terraform atmos terraform plan vpc -s prod

# Works with any command
atmos --ai describe stacks
atmos --ai validate stacks
atmos --ai list components
atmos --ai aws security analyze
```

## Testing

### Unit Tests

| Test File                           | Tests                                                                                                                                                                                        |
|-------------------------------------|----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| `pkg/flags/global_builder_test.go`  | AI and Skill flag registration on commands                                                                                                                                                   |
| `pkg/flags/global_registry_test.go` | AI and Skill flag parsing (default, CLI, env var)                                                                                                                                            |
| `pkg/ai/analyze/capture_test.go`    | Output capture (stdout, stderr, both, restore, empty)                                                                                                                                        |
| `pkg/ai/analyze/analyze_test.go`    | Config validation (not enabled, no provider, no key, valid, defaults), prompt building (success, error, empty, stderr only, whitespace, with skill prompt, without skill prompt), truncation |
| `cmd/root_test.go`                  | `hasAIFlagInternal`, `parseSkillFlagInternal` (present, absent, =value, separate value, after --, similar flags), `isAICommandInternal`                                                      |

### New Tests for `--skill`

| Test File                         | Tests                                                                                                                               |
|-----------------------------------|-------------------------------------------------------------------------------------------------------------------------------------|
| `cmd/root_test.go`                | `parseSkillFlagInternal` — `--skill atmos-terraform`, `--skill=atmos-terraform`, `--skill` after `--`, missing value, similar flags |
| `pkg/ai/analyze/analyze_test.go`  | `buildAnalysisPrompt` with skill prompt prepended, without skill prompt (backward compatible)                                       |
| `cmd/root_test.go` or integration | `--skill` without `--ai` returns error, `--skill` with invalid name returns error listing available skills                          |

### Coverage

- `pkg/ai/analyze/`: ~95% (AnalyzeOutput tested via mock client; only OS pipe failure path uncovered)
- `pkg/flags/`: Full coverage for AI and Skill flag registration and parsing
- `cmd/`: Full coverage for flag and command detection helpers

## Dependencies

- Requires AI provider configuration in `atmos.yaml` (provider, model, API key)
- Uses `pkg/ai` factory and registry for client creation
- Uses `pkg/ui` for markdown rendering (`ui.MarkdownMessage`) and status output (`ui.Writeln`), with `ui.ReinitFormatter()` to restore color after capture
- Uses `pkg/ai/analyze` for output capture (`CaptureSession`), config validation, and AI analysis
- Uses `pkg/ai/skills` for skill loading (`LoadSkills`), registry (`Registry`), and marketplace loader
- Uses `pkg/ai/skills/marketplace` for loading installed skills from `~/.atmos/skills/`
- See `docs/prd/atmos-ai.md` for the full AI integration PRD

## Implementation Steps for `--skill` Flag

### Step 1: Add Sentinel Errors (`errors/errors.go`)

```go
ErrAISkillRequiresAIFlag = errors.New("--skill flag requires --ai")
```

Note: `ErrAISkillNotFound` already exists in `errors/errors.go`.

### Step 2: Add `Skill` Field to Global Flags (`pkg/flags/global/flags.go`)

```go
// AI integration.
AI    bool   // Enable AI-powered analysis of command output (--ai).
Skill string // Specify skill for AI analysis context (--skill).
```

### Step 3: Register `--skill` Flag (`pkg/flags/global_builder.go` and `global_registry.go`)

In `registerAIFlags`:

```go
// global_builder.go
b.options = append(b.options, WithStringFlag("skill", "", defaults.Skill,
    "Specify skill for AI analysis context (requires --ai)"))
b.options = append(b.options, WithEnvVars("skill", "ATMOS_SKILL"))

// global_registry.go
registry.Register(&StringFlag{
    Name:        "skill",
    Default:     "",
    Description: "Specify skill for AI analysis context (requires --ai)",
    EnvVars:     []string{"ATMOS_SKILL"},
})
```

In `ParseGlobalFlags`:

```go
Skill: v.GetString("skill"),
```

### Step 4: Add Early Flag Parsing (`cmd/root.go`)

Add `parseSkillFlagInternal(args)` that extracts the skill name from `os.Args` before Cobra parses.
Handles `--skill <name>`, `--skill=<name>`, respects `--` delimiter.

```go
func parseSkillFlag() string {
    return parseSkillFlagInternal(os.Args)
}

func parseSkillFlagInternal(args []string) string {
    for i, arg := range args {
        if arg == "--" { break }
        if arg == "--skill" && i+1 < len(args) {
            return args[i+1]
        }
        if strings.HasPrefix(arg, "--skill=") {
            return strings.TrimPrefix(arg, "--skill=")
        }
    }
    return ""
}
```

### Step 5: Add Skill Validation (`cmd/root.go`)

Add `loadAndValidateSkill()` that:
1. Loads the skill registry using `skills.LoadSkills()` with marketplace loader
2. Checks if the skill exists using `registry.Get(skillName)`
3. Returns the skill or a helpful error listing available skills

```go
func loadAndValidateSkill(atmosConfig *schema.AtmosConfiguration, skillName string) (*skills.Skill, error) {
    loader := marketplace.NewInstaller(atmosConfig)
    registry, _ := skills.LoadSkills(atmosConfig, loader)

    skill, err := registry.Get(skillName)
    if err != nil {
        available := registry.List()
        names := make([]string, len(available))
        for i, s := range available { names[i] = s.Name }
        return nil, errUtils.Build(errUtils.ErrAISkillNotFound).
            WithExplanationf("The skill %q is not installed or configured.", skillName).
            WithHintf("Available skills: %s", strings.Join(names, ", ")).
            WithHint("Install skills: atmos ai skill install cloudposse/atmos").
            Err()
    }
    return skill, nil
}
```

### Step 6: Update `Execute()` Flow (`cmd/root.go`)

After `aiEnabled` is determined:
1. Parse `--skill` from `os.Args`
2. If `--skill` without `--ai`, return error
3. If `--skill` with `--ai`, load and validate skill, extract system prompt
4. Pass skill prompt to `runAIAnalysis()`

### Step 7: Update `runAIAnalysis()` (`cmd/root.go`)

Add `skillPrompt string` parameter. Pass it through to `analyze.AnalyzeOutput()` via `AnalysisInput`.

```go
func runAIAnalysis(atmosConfig *schema.AtmosConfiguration, captureSession *analyze.CaptureSession,
    cmdErr error, skillPrompt string) {
    // ... existing error formatting ...
    analyze.AnalyzeOutput(atmosConfig, &analyze.AnalysisInput{
        CommandName: commandName,
        Stdout:      stdout,
        Stderr:      stderrCaptured,
        CmdErr:      cmdErr,
        SkillPrompt: skillPrompt,
    })
}
```

### Step 8: Update `AnalyzeOutput()` and `buildAnalysisPrompt()` (`pkg/ai/analyze/analyze.go`)

Use `AnalysisInput` struct to pass all parameters (introduced to satisfy the `argument-limit` linter rule).
When a skill prompt is provided, prepend it to the system prompt for domain-specific expertise:

```go
// AnalysisInput holds the inputs for AI analysis of command output.
type AnalysisInput struct {
    CommandName string // Full command string (e.g., "atmos terraform plan vpc -s prod").
    Stdout      string // Captured standard output.
    Stderr      string // Captured standard error.
    CmdErr      error  // Error returned by the command (nil if successful).
    SkillPrompt string // Optional skill system prompt for domain-specific expertise.
}

func AnalyzeOutput(atmosConfig *schema.AtmosConfiguration, input *AnalysisInput) {
    prompt := buildAnalysisPrompt(input)
    // ... rest unchanged ...
}

func buildAnalysisPrompt(input *AnalysisInput) string {
    var b strings.Builder

    // Skill-specific expertise comes first (if provided).
    if input.SkillPrompt != "" {
        b.WriteString(input.SkillPrompt)
        b.WriteString("\n\n---\n\n")
    }

    // Then the general analysis system prompt.
    b.WriteString(systemPrompt)
    // ... rest unchanged ...
}
```

### Step 9: Add Tests

1. **`cmd/root_test.go`**: Table-driven tests for `parseSkillFlagInternal` (same pattern as `hasAIFlagInternal`)
2. **`pkg/ai/analyze/analyze_test.go`**: Tests for `buildAnalysisPrompt` with and without skill prompt
3. **`pkg/flags/global_builder_test.go`**: Skill flag registration test
4. **`pkg/flags/global_registry_test.go`**: Skill flag parsing (default, CLI, env var)

### Step 10: Update Documentation

1. Update `website/docs/cli/global-flags.mdx` to document `--skill` and `ATMOS_SKILL`
2. Update blog post with `--skill` examples
3. Update `examples/ai/README.md` with `--skill` usage

## Resolved: Capture Timing Issue (ui.ReinitFormatter)

### Problem

`ui.InitFormatter()` runs during Cobra's `PersistentPreRun` while output capture pipes are active.
The formatter's `terminal.New()` detects the pipe instead of the real terminal → caches `ColorNone` → no colors.
After `captureSession.Stop()` restores `os.Stdout`/`os.Stderr`, the formatter's terminal state is stale.

Note: The I/O writer issue was a non-problem — `io.Context` uses dynamic writers (`func() { return os.Stderr }`)
that resolve `os.Stderr` at write time, not at creation time. Only the terminal color detection was stale.

### Solution

Added `ui.ReinitFormatter()` which creates a fresh `io.Context` and calls `InitFormatter()`, re-detecting
the real terminal with correct color capabilities. Called in `AnalyzeOutput()` before rendering the AI response.

```go
// In pkg/ui/formatter.go:
func ReinitFormatter() {
    ioCtx, _ := io.NewContext()
    InitFormatter(ioCtx)
}

// In pkg/ai/analyze/analyze.go:
ui.ReinitFormatter()     // Re-detect terminal after capture stops
ui.MarkdownMessage(resp) // Now renders with full color support
```

This replaced the previous workaround of using `utils.PrintfMarkdownToTUI` / `utils.PrintfMessageToTUI`.
AI analysis output now uses the standard `ui` package: `ui.Writeln()` and `ui.MarkdownMessage()`.

## Future Considerations

- Auto-detect skill based on command (e.g., `terraform plan` → `atmos-terraform`)
- Streaming AI response for faster perceived latency
- AI provider auto-detection from environment
- Cost estimation and token usage reporting
- Integration with AI sessions for follow-up questions
