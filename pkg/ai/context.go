package ai

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	aiContext "github.com/cloudposse/atmos/pkg/ai/context"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// DefaultMaxContextFiles is the default maximum number of stack files to include in AI context.
	DefaultMaxContextFiles = 10
	// DefaultMaxContextLines is the default maximum number of lines per stack file in AI context.
	DefaultMaxContextLines = 500
)

// findStackFiles searches for stack configuration files in the given path.
func findStackFiles(stacksPath string) ([]string, error) {
	stackFiles, err := filepath.Glob(stacksPath + string(filepath.Separator) + "**/*.yaml")
	if err != nil {
		return nil, fmt.Errorf("failed to find stack files: %w", err)
	}

	// Add more patterns for common stack file locations.
	yamlFiles, err := filepath.Glob(stacksPath + string(filepath.Separator) + "*.yaml")
	if err == nil {
		stackFiles = append(stackFiles, yamlFiles...)
	}

	ymlFiles, err := filepath.Glob(stacksPath + string(filepath.Separator) + "**/*.yml")
	if err == nil {
		stackFiles = append(stackFiles, ymlFiles...)
	}

	if len(stackFiles) == 0 {
		return nil, fmt.Errorf("%w in %s", errUtils.ErrAINoStackFilesFound, stacksPath)
	}

	return stackFiles, nil
}

// formatFileContent reads and formats a stack file with optional line truncation.
func formatFileContent(file string, maxLines int) string {
	content, err := os.ReadFile(file)
	if err != nil {
		return fmt.Sprintf("Error reading file: %v\n", err)
	}

	lines := strings.Split(string(content), "\n")
	if len(lines) > maxLines {
		truncated := strings.Join(lines[:maxLines], "\n")
		truncatedCount := len(lines) - maxLines
		return truncated + fmt.Sprintf("\n... (truncated, %d more lines)", truncatedCount)
	}

	return string(content)
}

// gatherDiscoveredContext attempts automatic context discovery and returns the context string.
// Returns empty string if discovery is not enabled, fails, or finds no files.
func gatherDiscoveredContext(atmosConfig *schema.AtmosConfiguration) string {
	if !atmosConfig.AI.Context.Enabled {
		return ""
	}

	log.Debug("Using automatic context discovery")

	discoverer, err := aiContext.NewDiscoverer(atmosConfig.BasePath, &atmosConfig.AI.Context)
	if err != nil {
		log.Warnf("Failed to create context discoverer: %v", err)
		return ""
	}

	result, err := discoverer.Discover()
	if err != nil {
		log.Warnf("Failed to discover context files: %v", err)
		return ""
	}

	if len(result.Files) == 0 {
		return ""
	}

	discoveredContext := aiContext.FormatFilesContext(result)
	if discoveredContext == "" {
		return ""
	}

	// Show file list if configured.
	if atmosConfig.AI.Context.ShowFiles {
		log.Infof("Auto-discovered %d files for context", len(result.Files))
		for _, file := range result.Files {
			log.Debugf("  - %s (%d bytes)", file.RelativePath, file.Size)
		}
	}

	return discoveredContext
}

// gatherLegacyStackContext reads stack files and formats them as context for AI.
func gatherLegacyStackContext(atmosConfig *schema.AtmosConfiguration) (string, error) {
	log.Debug("Using legacy stack file context gathering")

	// Get stacks path.
	stacksPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)

	// Find stack files.
	stackFiles, err := findStackFiles(stacksPath)
	if err != nil {
		return "", err
	}

	var context strings.Builder

	// Limit the number of files to prevent overwhelming the AI.
	// Use configured value or default.
	maxFiles := DefaultMaxContextFiles
	if atmosConfig.AI.MaxContextFiles > 0 {
		maxFiles = atmosConfig.AI.MaxContextFiles
	}

	if len(stackFiles) > maxFiles {
		stackFiles = stackFiles[:maxFiles]
		fmt.Fprintf(&context, "Note: Showing first %d stack files (out of %d total)\n\n", maxFiles, len(stackFiles))
	}

	context.WriteString("=== Atmos Stack Configurations ===\n\n")

	// Get max lines configuration.
	maxLines := DefaultMaxContextLines
	if atmosConfig.AI.MaxContextLines > 0 {
		maxLines = atmosConfig.AI.MaxContextLines
	}

	for _, file := range stackFiles {
		relPath, _ := filepath.Rel(atmosConfig.BasePath, file)
		fmt.Fprintf(&context, "File: %s\n", relPath)
		context.WriteString("```yaml\n")

		content := formatFileContent(file, maxLines)
		context.WriteString(content)

		context.WriteString("\n```\n\n")
	}

	return context.String(), nil
}

// GatherStackContext reads stack configurations and returns them as context for AI.
func GatherStackContext(atmosConfig *schema.AtmosConfiguration) (string, error) {
	// Try automatic context discovery first.
	if discoveredContext := gatherDiscoveredContext(atmosConfig); discoveredContext != "" {
		return discoveredContext, nil
	}

	// Fallback to original stack file gathering.
	return gatherLegacyStackContext(atmosConfig)
}

// PromptForConsent asks the user for consent to send context to AI.
func PromptForConsent() (bool, error) {
	fmt.Fprintf(os.Stderr, "\n⚠️  This question may benefit from your stack configurations.\n")
	fmt.Fprintf(os.Stderr, "📤 Send stack files to AI provider for analysis? (y/N): ")

	reader := bufio.NewReader(os.Stdin)
	response, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	response = strings.TrimSpace(strings.ToLower(response))
	return response == "y" || response == "yes", nil
}

// ShouldSendContext determines if context should be sent based on configuration and environment.
func ShouldSendContext(atmosConfig *schema.AtmosConfiguration, question string) (bool, bool, error) {
	// Check environment variable ATMOS_AI_SEND_CONTEXT using viper.
	_ = viper.BindEnv("ATMOS_AI_SEND_CONTEXT", "ATMOS_AI_SEND_CONTEXT")
	envVal := viper.GetString("ATMOS_AI_SEND_CONTEXT")
	if envVal != "" {
		sendContext := envVal == "true" || envVal == "1" || strings.ToLower(envVal) == "yes"
		return sendContext, false, nil // No prompt needed, env var takes precedence.
	}

	// Check atmos.yaml configuration.
	if atmosConfig.AI.SendContext {
		// If prompt_on_send is true, ask for confirmation each time.
		if atmosConfig.AI.PromptOnSend {
			consent, err := PromptForConsent()
			return consent, false, err
		}
		// Send context without prompting.
		return true, false, nil
	}

	// Check if the question seems to need context.
	needsContext := QuestionNeedsContext(question)
	if needsContext {
		// Prompt user for consent.
		consent, err := PromptForConsent()
		return consent, true, err
	}

	return false, false, nil
}

// QuestionNeedsContext determines if a question likely needs repository context.
func QuestionNeedsContext(question string) bool {
	question = strings.ToLower(question)

	contextKeywords := []string{
		"this repo",
		"my repo",
		"my stack",
		"these stacks",
		"my component",
		"these components",
		"my configuration",
		"my config",
		"what stacks",
		"list stacks",
		"show me",
		"describe the",
		"analyze",
		"review my",
	}

	for _, keyword := range contextKeywords {
		if strings.Contains(question, keyword) {
			return true
		}
	}

	return false
}

// ReductSecrets replaces sensitive information with a masked value.
func RedactSecrets(text string) string {
	// Common secret patterns
	patterns := map[string]string{
		// AWS Access Key ID (AKIA...)
		`\b(AKIA|ASIA)[A-Z0-9]{16}\b`: "$1****************",
		// GitHub Personal Access Token (classic and fine-grained)
		`\b(ghp|gho|ghu|ghs|ghr|github_pat)_[a-zA-Z0-9]{36,}\b`: "$1_****************",
		// Private Key Header
		`-----BEGIN [A-Z ]+ PRIVATE KEY-----`: "-----BEGIN *** PRIVATE KEY-----",
		// Generic "password", "secret", "token" in YAML/JSON (key: value)
		`(?i)(password|secret|token|api_key|access_key|secret_key)["']?\s*[:=]\s*["']?([^"'\s]+)["']?`: "$1: \"***MASKED***\"",
	}

	redacted := text
	for pattern, replacement := range patterns {
		re := regexp.MustCompile(pattern)
		redacted = re.ReplaceAllString(redacted, replacement)
	}
	return redacted
}

// SanitizeContext removes sensitive information from context before sending to AI.
func SanitizeContext(context string) string {
	var sanitized strings.Builder

	sanitized.WriteString("⚠️  PRIVACY NOTE: The following configurations are being sent to the AI provider.\n")
	sanitized.WriteString("Sensitive data (API keys, passwords, secrets) has been automatically redacted.\n")
	sanitized.WriteString("========================================\n\n")

	// Apply redaction rules
	safeContext := RedactSecrets(context)
	sanitized.WriteString(safeContext)

	return sanitized.String()
}
