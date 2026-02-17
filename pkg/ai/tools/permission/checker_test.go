package permission

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	errUtils "github.com/cloudposse/atmos/errors"
)

// MockTool is a mock implementation of the Tool interface.
type MockTool struct {
	name        string
	description string
	restricted  bool
}

func (m *MockTool) Name() string {
	return m.name
}

func (m *MockTool) Description() string {
	return m.description
}

func (m *MockTool) IsRestricted() bool {
	return m.restricted
}

// MockPrompter is a mock implementation of the Prompter interface.
type MockPrompter struct {
	mock.Mock
}

func (m *MockPrompter) Prompt(ctx context.Context, tool Tool, params map[string]interface{}) (bool, error) {
	args := m.Called(ctx, tool, params)
	return args.Bool(0), args.Error(1)
}

func TestNewChecker(t *testing.T) {
	tests := []struct {
		name         string
		config       *Config
		prompter     Prompter
		wantMode     Mode
		wantPrompter bool
	}{
		{
			name: "creates checker with config",
			config: &Config{
				Mode: ModeAllow,
			},
			prompter:     &MockPrompter{},
			wantMode:     ModeAllow,
			wantPrompter: true,
		},
		{
			name:         "uses default config when nil",
			config:       nil,
			prompter:     &MockPrompter{},
			wantMode:     ModePrompt,
			wantPrompter: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			checker := NewChecker(tt.config, tt.prompter)

			assert.NotNil(t, checker)
			assert.Equal(t, tt.wantMode, checker.config.Mode)
			if tt.wantPrompter {
				assert.NotNil(t, checker.prompter)
			} else {
				assert.Nil(t, checker.prompter)
			}
		})
	}
}

func TestChecker_CheckPermission_YOLOMode(t *testing.T) {
	config := &Config{
		Mode:     ModePrompt,
		YOLOMode: true, // Overrides everything
	}
	checker := NewChecker(config, nil)
	ctx := context.Background()

	tool := &MockTool{name: "test_tool"}

	// YOLO mode should allow everything.
	allowed, err := checker.CheckPermission(ctx, tool, nil)
	assert.NoError(t, err)
	assert.True(t, allowed)
}

func TestChecker_CheckPermission_BlockedTool(t *testing.T) {
	config := &Config{
		Mode:         ModePrompt,
		BlockedTools: []string{"dangerous_*", "blocked_tool"},
	}
	checker := NewChecker(config, nil)
	ctx := context.Background()

	tests := []struct {
		name     string
		toolName string
		wantErr  bool
		errIs    error
	}{
		{
			name:     "blocks exact match",
			toolName: "blocked_tool",
			wantErr:  true,
			errIs:    errUtils.ErrAIToolBlocked,
		},
		{
			name:     "blocks wildcard match",
			toolName: "dangerous_operation",
			wantErr:  true,
			errIs:    errUtils.ErrAIToolBlocked,
		},
		{
			name:     "allows non-blocked tool",
			toolName: "safe_tool",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &MockTool{name: tt.toolName}
			prompter := new(MockPrompter)

			// For non-blocked tools, expect prompt.
			if !tt.wantErr {
				prompter.On("Prompt", ctx, tool, mock.Anything).Return(true, nil)
				checker.prompter = prompter
			}

			allowed, err := checker.CheckPermission(ctx, tool, nil)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
				assert.False(t, allowed)
			} else {
				assert.NoError(t, err)
				assert.True(t, allowed)
			}
		})
	}
}

func TestChecker_CheckPermission_ModeAllow(t *testing.T) {
	config := &Config{
		Mode: ModeAllow,
	}
	checker := NewChecker(config, nil)
	ctx := context.Background()

	tool := &MockTool{name: "any_tool"}

	// Allow mode should allow all tools.
	allowed, err := checker.CheckPermission(ctx, tool, nil)
	assert.NoError(t, err)
	assert.True(t, allowed)
}

func TestChecker_CheckPermission_ModeDeny(t *testing.T) {
	config := &Config{
		Mode: ModeDeny,
	}
	checker := NewChecker(config, nil)
	ctx := context.Background()

	tool := &MockTool{name: "any_tool"}

	// Deny mode should deny all tools.
	allowed, err := checker.CheckPermission(ctx, tool, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAIToolsDisabled)
	assert.False(t, allowed)
}

func TestChecker_CheckPermission_AllowedList(t *testing.T) {
	config := &Config{
		Mode:         ModePrompt,
		AllowedTools: []string{"atmos_*", "safe_tool"},
	}
	checker := NewChecker(config, nil) // No prompter needed for allowed tools.
	ctx := context.Background()

	tests := []struct {
		name     string
		toolName string
		wantErr  bool
	}{
		{
			name:     "allows exact match",
			toolName: "safe_tool",
			wantErr:  false,
		},
		{
			name:     "allows wildcard match",
			toolName: "atmos_describe_component",
			wantErr:  false,
		},
		{
			name:     "requires prompt for non-allowed",
			toolName: "other_tool",
			wantErr:  true, // Will error because no prompter
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &MockTool{name: tt.toolName}

			allowed, err := checker.CheckPermission(ctx, tool, nil)

			if tt.wantErr {
				assert.Error(t, err)
				assert.False(t, allowed)
			} else {
				assert.NoError(t, err)
				assert.True(t, allowed)
			}
		})
	}
}

func TestChecker_CheckPermission_RestrictedTools(t *testing.T) {
	tests := []struct {
		name             string
		toolName         string
		toolRestricted   bool
		restrictedList   []string
		prompterResponse bool
		prompterError    error
		expectPrompt     bool
		expectAllowed    bool
		expectErr        bool
	}{
		{
			name:             "prompts for tool marked restricted",
			toolName:         "sensitive_tool",
			toolRestricted:   true,
			restrictedList:   []string{},
			prompterResponse: true,
			expectPrompt:     true,
			expectAllowed:    true,
			expectErr:        false,
		},
		{
			name:             "prompts for tool in restricted list",
			toolName:         "admin_operation",
			toolRestricted:   false,
			restrictedList:   []string{"admin_*"},
			prompterResponse: false,
			expectPrompt:     true,
			expectAllowed:    false,
			expectErr:        false,
		},
		{
			name:             "user denies restricted tool",
			toolName:         "sensitive_tool",
			toolRestricted:   true,
			prompterResponse: false,
			expectPrompt:     true,
			expectAllowed:    false,
			expectErr:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &Config{
				Mode:            ModePrompt,
				RestrictedTools: tt.restrictedList,
			}

			prompter := new(MockPrompter)
			checker := NewChecker(config, prompter)
			ctx := context.Background()

			tool := &MockTool{
				name:       tt.toolName,
				restricted: tt.toolRestricted,
			}

			if tt.expectPrompt {
				prompter.On("Prompt", ctx, tool, mock.Anything).Return(tt.prompterResponse, tt.prompterError)
			}

			allowed, err := checker.CheckPermission(ctx, tool, nil)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectAllowed, allowed)

			if tt.expectPrompt {
				prompter.AssertExpectations(t)
			}
		})
	}
}

func TestChecker_CheckPermission_DefaultBehavior(t *testing.T) {
	config := &Config{
		Mode: ModePrompt,
	}

	prompter := new(MockPrompter)
	checker := NewChecker(config, prompter)
	ctx := context.Background()

	tool := &MockTool{name: "normal_tool"}
	params := map[string]interface{}{"key": "value"}

	// Default behavior: prompt user.
	prompter.On("Prompt", ctx, tool, params).Return(true, nil)

	allowed, err := checker.CheckPermission(ctx, tool, params)
	assert.NoError(t, err)
	assert.True(t, allowed)

	prompter.AssertExpectations(t)
}

func TestChecker_CheckPermission_NoPrompter(t *testing.T) {
	config := &Config{
		Mode: ModePrompt,
	}

	checker := NewChecker(config, nil) // No prompter
	ctx := context.Background()

	tool := &MockTool{name: "some_tool"}

	// Should error when prompt is needed but no prompter available.
	allowed, err := checker.CheckPermission(ctx, tool, nil)
	assert.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrAINoPrompter)
	assert.False(t, allowed)
}

func TestChecker_CheckPermission_PrompterError(t *testing.T) {
	config := &Config{
		Mode: ModePrompt,
	}

	prompter := new(MockPrompter)
	checker := NewChecker(config, prompter)
	ctx := context.Background()

	tool := &MockTool{name: "some_tool"}
	expectedErr := assert.AnError

	prompter.On("Prompt", ctx, tool, mock.Anything).Return(false, expectedErr)

	allowed, err := checker.CheckPermission(ctx, tool, nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "prompt failed")
	assert.False(t, allowed)

	prompter.AssertExpectations(t)
}

func TestMatchesPattern(t *testing.T) {
	tests := []struct {
		name     string
		toolName string
		pattern  string
		want     bool
	}{
		{
			name:     "exact match",
			toolName: "atmos_describe",
			pattern:  "atmos_describe",
			want:     true,
		},
		{
			name:     "exact no match",
			toolName: "atmos_describe",
			pattern:  "atmos_list",
			want:     false,
		},
		{
			name:     "wildcard prefix match",
			toolName: "atmos_describe_component",
			pattern:  "atmos_*",
			want:     true,
		},
		{
			name:     "wildcard no match",
			toolName: "other_tool",
			pattern:  "atmos_*",
			want:     false,
		},
		{
			name:     "wildcard matches empty suffix",
			toolName: "atmos_",
			pattern:  "atmos_*",
			want:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesPattern(tt.toolName, tt.pattern)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestChecker_CheckPermission_ComplexScenario(t *testing.T) {
	// Complex scenario: allowed list, restricted list, and blocked list.
	config := &Config{
		Mode:            ModePrompt,
		AllowedTools:    []string{"atmos_*"},
		RestrictedTools: []string{"atmos_validate_*"},
		BlockedTools:    []string{"atmos_delete_*"},
	}

	prompter := new(MockPrompter)
	checker := NewChecker(config, prompter)
	ctx := context.Background()

	tests := []struct {
		name          string
		toolName      string
		expectPrompt  bool
		promptResult  bool
		expectAllowed bool
		expectErr     bool
		errIs         error
	}{
		{
			name:          "blocked tool is blocked",
			toolName:      "atmos_delete_stack",
			expectPrompt:  false,
			expectAllowed: false,
			expectErr:     true,
			errIs:         errUtils.ErrAIToolBlocked,
		},
		{
			name:          "allowed tool is allowed without prompt",
			toolName:      "atmos_describe_component",
			expectPrompt:  false,
			expectAllowed: true,
			expectErr:     false,
		},
		{
			name:          "tool in both allowed and restricted lists - allowed takes precedence",
			toolName:      "atmos_validate_stacks",
			expectPrompt:  false, // Allowed list is checked first, so no prompt
			promptResult:  false,
			expectAllowed: true,
			expectErr:     false,
		},
		{
			name:          "tool not in any list - default behavior prompts user who denies",
			toolName:      "other_tool",
			expectPrompt:  true, // Not in any list, default behavior is to prompt
			promptResult:  false,
			expectAllowed: false,
			expectErr:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &MockTool{name: tt.toolName}

			if tt.expectPrompt {
				prompter.On("Prompt", ctx, tool, mock.Anything).Return(tt.promptResult, nil).Once()
			}

			allowed, err := checker.CheckPermission(ctx, tool, nil)

			if tt.expectErr {
				assert.Error(t, err)
				if tt.errIs != nil {
					assert.ErrorIs(t, err, tt.errIs)
				}
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectAllowed, allowed)
		})
	}

	prompter.AssertExpectations(t)
}
