package analyze

import (
	"context"
	"fmt"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

// messageSender is the minimal interface needed for AI analysis.
type messageSender interface {
	SendMessage(ctx context.Context, message string) (string, error)
}

const (
	// DefaultAnalysisTimeout is the default timeout for AI analysis requests.
	defaultAnalysisTimeout = 120

	// MaxOutputLength is the maximum number of characters to send to the AI provider.
	// This prevents sending excessively large outputs that exceed token limits.
	maxOutputLength = 50000

	// Newline constant for string building.
	newline = "\n"
)

// clientFactory creates AI clients. Overridden in tests for mocking.
var clientFactory func(cfg *schema.AtmosConfiguration) (messageSender, error) = func(cfg *schema.AtmosConfiguration) (messageSender, error) {
	return ai.NewClient(cfg)
}

// systemPrompt is the system prompt for AI analysis of command output.
const systemPrompt = `You are Atmos AI, an expert in infrastructure-as-code, DevOps, and cloud infrastructure.
Your task is to analyze the output of an Atmos CLI command and provide helpful insights.

Guidelines:
- If the command succeeded, provide a brief, clear summary of the output and key observations.
- If the command failed or produced errors, explain what went wrong and provide actionable steps to fix it.
- For Terraform plan output, highlight important changes (resources to add/change/destroy).
- For validation errors, explain the root cause and suggest corrections.
- Be concise and actionable. Focus on what the user needs to know.
- Use markdown formatting for readability.
- Do not repeat the full command output back to the user.`

// ValidateAIConfig checks that AI is properly configured for the --ai flag.
// Returns a user-friendly error with hints if configuration is missing.
func ValidateAIConfig(atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(nil, "analyze.ValidateAIConfig")()

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
			WithHintf("Add a provider configuration to your atmos.yaml:\n\n```yaml\nai:\n  enabled: true\n  default_provider: %s\n  providers:\n    %s:\n      model: claude-sonnet-4-5-20250514\n      api_key: !env ANTHROPIC_API_KEY\n```", provider, provider).
			Err()
	}

	if providerConfig.ApiKey == "" {
		return errUtils.Build(errUtils.ErrAIAPIKeyNotFound).
			WithExplanationf("The --ai flag requires an API key for provider %q, but none was found.", provider).
			WithHintf("Set the API key in your atmos.yaml using the !env function:\n\n```yaml\nai:\n  providers:\n    %s:\n      api_key: !env ANTHROPIC_API_KEY\n```", provider).
			WithHint("Or set the environment variable directly: export ANTHROPIC_API_KEY=your-key").
			Err()
	}

	return nil
}

// AnalyzeOutput sends captured command output to the configured AI provider for analysis.
// It creates an AI client, builds a prompt with the command context, and renders the response.
func AnalyzeOutput(atmosConfig *schema.AtmosConfiguration, commandName string, stdout, stderr string, cmdErr error) {
	defer perf.Track(nil, "analyze.AnalyzeOutput")()

	// Build the analysis prompt.
	prompt := buildAnalysisPrompt(commandName, stdout, stderr, cmdErr)
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

	// Show thinking indicator.
	ui.Info("Analyzing with AI...")

	// Send to AI provider.
	response, err := client.SendMessage(ctx, prompt)
	if err != nil {
		log.Error("AI analysis failed", "error", err)
		ui.Warningf("AI analysis failed: %s", err.Error())
		return
	}

	// Render AI response as markdown.
	ui.Markdown(response)
}

// buildAnalysisPrompt constructs the prompt for AI analysis.
func buildAnalysisPrompt(commandName, stdout, stderr string, cmdErr error) string {
	defer perf.Track(nil, "analyze.buildAnalysisPrompt")()

	// Truncate output if too large.
	stdout = truncateOutput(stdout)
	stderr = truncateOutput(stderr)

	// Skip if there's nothing meaningful to analyze.
	if strings.TrimSpace(stdout) == "" && strings.TrimSpace(stderr) == "" && cmdErr == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(systemPrompt)
	b.WriteString("\n\n---\n\n")

	// Command info.
	fmt.Fprintf(&b, "**Command:** `%s`\n\n", commandName)

	// Error status.
	if cmdErr != nil {
		fmt.Fprintf(&b, "**Status:** Failed with error: %s\n\n", cmdErr.Error())
	} else {
		b.WriteString("**Status:** Success\n\n")
	}

	// Stdout.
	if strings.TrimSpace(stdout) != "" {
		b.WriteString("**Standard Output:**\n```\n")
		b.WriteString(stdout)
		if !strings.HasSuffix(stdout, newline) {
			b.WriteString(newline)
		}
		b.WriteString("```\n\n")
	}

	// Stderr.
	if strings.TrimSpace(stderr) != "" {
		b.WriteString("**Standard Error:**\n```\n")
		b.WriteString(stderr)
		if !strings.HasSuffix(stderr, newline) {
			b.WriteString(newline)
		}
		b.WriteString("```\n\n")
	}

	// Instructions based on error status.
	if cmdErr != nil {
		b.WriteString("Please analyze the error output above. Explain what went wrong and provide step-by-step instructions to fix it.\n")
	} else {
		b.WriteString("Please provide a concise summary and analysis of the command output above.\n")
	}

	return b.String()
}

// truncateOutput limits output length to prevent exceeding AI token limits.
func truncateOutput(output string) string {
	if len(output) <= maxOutputLength {
		return output
	}
	return output[:maxOutputLength] + "\n... (output truncated)"
}
