package output

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Helper function to create minimal valid sections.
func validSections() map[string]any {
	return map[string]any{
		cfg.CommandSectionName:   "/usr/local/bin/terraform",
		cfg.WorkspaceSectionName: "test-workspace",
		"component_info": map[string]any{
			"component_path": "/tmp/test-component",
		},
		cfg.BackendTypeSectionName: "s3",
		cfg.BackendSectionName: map[string]any{
			"bucket": "test-bucket",
			"key":    "test-key",
		},
	}
}

// Helper function to create minimal valid atmos config.
func validAtmosConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				AutoGenerateBackendFile: false,
				InitRunReconfigure:      false,
			},
		},
		Logs: schema.Logs{
			Level: "info",
		},
	}
}

func TestNewExecutor(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)

	// Test basic creation.
	exec := NewExecutor(mockDescriber)
	require.NotNil(t, exec)
	assert.NotNil(t, exec.runnerFactory)
	assert.Equal(t, mockDescriber, exec.componentDescriber)
	assert.Nil(t, exec.staticRemoteStateGetter)
}

func TestNewExecutor_WithOptions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockGetter := NewMockStaticRemoteStateGetter(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return NewMockTerraformRunner(ctrl), nil
	}

	exec := NewExecutor(mockDescriber,
		WithRunnerFactory(customFactory),
		WithStaticRemoteStateGetter(mockGetter),
	)

	require.NotNil(t, exec)
	assert.NotNil(t, exec.runnerFactory)
	assert.Equal(t, mockGetter, exec.staticRemoteStateGetter)
}

func TestExecutor_ExecuteWithSections_DisabledComponent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	atmosConfig := validAtmosConfig()

	// Create sections with disabled component (enabled=false in vars).
	sections := validSections()
	sections[cfg.VarsSectionName] = map[string]any{
		"enabled": false,
	}

	outputs, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
	assert.Empty(t, outputs)
}

func TestExecutor_ExecuteWithSections_AbstractComponent(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	atmosConfig := validAtmosConfig()

	// Create sections with abstract component.
	sections := validSections()
	sections[cfg.MetadataSectionName] = map[string]any{
		"type": "abstract",
	}

	outputs, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
	assert.Empty(t, outputs)
}

func TestExecutor_ExecuteWithSections_MissingExecutable(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	atmosConfig := validAtmosConfig()

	// Create sections without executable.
	sections := map[string]any{
		cfg.WorkspaceSectionName: "test-workspace",
		"component_info": map[string]any{
			"component_path": "/tmp/test-component",
		},
	}

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrMissingExecutable), "expected ErrMissingExecutable")
}

func TestExecutor_ExecuteWithSections_MissingWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	atmosConfig := validAtmosConfig()

	// Create sections without workspace.
	sections := map[string]any{
		cfg.CommandSectionName: "/usr/local/bin/terraform",
		"component_info": map[string]any{
			"component_path": "/tmp/test-component",
		},
	}

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrMissingWorkspace), "expected ErrMissingWorkspace")
}

func TestExecutor_ExecuteWithSections_MissingComponentPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	atmosConfig := validAtmosConfig()

	// Create sections without component_info.
	sections := map[string]any{
		cfg.CommandSectionName:   "/usr/local/bin/terraform",
		cfg.WorkspaceSectionName: "test-workspace",
	}

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrMissingComponentInfo), "expected ErrMissingComponentInfo")
}

func TestExecutor_ExecuteWithSections_RunnerFactoryError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)

	factoryErr := errors.New("failed to create runner")
	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return nil, factoryErr
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to create runner")
}

func TestExecutor_ExecuteWithSections_InitError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Setup expectations.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(errors.New("init failed"))

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformInit), "expected ErrTerraformInit")
}

func TestExecutor_ExecuteWithSections_WorkspaceNewError_AlreadyExists(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Setup expectations - workspace already exists, so it falls back to select.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(errors.New("Workspace test-workspace already exists"))
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(errors.New("select failed"))

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformWorkspaceOp), "expected ErrTerraformWorkspaceOp")
}

func TestExecutor_ExecuteWithSections_WorkspaceNewError_Unexpected(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Setup expectations - unexpected error (network, permission, etc.) should fail fast.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(errors.New("network timeout"))
	// WorkspaceSelect should NOT be called for unexpected errors.

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformWorkspaceOp), "expected ErrTerraformWorkspaceOp")
	assert.Contains(t, err.Error(), "network timeout")
}

func TestExecutor_ExecuteWithSections_OutputError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Setup expectations.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(nil)
	// Use AnyTimes() because retryOnWindows may call Output multiple times on Windows.
	mockRunner.EXPECT().Output(gomock.Any()).Return(nil, errors.New("output failed")).AnyTimes()

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "output failed")
}

func TestExecutor_ExecuteWithSections_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Setup expectations.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"vpc_id": {
			Value: []byte(`"vpc-123456"`),
		},
		"enabled": {
			Value: []byte(`true`),
		},
	}, nil)

	outputs, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-123456", outputs["vpc_id"])
	assert.Equal(t, true, outputs["enabled"])
}

func TestExecutor_ExecuteWithSections_HTTPBackend_SkipsWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()
	sections[cfg.BackendTypeSectionName] = "http"

	// HTTP backend should skip workspace operations entirely.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	// No workspace calls expected for HTTP backend.
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"result": {
			Value: []byte(`"success"`),
		},
	}, nil)

	outputs, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
	assert.Equal(t, "success", outputs["result"])
}

func TestExecutor_GetOutput_StaticRemoteState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockGetter := NewMockStaticRemoteStateGetter(ctrl)

	exec := NewExecutor(mockDescriber, WithStaticRemoteStateGetter(mockGetter))

	// Clear any cached outputs for this test.
	terraformOutputsCache.Delete("test-stack-test-component")

	atmosConfig := validAtmosConfig()

	// Setup static remote state.
	staticOutputs := map[string]any{
		"vpc_id": "vpc-static-123",
	}

	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).Return(validSections(), nil)
	mockGetter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(staticOutputs)

	value, exists, err := exec.GetOutput(atmosConfig, "test-stack", "test-component", "vpc_id", true, nil, nil)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "vpc-static-123", value)
}

func TestExecutor_GetOutput_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	atmosConfig := validAtmosConfig()

	// Pre-populate cache.
	stackSlug := "cached-stack-cached-component"
	terraformOutputsCache.Store(stackSlug, map[string]any{
		"cached_value": "from-cache",
	})
	defer terraformOutputsCache.Delete(stackSlug)

	// No DescribeComponent call expected since we hit cache.
	value, exists, err := exec.GetOutput(atmosConfig, "cached-stack", "cached-component", "cached_value", false, nil, nil)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "from-cache", value)
}

func TestExecutor_GetOutput_NonexistentKey(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	atmosConfig := validAtmosConfig()

	// Pre-populate cache.
	stackSlug := "nonexistent-stack-nonexistent-component"
	terraformOutputsCache.Store(stackSlug, map[string]any{
		"existing_key": "value",
	})
	defer terraformOutputsCache.Delete(stackSlug)

	// Should return exists=false for nonexistent key.
	value, exists, err := exec.GetOutput(atmosConfig, "nonexistent-stack", "nonexistent-component", "missing_key", false, nil, nil)
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, value)
}

func TestQuietModeWriter(t *testing.T) {
	w := newQuietModeWriter()

	n, err := w.Write([]byte("test output"))
	require.NoError(t, err)
	assert.Equal(t, 11, n)

	assert.Equal(t, "test output", w.String())
}

func TestWrapErrorWithStderr(t *testing.T) {
	baseErr := errors.New("base error")

	// Test with nil capture.
	result := wrapErrorWithStderr(baseErr, nil)
	assert.Equal(t, baseErr, result)

	// Test with empty capture.
	emptyCapture := newQuietModeWriter()
	result = wrapErrorWithStderr(baseErr, emptyCapture)
	assert.Equal(t, baseErr, result)

	// Test with non-empty capture.
	capture := newQuietModeWriter()
	_, _ = capture.Write([]byte("stderr output"))
	result = wrapErrorWithStderr(baseErr, capture)
	assert.True(t, errors.Is(result, errUtils.ErrTerraformOutputFailed))
	// The error wraps the base error, so we need to check the wrapped error contains stdout.
	assert.Contains(t, result.Error(), "base error")
}

func TestSummarizeValue(t *testing.T) {
	// Create a string that's exactly 101 characters to test truncation at 100.
	longString := "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa" // 100 a's
	longString += "b"                                                                                                    // 101 chars total

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "short string",
			input:    "hello",
			expected: "hello",
		},
		{
			name:     "long string truncated",
			input:    longString,
			expected: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa...", // 100 a's + ...
		},
		{
			name:     "multiline string",
			input:    "line1\nline2\nline3",
			expected: "<multiline: 3 lines, 17 bytes>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := summarizeValue(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestCheckOutputsCache(t *testing.T) {
	// Test cache miss.
	result := checkOutputsCache("miss-slug", "component", "stack")
	assert.Nil(t, result)

	// Test cache hit.
	cached := map[string]any{"key": "value"}
	terraformOutputsCache.Store("hit-slug", cached)
	defer terraformOutputsCache.Delete("hit-slug")

	result = checkOutputsCache("hit-slug", "component", "stack")
	assert.Equal(t, cached, result)
}

func TestHandleDisabledComponent(t *testing.T) {
	// Test disabled component.
	result := handleDisabledComponent("comp", "stack", false, false)
	assert.Empty(t, result)

	// Test abstract component.
	result = handleDisabledComponent("comp", "stack", true, true)
	assert.Empty(t, result)
}

func TestExtractYqValue(t *testing.T) {
	atmosConfig := validAtmosConfig()

	data := map[string]any{
		"simple": "value",
		"nested": map[string]any{
			"key": "nested_value",
		},
	}

	// Test simple key.
	val, exists, err := extractYqValue(atmosConfig, data, "simple", "test context")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "value", val)

	// Test nested key.
	val, exists, err = extractYqValue(atmosConfig, data, ".nested.key", "test context")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "nested_value", val)

	// Test missing key.
	val, exists, err = extractYqValue(atmosConfig, data, "missing", "test context")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, val)
}

func TestGetStaticRemoteStateOutput(t *testing.T) {
	atmosConfig := validAtmosConfig()

	remoteState := map[string]any{
		"vpc_id":     "vpc-123",
		"subnet_ids": []any{"subnet-1", "subnet-2"},
	}

	// Test existing key.
	val, exists, err := GetStaticRemoteStateOutput(atmosConfig, "comp", "stack", remoteState, "vpc_id")
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "vpc-123", val)

	// Test missing key.
	val, exists, err = GetStaticRemoteStateOutput(atmosConfig, "comp", "stack", remoteState, "missing")
	require.NoError(t, err)
	assert.False(t, exists)
	assert.Nil(t, val)
}

func TestExecutor_ExecuteWithSections_QuietMode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Quiet mode should set stdout to discard and stderr to capture.
	mockRunner.EXPECT().SetStdout(gomock.Any()).Times(1)
	mockRunner.EXPECT().SetStderr(gomock.Any()).Times(1)
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"result": {
			Value: []byte(`"quiet_success"`),
		},
	}, nil)

	// Execute with quiet mode via internal execute call.
	ctx := context.Background()
	outputs, err := exec.execute(ctx, atmosConfig, "test-component", "test-stack", sections, nil, &OutputOptions{QuietMode: true})
	require.NoError(t, err)
	assert.Equal(t, "quiet_success", outputs["result"])
}

func TestGetOutputVariable(t *testing.T) {
	atmosConfig := validAtmosConfig()

	outputs := map[string]any{
		"vpc_id":  "vpc-123",
		"enabled": true,
		"count":   42,
	}

	tests := []struct {
		name      string
		output    string
		expected  any
		exists    bool
		expectErr bool
	}{
		{"simple string", "vpc_id", "vpc-123", true, false},
		{"boolean", "enabled", true, true, false},
		{"number", "count", 42, true, false},
		{"missing", "missing_key", nil, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			val, exists, err := getOutputVariable(atmosConfig, "comp", "stack", outputs, tt.output)
			if tt.expectErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.exists, exists)
				assert.Equal(t, tt.expected, val)
			}
		})
	}
}

// TestWrapDescribeError_BreaksErrInvalidComponentChain tests that wrapDescribeError
// correctly breaks the ErrInvalidComponent chain to prevent component type fallback.
// This is a regression test for https://github.com/cloudposse/atmos/issues/1030.
func TestWrapDescribeError_BreaksErrInvalidComponentChain(t *testing.T) {
	tests := []struct {
		name            string
		inputErr        error
		wantErrDescribe bool
		wantErrInvalid  bool
		wantMsgContains string
	}{
		{
			name: "ErrInvalidComponent chain is broken",
			// Use fmt.Errorf with %w for proper error wrapping (causality chain).
			inputErr:        fmt.Errorf("component not found: %w", errUtils.ErrInvalidComponent),
			wantErrDescribe: true,
			wantErrInvalid:  false, // Chain should be broken
			wantMsgContains: "component not found",
		},
		{
			name: "wrapped ErrInvalidComponent chain is broken",
			// Use fmt.Errorf with %w to express "this happened because of that".
			inputErr:        fmt.Errorf("outer error: %w", errUtils.ErrInvalidComponent),
			wantErrDescribe: true,
			wantErrInvalid:  false, // Chain should be broken
			wantMsgContains: "component",
		},
		{
			name:            "other errors preserve chain",
			inputErr:        errors.New("network timeout"),
			wantErrDescribe: false,
			wantErrInvalid:  false,
			wantMsgContains: "network timeout",
		},
		{
			name:            "ErrTerraformStateNotProvisioned preserves chain",
			inputErr:        errUtils.ErrTerraformStateNotProvisioned,
			wantErrDescribe: false,
			wantErrInvalid:  false,
			wantMsgContains: "not provisioned",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := wrapDescribeError("test-comp", "test-stack", tt.inputErr)
			require.Error(t, result)

			// Check ErrDescribeComponent.
			if tt.wantErrDescribe {
				assert.ErrorIs(t, result, errUtils.ErrDescribeComponent,
					"Expected ErrDescribeComponent in error chain")
			}

			// Check ErrInvalidComponent - should NOT be in chain for broken cases.
			if tt.wantErrInvalid {
				assert.ErrorIs(t, result, errUtils.ErrInvalidComponent,
					"Expected ErrInvalidComponent in error chain")
			} else {
				assert.NotErrorIs(t, result, errUtils.ErrInvalidComponent,
					"ErrInvalidComponent should NOT be in error chain (chain should be broken)")
			}

			// Check message content.
			if tt.wantMsgContains != "" {
				assert.Contains(t, result.Error(), tt.wantMsgContains)
			}
		})
	}
}
