package analyze

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	atmosErrors "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validAIConfig returns a minimal valid AI configuration for testing.
func validAIConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		AI: schema.AISettings{
			Enabled:         true,
			DefaultProvider: "anthropic",
			Providers: map[string]*schema.AIProviderConfig{
				"anthropic": {Model: "claude-sonnet-4-5-20250514", ApiKey: "sk-test-key"},
			},
		},
	}
}

// validAIConfigWithSkill returns a valid AI config with a custom skill defined.
func validAIConfigWithSkill() *schema.AtmosConfiguration {
	cfg := validAIConfig()
	cfg.AI.Skills = map[string]*schema.AISkillConfig{
		"test-skill": {
			DisplayName:  "Test Skill",
			Description:  "A test skill",
			SystemPrompt: "You are a test expert.",
		},
	}
	return cfg
}

// withMockCapture replaces startCaptureFunc with a mock for the duration of a test.
func withMockCapture(t *testing.T, cs *CaptureSession, err error) {
	t.Helper()
	original := startCaptureFunc
	startCaptureFunc = func() (*CaptureSession, error) {
		return cs, err
	}
	t.Cleanup(func() { startCaptureFunc = original })
}

// withMockAnalyzePtr replaces analyzeOutputFunc and returns a pointer to the captured input.
func withMockAnalyzePtr(t *testing.T) **AnalysisInput {
	t.Helper()
	original := analyzeOutputFunc
	var captured *AnalysisInput
	analyzeOutputFunc = func(_ *schema.AtmosConfiguration, input *AnalysisInput) {
		captured = input
	}
	t.Cleanup(func() { analyzeOutputFunc = original })
	return &captured
}

// withMockExit replaces exitFunc with a recorder for the duration of a test.
func withMockExit(t *testing.T) *int {
	t.Helper()
	original := exitFunc
	exitCode := -1
	exitFunc = func(code int) {
		exitCode = code
	}
	t.Cleanup(func() { exitFunc = original })
	return &exitCode
}

func TestNewDisabledContext(t *testing.T) {
	ctx := NewDisabledContext()
	require.NotNil(t, ctx)
	assert.False(t, ctx.Enabled(), "disabled context should not be enabled")
}

func TestContext_Enabled(t *testing.T) {
	tests := []struct {
		name     string
		ctx      *Context
		expected bool
	}{
		{name: "nil context", ctx: nil, expected: false},
		{name: "disabled context", ctx: &Context{enabled: false}, expected: false},
		{name: "enabled but no capture session", ctx: &Context{enabled: true, captureSession: nil}, expected: false},
		{name: "enabled with capture session", ctx: &Context{enabled: true, captureSession: &CaptureSession{}}, expected: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, tt.ctx.Enabled())
		})
	}
}

func TestContext_Cleanup_NilSafe(t *testing.T) {
	var ctx *Context
	assert.NotPanics(t, func() { ctx.Cleanup() })
}

func TestContext_Cleanup_DisabledContext(t *testing.T) {
	ctx := NewDisabledContext()
	assert.NotPanics(t, func() { ctx.Cleanup() })
}

func TestContext_Cleanup_Idempotent(t *testing.T) {
	ctx := &Context{enabled: true, captureSession: &CaptureSession{stopped: true}}
	assert.NotPanics(t, func() {
		ctx.Cleanup()
		ctx.Cleanup()
	})
}

func TestContext_RunAnalysis_DisabledReturnsFalse(t *testing.T) {
	ctx := NewDisabledContext()
	result := ctx.RunAnalysis(nil)
	assert.False(t, result, "disabled context RunAnalysis should return false")
}

func TestContext_RunAnalysis_NilContextReturnsFalse(t *testing.T) {
	var ctx *Context
	assert.False(t, ctx.Enabled())
}

func TestBuildCommandName_ReturnsNonEmpty(t *testing.T) {
	result := BuildCommandName()
	assert.NotEmpty(t, result)
}

func TestBuildCommandNameInternal(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected string
	}{
		{name: "simple command", args: []string{"atmos", "terraform", "plan"}, expected: "atmos terraform plan"},
		{name: "stops at -- delimiter", args: []string{"atmos", "terraform", "plan", "--", "-var", "secret=hunter2"}, expected: "atmos terraform plan"},
		{name: "includes flags before --", args: []string{"atmos", "--ai", "terraform", "plan", "--", "-target=module.vpc"}, expected: "atmos --ai terraform plan"},
		{name: "no args after --", args: []string{"atmos", "terraform", "--"}, expected: "atmos terraform"},
		{name: "empty args", args: []string{}, expected: ""},
		{name: "only --", args: []string{"--"}, expected: ""},
		{name: "single arg", args: []string{"atmos"}, expected: "atmos"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildCommandNameInternal(tt.args)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestSetup_InvalidConfig(t *testing.T) {
	cfg := &schema.AtmosConfiguration{
		AI: schema.AISettings{Enabled: false},
	}

	_, err := Setup(cfg, nil, "atmos terraform plan")
	require.Error(t, err)
	assert.True(t, errors.Is(err, atmosErrors.ErrAINotEnabled))
}

func TestSetup_ValidConfigNoSkills(t *testing.T) {
	cfg := validAIConfig()
	mockSession := &CaptureSession{stopped: false}
	withMockCapture(t, mockSession, nil)

	ctx, err := Setup(cfg, nil, "atmos terraform plan")
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.True(t, ctx.Enabled())
	assert.Equal(t, "atmos terraform plan", ctx.commandName)
	assert.Empty(t, ctx.skillPrompt)

	ctx.Cleanup()
}

func TestSetup_ValidConfigWithSkills(t *testing.T) {
	cfg := validAIConfigWithSkill()
	mockSession := &CaptureSession{stopped: false}
	withMockCapture(t, mockSession, nil)

	ctx, err := Setup(cfg, []string{"test-skill"}, "atmos terraform plan")
	require.NoError(t, err)
	require.NotNil(t, ctx)
	assert.True(t, ctx.Enabled())
	assert.Contains(t, ctx.skillPrompt, "You are a test expert.")

	ctx.Cleanup()
}

func TestSetup_InvalidSkillName(t *testing.T) {
	cfg := validAIConfig()
	mockSession := &CaptureSession{stopped: false}
	withMockCapture(t, mockSession, nil)

	_, err := Setup(cfg, []string{"nonexistent-skill"}, "atmos terraform plan")
	require.Error(t, err)
	assert.True(t, errors.Is(err, atmosErrors.ErrAISkillNotFound))
}

func TestSetup_CaptureFailure(t *testing.T) {
	cfg := validAIConfig()
	withMockCapture(t, nil, errors.New("pipe creation failed"))

	ctx, err := Setup(cfg, nil, "atmos terraform plan")
	require.NoError(t, err, "capture failure should not return error")
	require.NotNil(t, ctx)
	assert.False(t, ctx.Enabled(), "context should be disabled when capture fails")
}

func TestRunAnalysis_SuccessNoError(t *testing.T) {
	capturedInput := withMockAnalyzePtr(t)
	exitCode := withMockExit(t)

	ctx := &Context{
		enabled:     true,
		commandName: "atmos terraform plan",
		atmosConfig: validAIConfig(),
		captureSession: &CaptureSession{
			stopped: true,
		},
	}
	// Pre-populate buffers to simulate captured output.
	ctx.captureSession.stdoutBuf.WriteString("Plan: 1 to add")

	result := ctx.RunAnalysis(nil)
	assert.True(t, result)
	require.NotNil(t, *capturedInput)
	assert.Equal(t, "atmos terraform plan", (*capturedInput).CommandName)
	assert.Equal(t, "Plan: 1 to add", (*capturedInput).Stdout)
	assert.Empty(t, (*capturedInput).Stderr)
	assert.Nil(t, (*capturedInput).CmdErr)
	assert.Equal(t, -1, *exitCode, "should not call exit on success")
}

func TestRunAnalysis_WithError(t *testing.T) {
	capturedInput := withMockAnalyzePtr(t)
	exitCode := withMockExit(t)

	ctx := &Context{
		enabled:     true,
		commandName: "atmos terraform apply",
		atmosConfig: validAIConfig(),
		captureSession: &CaptureSession{
			stopped: true,
		},
	}
	ctx.captureSession.stderrBuf.WriteString("Error: access denied")

	cmdErr := errors.New("exit status 1")
	result := ctx.RunAnalysis(cmdErr)
	assert.True(t, result)
	require.NotNil(t, *capturedInput)
	assert.Equal(t, cmdErr, (*capturedInput).CmdErr)
	assert.Contains(t, (*capturedInput).Stderr, "access denied")
	assert.Equal(t, 1, *exitCode, "should call exit with code 1")
}

func TestRunAnalysis_WithSkills(t *testing.T) {
	capturedInput := withMockAnalyzePtr(t)
	_ = withMockExit(t)

	ctx := &Context{
		enabled:     true,
		commandName: "atmos terraform plan",
		skillNames:  []string{"atmos-terraform"},
		skillPrompt: "Terraform expert prompt.",
		atmosConfig: validAIConfig(),
		captureSession: &CaptureSession{
			stopped: true,
		},
	}
	ctx.captureSession.stdoutBuf.WriteString("output")

	result := ctx.RunAnalysis(nil)
	assert.True(t, result)
	require.NotNil(t, *capturedInput)
	assert.Equal(t, []string{"atmos-terraform"}, (*capturedInput).SkillNames)
	assert.Equal(t, "Terraform expert prompt.", (*capturedInput).SkillPrompt)
}

func TestRunAnalysis_ErrorAppendsToExistingStderr(t *testing.T) {
	capturedInput := withMockAnalyzePtr(t)
	_ = withMockExit(t)

	ctx := &Context{
		enabled:     true,
		commandName: "atmos terraform apply",
		atmosConfig: validAIConfig(),
		captureSession: &CaptureSession{
			stopped: true,
		},
	}
	ctx.captureSession.stderrBuf.WriteString("Warning: something")

	cmdErr := errors.New("failed")
	ctx.RunAnalysis(cmdErr)

	require.NotNil(t, *capturedInput)
	// Stderr should contain both the original warning and the formatted error.
	assert.Contains(t, (*capturedInput).Stderr, "Warning: something")
	assert.Contains(t, (*capturedInput).Stderr, "failed")
}

func TestRunAnalysis_ErrorWithEmptyStderr(t *testing.T) {
	capturedInput := withMockAnalyzePtr(t)
	_ = withMockExit(t)

	ctx := &Context{
		enabled:     true,
		commandName: "atmos terraform apply",
		atmosConfig: validAIConfig(),
		captureSession: &CaptureSession{
			stopped: true,
		},
	}

	cmdErr := errors.New("command failed")
	ctx.RunAnalysis(cmdErr)

	require.NotNil(t, *capturedInput)
	// Empty stderr should not have leading newline.
	assert.NotEqual(t, "\n", (*capturedInput).Stderr[:1])
	assert.Contains(t, (*capturedInput).Stderr, "command failed")
}
