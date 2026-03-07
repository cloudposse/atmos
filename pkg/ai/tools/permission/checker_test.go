package permission

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

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

func TestChecker_CheckPermission_ModeYOLO(t *testing.T) {
	config := &Config{
		Mode: ModeYOLO,
	}
	checker := NewChecker(config, nil)
	ctx := context.Background()

	tool := &MockTool{name: "any_tool"}

	// YOLO mode (via Mode field) should allow everything.
	allowed, err := checker.CheckPermission(ctx, tool, nil)
	assert.NoError(t, err)
	assert.True(t, allowed)
}

// simulateStdin replaces os.Stdin with a pipe that feeds the given input,
// calls f, then restores os.Stdin.
func simulateStdin(t *testing.T, input string, f func()) {
	t.Helper()

	r, w, err := os.Pipe()
	require.NoError(t, err)

	_, err = io.WriteString(w, input)
	require.NoError(t, err)
	w.Close()

	old := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = old
		r.Close()
	}()

	f()
}

func TestNewCLIPrompter(t *testing.T) {
	p := NewCLIPrompter()
	assert.NotNil(t, p)
	assert.Nil(t, p.cache)
}

func TestNewCLIPrompterWithCache(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	assert.NotNil(t, p)
	assert.Equal(t, cache, p.cache)
}

func TestCLIPrompter_checkCachedPermission_NoCache(t *testing.T) {
	p := NewCLIPrompter()

	// No cache means no cached decision.
	decision, found := p.checkCachedPermission("test_tool")
	assert.False(t, found)
	assert.False(t, decision)
}

func TestCLIPrompter_checkCachedPermission_Allowed(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	err = cache.AddAllow("test_tool")
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	decision, found := p.checkCachedPermission("test_tool")
	assert.True(t, found)
	assert.True(t, decision)
}

func TestCLIPrompter_checkCachedPermission_Denied(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	err = cache.AddDeny("test_tool")
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	decision, found := p.checkCachedPermission("test_tool")
	assert.True(t, found)
	assert.False(t, decision)
}

func TestCLIPrompter_checkCachedPermission_NotInCache(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	decision, found := p.checkCachedPermission("unknown_tool")
	assert.False(t, found)
	assert.False(t, decision)
}

func TestCLIPrompter_displayPrompt_NoCache(t *testing.T) {
	p := NewCLIPrompter()
	tool := &MockTool{name: "test_tool", description: "A test tool."}

	// Capture stderr by temporarily redirecting it.
	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStderr := os.Stderr
	os.Stderr = w

	p.displayPrompt(tool, nil)

	w.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	r.Close()

	output := buf.String()
	assert.Contains(t, output, "test_tool")
	assert.Contains(t, output, "A test tool.")
	assert.Contains(t, output, "y/N")
}

func TestCLIPrompter_displayPrompt_WithCache(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	params := map[string]interface{}{"key": "value"}

	// Capture stderr.
	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStderr := os.Stderr
	os.Stderr = w

	p.displayPrompt(tool, params)

	w.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	r.Close()

	output := buf.String()
	assert.Contains(t, output, "test_tool")
	assert.Contains(t, output, "Always allow")
	assert.Contains(t, output, "a/y/n/d")
	// Parameters section should be printed.
	assert.Contains(t, output, "key")
}

func TestCLIPrompter_displayPrompt_NoParams(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "test_tool", description: "A test tool."}

	// Capture stderr.
	r, w, err := os.Pipe()
	require.NoError(t, err)

	oldStderr := os.Stderr
	os.Stderr = w

	p.displayPrompt(tool, map[string]interface{}{})

	w.Close()
	os.Stderr = oldStderr

	var buf strings.Builder
	_, err = io.Copy(&buf, r)
	require.NoError(t, err)
	r.Close()

	output := buf.String()
	// Empty params map - Parameters section should not be printed.
	assert.NotContains(t, output, "Parameters:")
}

func TestCLIPrompter_handleCachedResponse_Always(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	// Capture stderr to suppress output.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	result := p.handleCachedResponse("a", "test_tool")

	w.Close()
	os.Stderr = oldStderr
	r.Close()

	assert.True(t, result)
	assert.True(t, cache.IsAllowed("test_tool"))
}

func TestCLIPrompter_handleCachedResponse_AlwaysLong(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	result := p.handleCachedResponse("always", "test_tool2")

	w.Close()
	os.Stderr = oldStderr
	r.Close()

	assert.True(t, result)
	assert.True(t, cache.IsAllowed("test_tool2"))
}

func TestCLIPrompter_handleCachedResponse_Yes(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	result := p.handleCachedResponse("y", "test_tool")
	assert.True(t, result)
	// Should NOT be in cache (allow once, not persisted).
	assert.False(t, cache.IsAllowed("test_tool"))
}

func TestCLIPrompter_handleCachedResponse_YesLong(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	result := p.handleCachedResponse("yes", "test_tool")
	assert.True(t, result)
	assert.False(t, cache.IsAllowed("test_tool"))
}

func TestCLIPrompter_handleCachedResponse_Deny(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	result := p.handleCachedResponse("d", "test_tool")

	w.Close()
	os.Stderr = oldStderr
	r.Close()

	assert.False(t, result)
	assert.True(t, cache.IsDenied("test_tool"))
}

func TestCLIPrompter_handleCachedResponse_DenyLong(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	result := p.handleCachedResponse("deny", "test_tool2")

	w.Close()
	os.Stderr = oldStderr
	r.Close()

	assert.False(t, result)
	assert.True(t, cache.IsDenied("test_tool2"))
}

func TestCLIPrompter_handleCachedResponse_Default(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	result := p.handleCachedResponse("n", "test_tool")
	assert.False(t, result)

	result = p.handleCachedResponse("no", "test_tool")
	assert.False(t, result)

	result = p.handleCachedResponse("", "test_tool")
	assert.False(t, result)
}

func TestCLIPrompter_Prompt_CachedAllowed(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	err = cache.AddAllow("cached_tool")
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "cached_tool"}
	ctx := context.Background()

	// Should return cached result without prompting.
	allowed, err := p.Prompt(ctx, tool, nil)
	assert.NoError(t, err)
	assert.True(t, allowed)
}

func TestCLIPrompter_Prompt_CachedDenied(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	err = cache.AddDeny("denied_tool")
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "denied_tool"}
	ctx := context.Background()

	// Should return cached denial without prompting.
	allowed, err := p.Prompt(ctx, tool, nil)
	assert.NoError(t, err)
	assert.False(t, allowed)
}

func TestCLIPrompter_Prompt_NoCache_Yes(t *testing.T) {
	p := NewCLIPrompter()
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "y\n", func() {
		// Suppress stderr output.
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.True(t, allowed)
}

func TestCLIPrompter_Prompt_NoCache_No(t *testing.T) {
	p := NewCLIPrompter()
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "n\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.False(t, allowed)
}

func TestCLIPrompter_Prompt_NoCache_YesLong(t *testing.T) {
	p := NewCLIPrompter()
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "yes\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.True(t, allowed)
}

func TestCLIPrompter_Prompt_NoCache_Default(t *testing.T) {
	p := NewCLIPrompter()
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.False(t, allowed)
}

func TestCLIPrompter_Prompt_WithCache_Always(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "new_tool", description: "A new tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "a\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.True(t, allowed)
	// Tool should be saved in cache.
	assert.True(t, cache.IsAllowed("new_tool"))
}

func TestCLIPrompter_Prompt_WithCache_DenyPersistent(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "new_tool", description: "A new tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "d\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.False(t, allowed)
	// Tool should be saved in deny cache.
	assert.True(t, cache.IsDenied("new_tool"))
}

func TestCLIPrompter_Prompt_WithParams(t *testing.T) {
	p := NewCLIPrompter()
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	ctx := context.Background()
	params := map[string]interface{}{"component": "vpc", "stack": "dev"}

	var allowed bool
	var promptErr error

	simulateStdin(t, "y\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, params)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.True(t, allowed)
}

func TestCLIPrompter_Prompt_ReadError(t *testing.T) {
	p := NewCLIPrompter()
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	ctx := context.Background()

	// Create a pipe but close the write end immediately to cause EOF/error.
	r, w, err := os.Pipe()
	require.NoError(t, err)
	w.Close() // Close write end so reads return EOF.

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		r.Close()
	}()

	// Suppress stderr output.
	sr, sw, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = sw

	_, promptErr := p.Prompt(ctx, tool, nil)

	sw.Close()
	os.Stderr = oldStderr
	sr.Close()

	// EOF results in an error from ReadString.
	assert.Error(t, promptErr)
	assert.Contains(t, promptErr.Error(), "failed to read input")
}

func TestCLIPrompter_Prompt_WithCache_OnceAllow(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "once_tool", description: "A once tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "y\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.True(t, allowed)
	// "y" (allow once) should NOT be persisted.
	assert.False(t, cache.IsAllowed("once_tool"))
}

// TestCLIPrompter_Prompt_WithCache_OnceDeny tests "n" (deny once) with cache.
func TestCLIPrompter_Prompt_WithCache_OnceDeny(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "once_tool", description: "A once tool."}
	ctx := context.Background()

	var allowed bool
	var promptErr error

	simulateStdin(t, "n\n", func() {
		r, w, err := os.Pipe()
		require.NoError(t, err)
		oldStderr := os.Stderr
		os.Stderr = w

		allowed, promptErr = p.Prompt(ctx, tool, nil)

		w.Close()
		os.Stderr = oldStderr
		r.Close()
	})

	assert.NoError(t, promptErr)
	assert.False(t, allowed)
	// "n" (deny once) should NOT be persisted.
	assert.False(t, cache.IsDenied("once_tool"))
}

// TestCLIPrompter_Prompt_WithCache_ReadError tests that read errors are propagated.
func TestCLIPrompter_Prompt_WithCache_ReadError(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)
	tool := &MockTool{name: "test_tool", description: "A test tool."}
	ctx := context.Background()

	r, w, err := os.Pipe()
	require.NoError(t, err)
	w.Close() // Immediately close the write end to cause EOF.

	oldStdin := os.Stdin
	os.Stdin = r
	defer func() {
		os.Stdin = oldStdin
		r.Close()
	}()

	sr, sw, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = sw

	_, promptErr := p.Prompt(ctx, tool, nil)

	sw.Close()
	os.Stderr = oldStderr
	sr.Close()

	assert.Error(t, promptErr)
	assert.Contains(t, promptErr.Error(), "failed to read input")
}

// TestCLIPrompter_handleCachedResponse_SaveError verifies that when AddAllow/AddDeny
// fails (e.g. because the file path is a directory), a warning is printed but
// the function still returns the expected result.
func TestCLIPrompter_handleCachedResponse_SaveError(t *testing.T) {
	tests := []struct {
		name           string
		response       string
		expectedResult bool
	}{
		{"allow_error", "a", true},
		{"deny_error", "d", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			cache, err := NewPermissionCache(tmpDir)
			require.NoError(t, err)

			// Make filePath point to a directory so save() fails.
			dirPath := filepath.Join(tmpDir, ".atmos", "unwritable_"+tt.name)
			err = os.MkdirAll(dirPath, 0o755)
			require.NoError(t, err)
			cache.filePath = dirPath

			p := NewCLIPrompterWithCache(cache)

			r, w, err := os.Pipe()
			require.NoError(t, err)
			oldStderr := os.Stderr
			os.Stderr = w

			result := p.handleCachedResponse(tt.response, "fail_tool")

			w.Close()
			os.Stderr = oldStderr

			var buf strings.Builder
			_, err = io.Copy(&buf, r)
			require.NoError(t, err)
			r.Close()

			assert.Equal(t, tt.expectedResult, result)
			assert.Contains(t, buf.String(), "Warning")
		})
	}
}

func TestCLIPrompter_handleCachedResponse_AddAllow_Duplicate(t *testing.T) {
	tmpDir := t.TempDir()
	cache, err := NewPermissionCache(tmpDir)
	require.NoError(t, err)

	// Pre-add the tool to allow list.
	err = cache.AddAllow("dup_tool")
	require.NoError(t, err)

	p := NewCLIPrompterWithCache(cache)

	r, w, err := os.Pipe()
	require.NoError(t, err)
	oldStderr := os.Stderr
	os.Stderr = w

	// Adding again via "a" should not error (duplicate handling).
	result := p.handleCachedResponse("a", "dup_tool")

	w.Close()
	os.Stderr = oldStderr
	r.Close()

	assert.True(t, result)
	// Still only one entry in allow list.
	allowList := cache.GetAllowList()
	count := 0
	for _, item := range allowList {
		if item == "dup_tool" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}
