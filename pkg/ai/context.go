package ai

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
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

// GatherStackContext reads stack configurations and returns them as context for AI.
func GatherStackContext(atmosConfig *schema.AtmosConfiguration) (string, error) {
	var context strings.Builder

	// Try automatic context discovery first if enabled.
	if atmosConfig.Settings.AI.Context.Enabled {
		log.Debug("Using automatic context discovery")

		discoverer, err := aiContext.NewDiscoverer(atmosConfig.BasePath, atmosConfig.Settings.AI.Context)
		if err != nil {
			log.Warn(fmt.Sprintf("Failed to create context discoverer: %v", err))
		} else {
			result, err := discoverer.Discover()
			if err != nil {
				log.Warn(fmt.Sprintf("Failed to discover context files: %v", err))
			} else if len(result.Files) > 0 {
				// Use discovered files as context.
				discoveredContext := aiContext.FormatFilesContext(result)
				if discoveredContext != "" {
					context.WriteString(discoveredContext)

					// Show file list if configured.
					if atmosConfig.Settings.AI.Context.ShowFiles {
						log.Info(fmt.Sprintf("Auto-discovered %d files for context", len(result.Files)))
						for _, file := range result.Files {
							log.Debug(fmt.Sprintf("  - %s (%d bytes)", file.RelativePath, file.Size))
						}
					}

					// Return early if we got discovered context.
					return context.String(), nil
				}
			}
		}
	}

	// Fallback to original stack file gathering.
	log.Debug("Using legacy stack file context gathering")

	// Get stacks path.
	stacksPath := filepath.Join(atmosConfig.BasePath, atmosConfig.Stacks.BasePath)

	// Find stack files.
	stackFiles, err := findStackFiles(stacksPath)
	if err != nil {
		return "", err
	}

	// Limit the number of files to prevent overwhelming the AI.
	// Use configured value or default.
	maxFiles := DefaultMaxContextFiles
	if atmosConfig.Settings.AI.MaxContextFiles > 0 {
		maxFiles = atmosConfig.Settings.AI.MaxContextFiles
	}

	if len(stackFiles) > maxFiles {
		stackFiles = stackFiles[:maxFiles]
		context.WriteString(fmt.Sprintf("Note: Showing first %d stack files (out of %d total)\n\n", maxFiles, len(stackFiles)))
	}

	context.WriteString("=== Atmos Stack Configurations ===\n\n")

	// Get max lines configuration.
	maxLines := DefaultMaxContextLines
	if atmosConfig.Settings.AI.MaxContextLines > 0 {
		maxLines = atmosConfig.Settings.AI.MaxContextLines
	}

	for _, file := range stackFiles {
		relPath, _ := filepath.Rel(atmosConfig.BasePath, file)
		context.WriteString(fmt.Sprintf("File: %s\n", relPath))
		context.WriteString("```yaml\n")

		content := formatFileContent(file, maxLines)
		context.WriteString(content)

		context.WriteString("\n```\n\n")
	}

	return context.String(), nil
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
	if atmosConfig.Settings.AI.SendContext {
		// If prompt_on_send is true, ask for confirmation each time.
		if atmosConfig.Settings.AI.PromptOnSend {
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

// SanitizeContext removes sensitive information from context before sending to AI.
func SanitizeContext(context string) string {
	// This is a basic implementation - can be enhanced based on security requirements.
	// For now, we'll add warnings about sensitive data.

	var sanitized strings.Builder

	sanitized.WriteString("⚠️  PRIVACY NOTE: The following configurations are being sent to the AI provider.\n")
	sanitized.WriteString("Ensure no sensitive data (API keys, passwords, secrets) are included.\n")
	sanitized.WriteString("========================================\n\n")
	sanitized.WriteString(context)

	return sanitized.String()
}
