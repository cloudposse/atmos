package output

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/hashicorp/terraform-exec/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// Helper function to create minimal valid sections.
func validSections() map[string]any {
	return map[string]any{
		cfg.CommandSectionName:   "/usr/local/bin/terraform",
		cfg.WorkspaceSectionName: "test-workspace",
		cfg.ComponentSectionName: "test-component",
		"component_info": map[string]any{
			"component_type": "terraform",
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
		BasePath: filepath.Join(os.TempDir(), "test-project"),
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath:                "components/terraform",
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

func TestExecutor_ExecuteWithSections_WorkspaceSelectFails_NewFails(t *testing.T) {
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

	// Setup expectations - select fails, then new also fails.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(errors.New(`Workspace "test-workspace" doesn't exist.`))
	mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(errors.New("create failed"))

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformWorkspaceOp), "expected ErrTerraformWorkspaceOp")
}

func TestExecutor_ExecuteWithSections_WorkspaceSelectFails_NewSucceeds(t *testing.T) {
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

	// Setup expectations - select fails (workspace doesn't exist), new succeeds.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(errors.New(`Workspace "test-workspace" doesn't exist.`))
	mockRunner.EXPECT().WorkspaceNew(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(nil, nil)

	_, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
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
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
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
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
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
	terraformOutputsCache.Delete(stackComponentKey("test-stack", "test-component"))

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
	stackSlug := stackComponentKey("cached-stack", "cached-component")
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
	stackSlug := stackComponentKey("nonexistent-stack", "nonexistent-component")
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
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
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

// TestExecutor_GetOutput_InvalidAuthManagerType tests that GetOutput returns an error
// when an invalid authManager type is provided.
func TestExecutor_GetOutput_InvalidAuthManagerType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)
	atmosConfig := validAtmosConfig()

	// Pass an invalid authManager type (string instead of auth.AuthManager).
	invalidAuthManager := "not an auth manager"

	_, _, err := exec.GetOutput(atmosConfig, "test-stack", "test-component", "output", true, nil, invalidAuthManager)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidAuthManagerType)
}

// TestExecutor_GetOutput_FullExecutionPath tests the full execution path of GetOutput
// when not using cache or static remote state.
func TestExecutor_GetOutput_FullExecutionPath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Clear cache to force full execution.
	stackSlug := stackComponentKey("full-exec-stack", "full-exec-component")
	terraformOutputsCache.Delete(stackSlug)
	defer terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Setup expectations for DescribeComponent.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).Return(sections, nil)

	// Setup expectations for terraform operations.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"vpc_id": {
			Value: []byte(`"vpc-full-exec"`),
		},
	}, nil)

	value, exists, err := exec.GetOutput(atmosConfig, "full-exec-stack", "full-exec-component", "vpc_id", true, nil, nil)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "vpc-full-exec", value)
}

// TestExecutor_GetOutput_DescribeError tests that GetOutput returns an error
// when DescribeComponent fails.
func TestExecutor_GetOutput_DescribeError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	// Clear cache to force describe call.
	stackSlug := stackComponentKey("describe-err-stack", "describe-err-component")
	terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()

	// Setup expectations - DescribeComponent returns error.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).Return(nil, errors.New("component not found"))

	_, _, err := exec.GetOutput(atmosConfig, "describe-err-stack", "describe-err-component", "output", true, nil, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "component not found")
}

// TestExecutor_GetAllOutputs_Success tests the happy path for GetAllOutputs.
func TestExecutor_GetAllOutputs_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Clear cache.
	stackSlug := stackComponentKey("all-outputs-stack", "all-outputs-component")
	terraformOutputsCache.Delete(stackSlug)
	defer terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	// Set debug level to avoid spinner.
	atmosConfig.Logs.Level = "debug"

	sections := validSections()

	// Setup expectations.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).Return(sections, nil)
	mockRunner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetStderr(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"vpc_id":  {Value: []byte(`"vpc-all"`)},
		"enabled": {Value: []byte(`true`)},
	}, nil)

	outputs, err := exec.GetAllOutputs(atmosConfig, "all-outputs-component", "all-outputs-stack", false, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-all", outputs["vpc_id"])
	assert.Equal(t, true, outputs["enabled"])
}

// TestExecutor_GetAllOutputs_CacheHit tests that GetAllOutputs returns cached values.
func TestExecutor_GetAllOutputs_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	// Pre-populate cache.
	stackSlug := stackComponentKey("cache-hit-stack", "cache-hit-component")
	cachedOutputs := map[string]any{"cached_key": "cached_value"}
	terraformOutputsCache.Store(stackSlug, cachedOutputs)
	defer terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()

	// No DescribeComponent call expected.
	outputs, err := exec.GetAllOutputs(atmosConfig, "cache-hit-component", "cache-hit-stack", false, nil)
	require.NoError(t, err)
	assert.Equal(t, cachedOutputs, outputs)
}

// TestExecutor_GetAllOutputs_Error tests that GetAllOutputs handles errors properly.
func TestExecutor_GetAllOutputs_Error(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	exec := NewExecutor(mockDescriber)

	// Clear cache.
	stackSlug := stackComponentKey("error-stack", "error-component")
	terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	// Set debug level to avoid spinner.
	atmosConfig.Logs.Level = "debug"

	// Setup expectations - DescribeComponent returns error.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).Return(nil, errors.New("describe failed"))

	outputs, err := exec.GetAllOutputs(atmosConfig, "error-component", "error-stack", false, nil)
	require.Error(t, err)
	assert.Nil(t, outputs)
}

// TestStartSpinnerOrLog_DebugMode tests that startSpinnerOrLog logs in debug mode.
func TestStartSpinnerOrLog_DebugMode(t *testing.T) {
	atmosConfig := validAtmosConfig()
	atmosConfig.Logs.Level = "debug"

	stopFunc := startSpinnerOrLog(atmosConfig, "test message", "component", "stack")
	require.NotNil(t, stopFunc)

	// Should be a no-op function.
	stopFunc()
}

// TestStartSpinnerOrLog_TraceMode tests that startSpinnerOrLog logs in trace mode.
func TestStartSpinnerOrLog_TraceMode(t *testing.T) {
	atmosConfig := validAtmosConfig()
	atmosConfig.Logs.Level = "trace"

	stopFunc := startSpinnerOrLog(atmosConfig, "test message", "component", "stack")
	require.NotNil(t, stopFunc)

	// Should be a no-op function.
	stopFunc()
}

// TestExecutor_GetAllOutputs_StaticRemoteState tests GetAllOutputs with static remote state.
func TestExecutor_GetAllOutputs_StaticRemoteState(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockGetter := NewMockStaticRemoteStateGetter(ctrl)

	exec := NewExecutor(mockDescriber, WithStaticRemoteStateGetter(mockGetter))

	// Clear cache.
	stackSlug := stackComponentKey("static-stack", "static-component")
	terraformOutputsCache.Delete(stackSlug)
	defer terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	atmosConfig.Logs.Level = "debug"

	sections := validSections()
	staticOutputs := map[string]any{"static_key": "static_value"}

	// Setup expectations.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).Return(sections, nil)
	mockGetter.EXPECT().GetStaticRemoteStateOutputs(gomock.Any()).Return(staticOutputs)

	outputs, err := exec.GetAllOutputs(atmosConfig, "static-component", "static-stack", false, nil)
	require.NoError(t, err)
	assert.Equal(t, staticOutputs, outputs)
}

// TestProcessOutputs_WithInvalidJSON tests processOutputs handling of invalid JSON.
func TestProcessOutputs_WithInvalidJSON(t *testing.T) {
	atmosConfig := validAtmosConfig()

	// Create output meta with invalid JSON value.
	outputMeta := map[string]tfexec.OutputMeta{
		"invalid_json": {
			Value: []byte(`not valid json`),
		},
		"valid_value": {
			Value: []byte(`"hello"`),
		},
	}

	outputs := processOutputs(outputMeta, atmosConfig)

	// Invalid JSON should result in nil value.
	assert.Nil(t, outputs["invalid_json"])
	// Valid value should be processed correctly.
	assert.Equal(t, "hello", outputs["valid_value"])
}

// TestExecutor_ExecuteWithSections_InitWithReconfigure tests init with reconfigure option.
func TestExecutor_ExecuteWithSections_InitWithReconfigure(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))
	atmosConfig := validAtmosConfig()
	atmosConfig.Components.Terraform.InitRunReconfigure = true

	sections := validSections()

	// Setup expectations - init should be called with reconfigure option.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"result": {Value: []byte(`"reconfigure_success"`)},
	}, nil)

	outputs, err := exec.ExecuteWithSections(atmosConfig, "test-component", "test-stack", sections, nil)
	require.NoError(t, err)
	assert.Equal(t, "reconfigure_success", outputs["result"])
}

// TestExecutor_GetOutput_ExecuteError tests GetOutput when execute fails.
func TestExecutor_GetOutput_ExecuteError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Clear cache.
	stackSlug := stackComponentKey("exec-err-stack", "exec-err-component")
	terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	sections := validSections()

	// Setup expectations.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).Return(sections, nil)
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(errors.New("init failed"))

	_, _, err := exec.GetOutput(atmosConfig, "exec-err-stack", "exec-err-component", "output", true, nil, nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, errUtils.ErrTerraformOutputFailed), "expected ErrTerraformOutputFailed")
}

// TestHighlightValue_NilConfig tests the highlightValue function with nil config.
func TestHighlightValue_NilConfig(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		config   *schema.AtmosConfiguration
		expected string
	}{
		{
			name:     "nil config returns input unchanged",
			input:    `{"key": "value"}`,
			config:   nil,
			expected: `{"key": "value"}`,
		},
		{
			name:     "with config attempts highlighting",
			input:    `{"key": "value"}`,
			config:   validAtmosConfig(),
			expected: `{"key": "value"}`, // May be highlighted or not depending on TTY.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := highlightValue(tt.input, tt.config)
			// For nil config, result should be exactly the input.
			if tt.config == nil {
				assert.Equal(t, tt.expected, result)
			} else {
				// For non-nil config, result may be highlighted or unchanged.
				// Just ensure it contains the key content.
				assert.Contains(t, result, "key")
			}
		})
	}
}

// TestExecutor_ExecuteWithSections_ComponentPathResolution tests that component paths
// are correctly resolved using utils.GetComponentPath, ensuring proper path construction
// based on atmosConfig.BasePath and component settings.
// This is a regression test for the issue where atmos.Component template function failed with
// "backend.tf.json: no such file or directory" when running with --chdir or from a non-project-root directory.
func TestExecutor_ExecuteWithSections_ComponentPathResolution(t *testing.T) {
	// Create a temp directory to use as an absolute path that works cross-platform.
	tempDir := t.TempDir()

	tests := []struct {
		name                  string
		basePath              string
		componentName         string
		componentFolderPrefix string
		expectedWorkdirSuffix string // Use suffix check for cross-platform compatibility.
		description           string
	}{
		{
			name:                  "standard component path resolution",
			basePath:              tempDir,
			componentName:         "vpc",
			componentFolderPrefix: "",
			expectedWorkdirSuffix: filepath.Join("components", "terraform", "vpc"),
			description:           "Component path should be constructed using BasePath and component settings",
		},
		{
			name:                  "component with folder prefix",
			basePath:              tempDir,
			componentName:         "mycomponent",
			componentFolderPrefix: "custom",
			expectedWorkdirSuffix: filepath.Join("components", "terraform", "custom", "mycomponent"),
			description:           "Component path should include folder prefix when specified",
		},
		{
			name:                  "nested component name",
			basePath:              tempDir,
			componentName:         "network/vpc",
			componentFolderPrefix: "",
			expectedWorkdirSuffix: filepath.Join("components", "terraform", "network", "vpc"),
			description:           "Nested component names should be handled correctly",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockDescriber := NewMockComponentDescriber(ctrl)
			mockRunner := NewMockTerraformRunner(ctrl)

			// Track what workdir is passed to the runner factory.
			var capturedWorkdir string
			customFactory := func(workdir, executable string) (TerraformRunner, error) {
				capturedWorkdir = workdir
				return mockRunner, nil
			}

			exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

			atmosConfig := &schema.AtmosConfiguration{
				BasePath: tt.basePath,
				Components: schema.Components{
					Terraform: schema.Terraform{
						BasePath:                "components/terraform",
						AutoGenerateBackendFile: false,
						InitRunReconfigure:      false,
					},
				},
				Logs: schema.Logs{
					Level: "info",
				},
			}

			sections := map[string]any{
				cfg.CommandSectionName:   "terraform",
				cfg.WorkspaceSectionName: "test-workspace",
				cfg.ComponentSectionName: tt.componentName,
				"component_info": map[string]any{
					"component_type": "terraform",
				},
				cfg.BackendTypeSectionName: "s3",
				cfg.BackendSectionName: map[string]any{
					"bucket": "test-bucket",
					"key":    "test-key",
				},
			}

			// Add folder prefix to metadata if specified.
			if tt.componentFolderPrefix != "" {
				sections[cfg.MetadataSectionName] = map[string]any{
					"component_folder_prefix": tt.componentFolderPrefix,
				}
			}

			// Setup expectations.
			mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
			mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
			mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
			mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
				"vpc_id": {
					Value: []byte(`"vpc-123456"`),
				},
			}, nil)

			outputs, err := exec.ExecuteWithSections(atmosConfig, tt.componentName, "dev-ue1", sections, nil)
			require.NoError(t, err)
			assert.Equal(t, "vpc-123456", outputs["vpc_id"])

			// Verify the path is absolute.
			assert.True(t, filepath.IsAbs(capturedWorkdir), "%s: expected absolute path, got %q", tt.description, capturedWorkdir)

			// Verify the path contains expected suffix using normalized slashes for cross-platform compatibility.
			normalizedCaptured := filepath.ToSlash(capturedWorkdir)
			normalizedExpected := filepath.ToSlash(tt.expectedWorkdirSuffix)
			assert.True(t, strings.HasSuffix(normalizedCaptured, normalizedExpected),
				"%s: expected workdir to end with %q, got %q", tt.description, normalizedExpected, normalizedCaptured)
		})
	}
}

// TestExecutor_GetAllOutputs_SkipInit_SkipsInitAndWorkspace verifies that when skipInit=true,
// GetAllOutputs skips CleanWorkspace, terraform init, and workspace operations, only running
// terraform output. This is critical for CI PostRunE context where the component was just
// applied and auth credentials may not be available for re-initialization.
func TestExecutor_GetAllOutputs_SkipInit_SkipsInitAndWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Clear cache.
	stackSlug := stackComponentKey("skipinit-stack", "skipinit-component")
	terraformOutputsCache.Delete(stackSlug)
	defer terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	atmosConfig.Logs.Level = "debug"

	sections := validSections()

	// DescribeComponent should be called with ProcessYamlFunctions=false when skipInit=true and authManager=nil.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).DoAndReturn(
		func(params *DescribeComponentParams) (map[string]any, error) {
			assert.False(t, params.ProcessYamlFunctions,
				"ProcessYamlFunctions should be false when skipInit=true and authManager=nil")
			return sections, nil
		},
	)

	// Quiet mode sets stdout/stderr.
	mockRunner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetStderr(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()

	// Init and workspace operations should NOT be called.
	// (If they were called, the test would fail with "unexpected call".)

	// Only terraform output should be called.
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"vpc_id": {Value: []byte(`"vpc-skipinit"`)},
	}, nil)

	outputs, err := exec.GetAllOutputs(atmosConfig, "skipinit-component", "skipinit-stack", true, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-skipinit", outputs["vpc_id"])
}

// TestExecutor_GetAllOutputs_SkipInit_False_RunsInitAndWorkspace verifies that when
// skipInit=false, GetAllOutputs runs the full init/workspace sequence as normal.
func TestExecutor_GetAllOutputs_SkipInit_False_RunsInitAndWorkspace(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)
	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Clear cache.
	stackSlug := stackComponentKey("noskip-stack", "noskip-component")
	terraformOutputsCache.Delete(stackSlug)
	defer terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	atmosConfig.Logs.Level = "debug"

	sections := validSections()

	// DescribeComponent should be called with ProcessYamlFunctions=true when skipInit=false.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).DoAndReturn(
		func(params *DescribeComponentParams) (map[string]any, error) {
			assert.True(t, params.ProcessYamlFunctions,
				"ProcessYamlFunctions should be true when skipInit=false")
			return sections, nil
		},
	)

	mockRunner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetStderr(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()

	// Init and workspace operations SHOULD be called.
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"vpc_id": {Value: []byte(`"vpc-noskip"`)},
	}, nil)

	outputs, err := exec.GetAllOutputs(atmosConfig, "noskip-component", "noskip-stack", false, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-noskip", outputs["vpc_id"])
}

// TestExecutor_GetAllOutputs_SkipInit_WithAuthManager_ProcessesYamlFunctions verifies that
// when skipInit=true but authManager is provided, YAML functions are still processed.
func TestExecutor_GetAllOutputs_SkipInit_WithAuthManager_ProcessesYamlFunctions(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Clear cache.
	stackSlug := "skipauth-stack-skipauth-component"
	terraformOutputsCache.Delete(stackSlug)
	defer terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	atmosConfig.Logs.Level = "debug"

	sections := validSections()

	// Use a non-nil authManager (string is fine since it won't be type-asserted in this path).
	fakeAuthManager := "fake-auth-manager"

	// DescribeComponent should be called with ProcessYamlFunctions=true when authManager is non-nil.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).DoAndReturn(
		func(params *DescribeComponentParams) (map[string]any, error) {
			assert.True(t, params.ProcessYamlFunctions,
				"ProcessYamlFunctions should be true when authManager is provided, even with skipInit=true")
			assert.Equal(t, fakeAuthManager, params.AuthManager,
				"AuthManager should be passed through")
			return sections, nil
		},
	)

	mockRunner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetStderr(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()

	// Init should NOT be called (skipInit=true).
	// Only terraform output should be called.
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"vpc_id": {Value: []byte(`"vpc-auth"`)},
	}, nil)

	outputs, err := exec.GetAllOutputs(atmosConfig, "skipauth-component", "skipauth-stack", true, fakeAuthManager)
	require.NoError(t, err)
	assert.Equal(t, "vpc-auth", outputs["vpc_id"])
}

// TestExecutor_Execute_SkipInit_DirectCall verifies SkipInit behavior at the execute() level.
func TestExecutor_Execute_SkipInit_DirectCall(t *testing.T) {
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

	// Setup expectations — no Init or Workspace calls expected.
	mockRunner.EXPECT().SetStdout(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetStderr(gomock.Any()).AnyTimes()
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"result": {Value: []byte(`"skip_init_direct"`)},
	}, nil)

	ctx := context.Background()
	outputs, err := exec.execute(ctx, atmosConfig, "comp", "stack", sections, nil, &OutputOptions{QuietMode: true, SkipInit: true})
	require.NoError(t, err)
	assert.Equal(t, "skip_init_direct", outputs["result"])
}

// TestExecutor_ExecuteWithSections_ToolchainResolvesExecutable verifies that the executor
// resolves toolchain-installed executables to absolute paths before passing them to the
// runner factory. This ensures that template functions like atmos.Component() and YAML
// functions like !terraform.output can find toolchain-installed binaries (e.g., tofu).
//
// This is a regression test for the issue where `atmos describe stacks` with `atmos.Component()`
// template functions fails with: exec: "tofu": executable file not found in $PATH
// even though tofu was installed via `atmos toolchain install`.
func TestExecutor_ExecuteWithSections_ToolchainResolvesExecutable(t *testing.T) {
	// Restore package-global toolchain config after test to avoid leaking
	// t.TempDir() paths into subsequent tests.
	origToolchainConfig := toolchain.GetAtmosConfig()
	t.Cleanup(func() {
		toolchain.SetAtmosConfig(origToolchainConfig)
	})

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	// Track what executable path is passed to the runner factory.
	var capturedExecutable string
	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		capturedExecutable = executable
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	// Create a temp dir structure simulating a toolchain install.
	tempDir := t.TempDir()
	componentDir := filepath.Join(tempDir, "components", "terraform", "vpc")
	require.NoError(t, os.MkdirAll(componentDir, 0o755))

	// Create a fake toolchain binary at the expected toolchain install path.
	toolchainDir := filepath.Join(tempDir, "toolchain")
	binaryDir := filepath.Join(toolchainDir, "bin", "opentofu", "opentofu", "1.8.0")
	require.NoError(t, os.MkdirAll(binaryDir, 0o755))
	binaryName := "tofu"
	if runtime.GOOS == "windows" {
		binaryName = "tofu.exe"
	}
	fakeBinary := filepath.Join(binaryDir, binaryName)
	require.NoError(t, os.WriteFile(fakeBinary, []byte("#!/bin/sh\n"), 0o755))

	atmosConfig := &schema.AtmosConfiguration{
		BasePath: tempDir,
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath:                "components/terraform",
				AutoGenerateBackendFile: false,
				InitRunReconfigure:      false,
			},
		},
		Toolchain: schema.Toolchain{
			InstallPath: toolchainDir,
		},
		Logs: schema.Logs{
			Level: "info",
		},
	}

	// Sections with bare "tofu" executable and toolchain dependencies.
	sections := map[string]any{
		cfg.CommandSectionName:   "tofu",
		cfg.WorkspaceSectionName: "test-workspace",
		cfg.ComponentSectionName: "vpc",
		"component_info": map[string]any{
			"component_type": "terraform",
		},
		cfg.BackendTypeSectionName: "s3",
		cfg.BackendSectionName: map[string]any{
			"bucket": "test-bucket",
			"key":    "test-key",
		},
		"dependencies": map[string]any{
			"tools": map[string]any{
				"opentofu": "1.8.0",
			},
		},
	}

	// Setup mock expectations for the full execution path.
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	mockRunner.EXPECT().Init(gomock.Any(), gomock.Any()).Return(nil)
	mockRunner.EXPECT().WorkspaceSelect(gomock.Any(), "test-workspace").Return(nil)
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"vpc_id": {Value: []byte(`"vpc-123"`)},
	}, nil)

	outputs, err := exec.ExecuteWithSections(atmosConfig, "vpc", "dev-ue1", sections, nil)
	require.NoError(t, err)
	assert.Equal(t, "vpc-123", outputs["vpc_id"])

	// Verify the executable was resolved to the absolute toolchain path,
	// not passed as the bare "tofu" name.
	assert.True(t, filepath.IsAbs(capturedExecutable),
		"expected executable to be resolved to an absolute path, got %q", capturedExecutable)
	assert.Equal(t, fakeBinary, capturedExecutable,
		"expected executable to be resolved to the toolchain binary path")
}

// TestExecutor_GetOutputWithOptions_SkipInit verifies that GetOutputWithOptions with
// SkipInit: true does not call terraform init or workspace operations. This is the
// contract relied on by after-terraform-apply hooks which run in an already-initialized
// workdir — calling init again causes state migration errors with stdin disabled.
func TestExecutor_GetOutputWithOptions_SkipInit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockDescriber := NewMockComponentDescriber(ctrl)
	mockRunner := NewMockTerraformRunner(ctrl)

	customFactory := func(workdir, executable string) (TerraformRunner, error) {
		return mockRunner, nil
	}

	exec := NewExecutor(mockDescriber, WithRunnerFactory(customFactory))

	stackSlug := "skip-init-stack-skip-init-component"
	terraformOutputsCache.Delete(stackSlug)

	atmosConfig := validAtmosConfig()
	sections := validSections()

	// DescribeComponent should be called with ProcessYamlFunctions=false when SkipInit=true and authManager=nil.
	mockDescriber.EXPECT().DescribeComponent(gomock.Any()).DoAndReturn(
		func(params *DescribeComponentParams) (map[string]any, error) {
			assert.False(t, params.ProcessYamlFunctions,
				"ProcessYamlFunctions should be false when SkipInit=true and authManager=nil")
			return sections, nil
		},
	)
	mockRunner.EXPECT().SetEnv(gomock.Any()).Return(nil).AnyTimes()
	// Init and Workspace calls must NOT happen.
	mockRunner.EXPECT().Output(gomock.Any()).Return(map[string]tfexec.OutputMeta{
		"id": {Value: []byte(`"eg-test-override"`)},
	}, nil)

	value, exists, err := exec.GetOutputWithOptions(
		atmosConfig,
		"skip-init-stack",
		"skip-init-component",
		"id",
		true,
		nil,
		nil,
		&OutputOptions{SkipInit: true},
	)
	require.NoError(t, err)
	assert.True(t, exists)
	assert.Equal(t, "eg-test-override", value)
}
