package permission

import (
	"context"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
)

// Tool interface (minimal interface needed for permission checking).
type Tool interface {
	Name() string
	Description() string
	IsRestricted() bool
}

// Checker handles permission checking for tool execution.
type Checker struct {
	config   *Config
	prompter Prompter
}

// Prompter defines the interface for user prompting.
type Prompter interface {
	// Prompt asks the user for permission to execute a tool.
	Prompt(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error)
}

// NewChecker creates a new permission checker.
func NewChecker(config *Config, prompter Prompter) *Checker {
	if config == nil {
		config = &Config{
			Mode: ModePrompt,
		}
	}

	return &Checker{
		config:   config,
		prompter: prompter,
	}
}

// CheckPermission checks if a tool can be executed.
func (c *Checker) CheckPermission(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	toolName := tool.Name()

	// YOLO mode - allow everything (dangerous!).
	if c.config.YOLOMode {
		return true, nil
	}

	// Check if tool is blocked.
	if c.isBlocked(toolName) {
		return false, fmt.Errorf("%w: %s", errUtils.ErrAIToolBlocked, toolName)
	}

	// Check global mode.
	switch c.config.Mode {
	case ModeAllow:
		return true, nil

	case ModeDeny:
		return false, errUtils.ErrAIToolsDisabled

	case ModeYOLO:
		return true, nil

	case ModePrompt:
		// Continue to specific checks.
	}

	// Check if tool is in allowed list (auto-allow).
	if c.isAllowed(toolName) {
		return true, nil
	}

	// Check if tool is restricted or requires permission.
	if c.isRestricted(toolName) || tool.IsRestricted() {
		// Always prompt for restricted tools.
		return c.promptUser(ctx, tool, params)
	}

	// Default: prompt user.
	return c.promptUser(ctx, tool, params)
}

// isAllowed checks if a tool is in the allowed list.
func (c *Checker) isAllowed(toolName string) bool {
	for _, pattern := range c.config.AllowedTools {
		if matchesPattern(toolName, pattern) {
			return true
		}
	}
	return false
}

// isRestricted checks if a tool is in the restricted list.
func (c *Checker) isRestricted(toolName string) bool {
	for _, pattern := range c.config.RestrictedTools {
		if matchesPattern(toolName, pattern) {
			return true
		}
	}
	return false
}

// isBlocked checks if a tool is in the blocked list.
func (c *Checker) isBlocked(toolName string) bool {
	for _, pattern := range c.config.BlockedTools {
		if matchesPattern(toolName, pattern) {
			return true
		}
	}
	return false
}

// promptUser prompts the user for permission.
func (c *Checker) promptUser(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	if c.prompter == nil {
		// No prompter available - default to deny for safety.
		return false, fmt.Errorf("%w: %s", errUtils.ErrAINoPrompter, tool.Name())
	}

	allowed, err := c.prompter.Prompt(ctx, tool, params)
	if err != nil {
		return false, fmt.Errorf("prompt failed: %w", err)
	}

	return allowed, nil
}

// matchesPattern checks if a tool name matches a pattern (supports wildcards).
func matchesPattern(toolName, pattern string) bool {
	// Simple wildcard matching: "atmos_*" matches "atmos_describe_component".
	if strings.HasSuffix(pattern, "*") {
		prefix := strings.TrimSuffix(pattern, "*")
		return strings.HasPrefix(toolName, prefix)
	}

	// Exact match.
	return toolName == pattern
}
