package analyze

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
)

// messageSender is the minimal interface needed for AI analysis.
type messageSender interface {
	SendMessage(ctx context.Context, message string) (string, error)
}

const (
	// DefaultAnalysisTimeout is the default timeout for AI analysis requests.
	defaultAnalysisTimeout = 120

	// MaxOutputLength is the maximum number of characters of command output (stdout+stderr)
	// to include in the analysis prompt. Skill prompts and system prompt are added separately.
	maxOutputLength = 50000

	// PromptOverheadReserve is the character budget reserved for skill prompts, system prompt,
	// and prompt framing (headers, separators, command name, error text). Output truncation
	// subtracts this from maxOutputLength to keep the total prompt within reasonable bounds.
	promptOverheadReserve = 20000

	// Newline constant for string building.
	newline = "\n"
)

// clientFactory creates AI clients. Overridden in tests for mocking.
var clientFactory func(cfg *schema.AtmosConfiguration) (messageSender, error) = func(cfg *schema.AtmosConfiguration) (messageSender, error) {
	return ai.NewClient(cfg)
}

// systemPrompt is the system prompt for AI analysis of command output.
//
//go:embed prompts/system.md
var systemPrompt string

// providerHintDefaults maps known providers to example model names and API key env vars.
var providerHintDefaults = map[string]struct{ model, envVar string }{
	"anthropic":   {model: "claude-sonnet-4-5-20250514", envVar: "ANTHROPIC_API_KEY"},
	"openai":      {model: "gpt-4o", envVar: "OPENAI_API_KEY"},
	"gemini":      {model: "gemini-2.0-flash", envVar: "GEMINI_API_KEY"},
	"bedrock":     {model: "anthropic.claude-sonnet-4-5-20250514-v2:0", envVar: "AWS_ACCESS_KEY_ID"},
	"azureopenai": {model: "gpt-4o", envVar: "AZURE_OPENAI_API_KEY"},
	"ollama":      {model: "llama3", envVar: "OLLAMA_API_KEY"},
	"grok":        {model: "grok-3", envVar: "GROK_API_KEY"},
}

// hintModel returns an example model name for the given provider.
func hintModel(provider string) string {
	if d, ok := providerHintDefaults[provider]; ok {
		return d.model
	}
	return "<model-name>"
}

// hintEnvVar returns an example API key env var name for the given provider.
func hintEnvVar(provider string) string {
	if d, ok := providerHintDefaults[provider]; ok {
		return d.envVar
	}
	return strings.ToUpper(provider) + "_API_KEY"
}

// ValidateAIConfig checks that AI is properly configured for the --ai flag.
// Returns a user-friendly error with hints if configuration is missing.
func ValidateAIConfig(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "analyze.ValidateAIConfig")()

	if !atmosConfig.AI.Enabled {
		return errUtils.Build(errUtils.ErrAINotEnabled).
			WithExplanation("The --ai flag requires AI to be enabled in your atmos.yaml configuration.").
			WithHint("Add the following to your atmos.yaml:\n\n```yaml\nai:\n  enabled: true\n  default_provider: anthropic\n  providers:\n    anthropic:\n      model: claude-sonnet-4-5-20250514\n      api_key: !env ANTHROPIC_API_KEY\n```").
			WithHint("See https://atmos.tools/cli/configuration/ai for full configuration options.").
			Err()
	}

	// Check that a provider is configured.
	provider := atmosConfig.AI.DefaultProvider
	if provider == "" {
		provider = "anthropic"
	}

	providerConfig, err := ai.GetProviderConfig(atmosConfig, provider)
	if err != nil {
		return errUtils.Build(errUtils.ErrAIUnsupportedProvider).
			WithCause(err).
			WithExplanationf("The --ai flag requires a configured AI provider, but provider %q is not configured.", provider).
			WithHintf("Add a provider configuration to your atmos.yaml:\n\n```yaml\nai:\n  enabled: true\n  default_provider: %s\n  providers:\n    %s:\n      model: %s\n      api_key: !env %s\n```", provider, provider, hintModel(provider), hintEnvVar(provider)).
			Err()
	}

	if providerConfig.ApiKey == "" {
		envVar := hintEnvVar(provider)
		return errUtils.Build(errUtils.ErrAIAPIKeyNotFound).
			WithExplanationf("The --ai flag requires an API key for provider %q, but none was found.", provider).
			WithHintf("Set the API key in your atmos.yaml using the !env function:\n\n```yaml\nai:\n  providers:\n    %s:\n      api_key: !env %s\n```", provider, envVar).
			WithHintf("Or set the environment variable directly: export %s=your-key", envVar).
			Err()
	}

	return nil
}

// AnalysisInput holds the inputs for AI analysis of command output.
type AnalysisInput struct {
	// CommandName is the full command string (e.g., "atmos terraform plan vpc -s prod").
	CommandName string
	// Stdout is the captured standard output.
	Stdout string
	// Stderr is the captured standard error.
	Stderr string
	// CmdErr is the error returned by the command (nil if successful).
	CmdErr error
	// SkillNames is the list of skill names used for AI analysis (e.g., ["atmos-terraform", "atmos-stacks"]).
	SkillNames []string
	// SkillPrompt is an optional skill system prompt for domain-specific expertise.
	SkillPrompt string
}

// AnalyzeOutput sends captured command output to the configured AI provider for analysis.
// It creates an AI client, builds a prompt with the command context, and renders the response.
func AnalyzeOutput(atmosConfig *schema.AtmosConfiguration, input *AnalysisInput) {
	defer perf.Track(atmosConfig, "analyze.AnalyzeOutput")()

	if input == nil {
		log.Error("AnalyzeOutput called with nil AnalysisInput")
		return
	}

	// Build the analysis prompt.
	prompt := buildAnalysisPrompt(input)
	if prompt == "" {
		log.Debug("No output to analyze, skipping AI analysis")
		return
	}

	// Create AI client.
	client, err := clientFactory(atmosConfig)
	if err != nil {
		log.Error("Failed to create AI client for output analysis", "error", err)
		return
	}

	// Set timeout.
	timeoutSeconds := defaultAnalysisTimeout
	if atmosConfig.AI.TimeoutSeconds > 0 {
		timeoutSeconds = atmosConfig.AI.TimeoutSeconds
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeoutSeconds)*time.Second)
	defer cancel()

	// Reinitialize the UI formatter now that output capture pipes are restored.
	// InitFormatter() ran during PersistentPreRun while pipes were active and cached
	// ColorNone. ReinitFormatter() re-detects the real terminal for color support.
	ui.ReinitFormatter()
	ui.Writeln("") // Visual separation before spinner.

	s := spinner.New(spinnerMessage(input.SkillNames))
	s.Start()

	// Send to AI provider.
	response, err := client.SendMessage(ctx, prompt)
	if err != nil {
		s.Error(fmt.Sprintf("AI analysis failed: %s", err.Error()))
		log.Error("AI analysis failed", "error", err)
		return
	}

	s.Success(successMessage(input.SkillNames))

	// Render AI response as markdown with colors to stderr (UI channel).
	ui.MarkdownMessage(response)

	// Add trailing newline for visual separation from subsequent output (e.g., exit status).
	ui.Writeln("")
}

// spinnerMessage returns the spinner text, including skill names if provided.
func spinnerMessage(skillNames []string) string {
	if len(skillNames) > 0 {
		return fmt.Sprintf("👽 Analyzing with AI using skills '%s'...", strings.Join(skillNames, "', '"))
	}
	return "👽 Analyzing with AI..."
}

// successMessage returns the success text, including skill names if provided.
func successMessage(skillNames []string) string {
	if len(skillNames) > 0 {
		return fmt.Sprintf("AI analysis complete (skills: %s)", strings.Join(skillNames, ", "))
	}
	return "AI analysis complete"
}

// buildAnalysisPrompt constructs the prompt for AI analysis.
func buildAnalysisPrompt(input *AnalysisInput) string {
	defer perf.Track(nil, "analyze.buildAnalysisPrompt")()

	// Reserve space for skill prompts, system prompt, and framing, then split
	// the remaining budget across stdout and stderr.
	outputBudget := maxOutputLength - len(input.SkillPrompt)
	if outputBudget < promptOverheadReserve {
		outputBudget = promptOverheadReserve
	}
	perStreamBudget := outputBudget
	if strings.TrimSpace(input.Stdout) != "" && strings.TrimSpace(input.Stderr) != "" {
		perStreamBudget = outputBudget / 2 //nolint:mnd // Half budget per stream when both present.
	}
	stdout := truncateOutput(input.Stdout, perStreamBudget)
	stderr := truncateOutput(input.Stderr, perStreamBudget)

	// Skip if there's nothing meaningful to analyze.
	if strings.TrimSpace(stdout) == "" && strings.TrimSpace(stderr) == "" && input.CmdErr == nil {
		return ""
	}

	var b strings.Builder

	// Skill-specific expertise comes first (if provided).
	if input.SkillPrompt != "" {
		b.WriteString(input.SkillPrompt)
		b.WriteString("\n\n---\n\n")
	}

	b.WriteString(systemPrompt)
	b.WriteString("\n\n---\n\n")

	// Command info.
	fmt.Fprintf(&b, "**Command:** `%s`\n\n", input.CommandName)

	// Error status.
	if input.CmdErr != nil {
		fmt.Fprintf(&b, "**Status:** Failed with error: %s\n\n", input.CmdErr.Error())
	} else {
		b.WriteString("**Status:** Success\n\n")
	}

	// Output streams.
	writeStream(&b, "**Standard Output:**", stdout)
	writeStream(&b, "**Standard Error:**", stderr)

	// Instructions based on error status.
	if input.CmdErr != nil {
		b.WriteString("Please analyze the error output above. Explain what went wrong and provide step-by-step instructions to fix it.\n")
	} else {
		b.WriteString("Please provide a concise summary and analysis of the command output above.\n")
	}

	return b.String()
}

// writeStream writes a labeled code block to the builder if the stream has non-whitespace content.
func writeStream(b *strings.Builder, label, content string) {
	if strings.TrimSpace(content) == "" {
		return
	}
	b.WriteString(label)
	b.WriteString("\n```\n")
	b.WriteString(content)
	if !strings.HasSuffix(content, newline) {
		b.WriteString(newline)
	}
	b.WriteString("```\n\n")
}

// truncateOutput limits output length to prevent exceeding AI token limits.
// The suffix is included within the limit so the result never exceeds it.
func truncateOutput(output string, limit int) string {
	const suffix = "\n... (output truncated)"

	if len(output) <= limit {
		return output
	}
	cutAt := limit - len(suffix)
	if cutAt <= 0 {
		return output[:limit]
	}
	return output[:cutAt] + suffix
}
