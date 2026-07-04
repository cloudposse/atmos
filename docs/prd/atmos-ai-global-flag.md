# Atmos Global `--ai` Flag - Product Requirements Document

**Status:** Shipped
**Version:** 3.1
**Last Updated:** 2026-03-30

---

## Executive Summary

The global `--ai` flag enables AI-powered analysis of command output across all Atmos CLI commands. When enabled, command output (stdout and stderr) is automatically captured, sent to the configured AI provider for analysis, and the AI response is rendered after command execution. For errors, the AI explains what went wrong and how to fix it. For successful output, it provides a concise summary.

The companion `--skill <name>` flag provides domain-specific context to the AI by loading skills' system prompts. Multiple skills can be specified via comma-separated values (`--skill a,b`) or repeated flags (`--skill a --skill b`). This gives the AI deep knowledge of particular Atmos subsystems (e.g., Terraform, stacks, validation) for more accurate and actionable analysis.

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
12. **Skill Flag**: The `--skill <name>` flag MUST be a global persistent string slice flag that specifies one or more skills for AI analysis context. Supports comma-separated values (`--skill a,b`) and repeated flags (`--skill a --skill b`).
13. **Skill Requires AI**: If `--skill` is used without `--ai`, MUST show a helpful error explaining that `--skill` requires `--ai`.
14. **Skill Validation**: All specified skills MUST exist in the skill registry (marketplace-installed or custom). If any are not found, MUST show an error listing the invalid and available skills.
15. **Skill Context Injection**: When valid skills are specified, their system prompts MUST be concatenated (separated by `\n\n---\n\n`) and prepended to the analysis prompt, giving the AI multi-domain expertise.
16. **Skill Environment Variable**: The flag MUST support the `ATMOS_SKILL` environment variable with comma-separated values.
17. **Skill Flag Precedence**: Standard Atmos precedence MUST apply: CLI flag > `ATMOS_SKILL` env var > config file > default (empty slice).

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

session, err := analyze.StartCapture()
if err != nil {
    return err
}
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

1. `parseSkillFlag()` extracts skill names from `os.Args` — supports comma-separated (`--skill a,b`) and repeated flags (`--skill a --skill b`), returns `[]string`
2. `setupAIAnalysis()` → `loadAndValidateSkills()` → loads each skill from marketplace (`~/.atmos/skills/`) → `registry.Get(name)` → returns `[]*skills.Skill`
3. Each `skill.SystemPrompt` is concatenated with `\n\n---\n\n` separators into a single `skillPrompt string`
4. `runAIAnalysis()` passes `skillPrompt` in `AnalysisInput{SkillPrompt: skillPrompt, SkillNames: skillNames}`
5. `buildAnalysisPrompt()` prepends the merged skill prompt before the general system prompt
6. The complete prompt is sent to the AI provider via `client.SendMessage(ctx, prompt)`

This means the AI receives the combined domain knowledge from all specified skills (e.g., Terraform best practices + stack conventions) alongside the command output, enabling significantly more accurate and actionable analysis.

### Execution Flow in cmd/root.go

```go
func Execute() error {
    // ... config loading, setup ...

    aiEnabled := hasAIFlag() && !isAICommand()
    skillNames := parseSkillFlag()  // Parse --skill from os.Args ([]string)
    var captureSession *analyze.CaptureSession
    var skillPrompt string

    // Validate --skill requires --ai.
    if len(skillNames) > 0 && !aiEnabled {
        return errUtils.Build(errUtils.ErrAISkillRequiresAIFlag).
            WithExplanation("...").WithHintf("...").Err()
    }

    if aiEnabled {
        // setupAIAnalysis validates config, loads skills, starts capture.
        var setupErr error
        captureSession, skillPrompt, setupErr = setupAIAnalysis(&atmosConfig, skillNames)
        if setupErr != nil {
            return setupErr
        }
        if captureSession == nil {
            aiEnabled = false  // Capture failed, disable AI
        } else {
            // Ensure stdout/stderr are restored even on panic.
            defer captureSession.Stop()
        }
    }

    cmd, err := internal.Execute(RootCmd)

    telemetry.CaptureCmd(cmd, err)

    // Stop capture, print error (if any), then run AI analysis.
    if aiEnabled && captureSession != nil {
        runAIAnalysis(&atmosConfig, captureSession, err, skillNames, skillPrompt)
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
AI    bool     // Enable AI-powered analysis of command output (--ai).
Skill []string // Specify skills for AI analysis context (--skill, comma-separated or repeated).

// In global_builder.go:
func (b *GlobalOptionsBuilder) registerAIFlags(defaults *global.Flags) {
    b.options = append(b.options, WithBoolFlag("ai", "", defaults.AI, "Enable AI-powered analysis of command output"))
    b.options = append(b.options, WithEnvVars("ai", "ATMOS_AI"))
    b.options = append(b.options, func(cfg *parserConfig) {
        cfg.registry.Register(&StringSliceFlag{
            Name:        "skill",
            Default:     defaults.Skill,
            Description: "Specify skills for AI analysis context (comma-separated or repeated flag, requires --ai)",
            EnvVars:     []string{"ATMOS_SKILL"},
        })
    })
}

// In global_registry.go:
registry.Register(&StringSliceFlag{
    Name:        "skill",
    Default:     []string{},
    Description: "Specify skills for AI analysis context (comma-separated or repeated flag, requires --ai)",
    EnvVars:     []string{"ATMOS_SKILL"},
})

// In ParseGlobalFlags:
AI:    v.GetBool("ai"),
Skill: v.GetStringSlice("skill"),
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
    atmos terraform plan vpc -s prod --ai --skill atmos-terraform

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
atmos terraform plan vpc -s prod --ai

# AI explains errors
atmos terraform apply vpc -s prod --ai
# If apply fails, AI explains the error and suggests fixes

# Use a skill for domain-specific AI analysis
atmos terraform plan vpc -s prod --ai --skill atmos-terraform
atmos describe stacks --ai --skill atmos-stacks
atmos validate stacks --ai --skill atmos-validation

# Multiple skills (comma-separated or repeated flag)
atmos terraform plan vpc -s prod --ai --skill atmos-terraform,atmos-stacks
atmos terraform plan vpc -s prod --ai --skill atmos-terraform --skill atmos-stacks

# Enable via environment variable for CI/CD
export ATMOS_AI=true
atmos terraform plan vpc -s prod

# Enable with skills via environment variables
ATMOS_AI=true ATMOS_SKILL=atmos-terraform,atmos-stacks atmos terraform plan vpc -s prod

# Works with any command
atmos describe stacks --ai
atmos validate stacks --ai
atmos list components --ai
atmos aws security analyze --ai
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
AI    bool     // Enable AI-powered analysis of command output (--ai).
Skill []string // Specify skills for AI analysis context (--skill, comma-separated or repeated).
```

### Step 3: Register `--skill` Flag (`pkg/flags/global_builder.go` and `global_registry.go`)

In `registerAIFlags`:

```go
// global_builder.go
b.options = append(b.options, func(cfg *parserConfig) {
    cfg.registry.Register(&StringSliceFlag{
        Name:        "skill",
        Default:     defaults.Skill,
        Description: "Specify skills for AI analysis context (comma-separated or repeated flag, requires --ai)",
        EnvVars:     []string{"ATMOS_SKILL"},
    })
})

// global_registry.go
registry.Register(&StringSliceFlag{
    Name:        "skill",
    Default:     []string{},
    Description: "Specify skills for AI analysis context (comma-separated or repeated flag, requires --ai)",
    EnvVars:     []string{"ATMOS_SKILL"},
})
```

In `ParseGlobalFlags`:

```go
Skill: v.GetStringSlice("skill"),
```

### Step 4: Add Early Flag Parsing (`cmd/root.go`)

Add `parseSkillFlagInternal(args)` that extracts all skill names from `os.Args` before Cobra parses.
Supports repeated flags (`--skill a --skill b`) and comma-separated values (`--skill a,b`).
Handles `--skill <name>`, `--skill=<name>`, respects `--` delimiter.

```go
func parseSkillFlag() []string {
    return parseSkillFlagInternal(os.Args)
}

func parseSkillFlagInternal(args []string) []string {
    var result []string
    flagSeen := false
    for i, arg := range args {
        if arg == "--" { break }
        var value string
        if arg == "--skill" && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
            value = args[i+1]
            flagSeen = true
        } else if strings.HasPrefix(arg, "--skill=") {
            value = strings.TrimPrefix(arg, "--skill=")
            flagSeen = true
        }
        if value != "" {
            result = append(result, splitCSV(value)...)
        }
    }
    // Fall back to ATMOS_SKILL env var only when no --skill CLI flag was provided.
    if !flagSeen {
        result = splitCSV(os.Getenv("ATMOS_SKILL"))
    }
    return result
}
```

### Step 5: Add Skill Validation (`cmd/root.go`)

Add `loadAndValidateSkills()` that:
1. Loads the skill registry using `skills.LoadSkills()` with marketplace loader
2. Checks if all skills exist using `registry.Get(name)`
3. Returns the skills or a helpful error listing invalid and available skills

```go
func loadAndValidateSkills(atmosConfig *schema.AtmosConfiguration, skillNames []string) ([]*skills.Skill, error) {
    loader := marketplace.NewInstaller(atmosConfig)
    registry, _ := skills.LoadSkills(atmosConfig, loader)

    var validSkills []*skills.Skill
    var invalidNames []string
    for _, name := range skillNames {
        skill, err := registry.Get(name)
        if err != nil { invalidNames = append(invalidNames, name) } else { validSkills = append(validSkills, skill) }
    }
    if len(invalidNames) > 0 {
        available := registry.List()
        names := make([]string, 0, len(available))
        for _, s := range available { names = append(names, s.Name) }
        return nil, errUtils.Build(errUtils.ErrAISkillNotFound).
            WithExplanationf("The following skills are not installed or configured: %s", strings.Join(invalidNames, ", ")).
            WithHintf("Available skills: %s", strings.Join(names, ", ")).
            Err()
    }
    return validSkills, nil
}
```

### Step 6: Update `Execute()` Flow (`cmd/root.go`)

After `aiEnabled` is determined:
1. Parse `--skill` from `os.Args` (returns `[]string`)
2. If `len(skillNames) > 0` without `--ai`, return error
3. If skills with `--ai`, load and validate all skills, concatenate system prompts
4. Pass skill names and merged prompt to `runAIAnalysis()`

### Step 7: Update `runAIAnalysis()` (`cmd/root.go`)

Change `skillName string` to `skillNames []string`. Pass through to `analyze.AnalyzeOutput()` via `AnalysisInput`.

```go
func runAIAnalysis(atmosConfig *schema.AtmosConfiguration, captureSession *analyze.CaptureSession,
    cmdErr error, skillNames []string, skillPrompt string) {
    // ... existing error formatting ...
    analyze.AnalyzeOutput(atmosConfig, &analyze.AnalysisInput{
        CommandName: commandName,
        Stdout:      stdout,
        Stderr:      stderrCaptured,
        CmdErr:      cmdErr,
        SkillNames:  skillNames,
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
    CommandName string   // Full command string (e.g., "atmos terraform plan vpc -s prod").
    Stdout      string   // Captured standard output.
    Stderr      string   // Captured standard error.
    CmdErr      error    // Error returned by the command (nil if successful).
    SkillNames  []string // Skill names used for AI analysis (e.g., ["atmos-terraform", "atmos-stacks"]).
    SkillPrompt string   // Concatenated skill system prompts for domain-specific expertise.
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

- Streaming AI response for faster perceived latency
- AI provider auto-detection from environment
- Cost estimation and token usage reporting
- Integration with AI sessions for follow-up questions

### Auto-Skill Selection (when `--skill` is not specified)

When `--ai` is used without `--skill`, Atmos could automatically select the most relevant skill(s)
based on the command being run. Three approaches, in order of implementation priority:

#### Approach 1: Command-based mapping (recommended first)

Map Atmos command prefixes to skills deterministically in code. Zero latency, no AI call.

```go
var defaultSkillMap = map[string][]string{
    "terraform":  {"atmos-terraform"},
    "helmfile":   {"atmos-helmfile"},
    "validate":   {"atmos-validation"},
    "describe":   {"atmos-introspection"},
    "list":       {"atmos-introspection"},
    "workflow":   {"atmos-workflows"},
    "vendor":     {"atmos-vendoring"},
}
```

**Pros:** Instant, deterministic, no extra API cost, easy to test.
**Cons:** Cannot handle nuanced cases (e.g., a terraform error that is really a stack config issue
needing `atmos-stacks`).

#### Approach 2: Skill metadata matching (layer on top of Approach 1)

Add a `triggers` field to SKILL.md metadata so skill authors can define when their skill applies:

```yaml
---
name: atmos-terraform
triggers:
  commands: ["terraform"]
  keywords: ["plan", "apply", "init", "state", "backend"]
---
```

The code scans installed skills, matches against the current command and output keywords.
No AI call required.

**Pros:** Extensible — skill authors define their own triggers. No API cost.
**Cons:** More complex implementation. Keyword matching can be imprecise.

#### Approach 3: AI-powered selection (not recommended)

Send the skill list and command context to the AI to pick the best skill(s). Requires an extra
API call before the actual analysis.

**Pros:** Handles edge cases intelligently.
**Cons:** Extra API round-trip adds latency and cost. Overkill when the command name already
indicates which skill to use. Defeats the speed advantage of `--ai`.

#### Recommendation

Start with Approach 1 (covers ~90% of cases, trivial to implement). Layer Approach 2 for
custom/community skills. Skip Approach 3 — the latency and cost are not justified.

When no skill matches, fall back to general analysis (no skill), which is the current behavior.
