package ci

import (
	"context"
	"embed"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	plugin "github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/schema"
)

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// capturingProvider is a mock CI provider that records CreateCheckRun and UpdateCheckRun calls.
type capturingProvider struct {
	mockProvider
	createCheckRunCalls []*provider.CreateCheckRunOptions
	updateCheckRunCalls []*provider.UpdateCheckRunOptions
	nextID              int64
}

func (c *capturingProvider) CreateCheckRun(_ context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	c.createCheckRunCalls = append(c.createCheckRunCalls, opts)
	c.nextID++
	return &provider.CheckRun{
		ID:     c.nextID,
		Name:   opts.Name,
		Status: opts.Status,
	}, nil
}

func (c *capturingProvider) UpdateCheckRun(_ context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	c.updateCheckRunCalls = append(c.updateCheckRunCalls, opts)
	return &provider.CheckRun{
		ID:     opts.CheckRunID,
		Name:   opts.Name,
		Status: opts.Status,
	}, nil
}

func TestExecute_BeforeTerraformPlan_TriggersCheckCreate(t *testing.T) {
	// Set up provider registry with a capturing provider.
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	Register(cp)

	// Set up plugin registry with a mock terraform plugin that has before.terraform.plan → ActionCheck.
	ClearPlugins()
	ctrl := gomock.NewController(t)
	mockPlugin := NewMockPlugin(ctrl)
	mockPlugin.EXPECT().GetType().Return("terraform").AnyTimes()
	mockPlugin.EXPECT().GetHookBindings().Return([]plugin.HookBinding{
		{
			Event:   "before.terraform.plan",
			Actions: []plugin.HookAction{plugin.ActionCheck},
		},
	}).AnyTimes()
	mockPlugin.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
	mockPlugin.EXPECT().ParseOutput("", "plan").Return(&plugin.OutputResult{}, nil).AnyTimes()
	err := RegisterPlugin(mockPlugin)
	require.NoError(t, err)

	// Execute with CI forced, checks enabled.
	err = Execute(ExecuteOptions{
		Event:       "before.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Checks: schema.CIChecksConfig{Enabled: boolPtr(true)},
			},
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev-us-east-1",
			ComponentFromArg: "vpc",
		},
	})
	require.NoError(t, err)

	// Verify CreateCheckRun was called.
	require.Len(t, cp.createCheckRunCalls, 1, "CreateCheckRun should have been called once")
	assert.Equal(t, "atmos/plan: dev-us-east-1/vpc", cp.createCheckRunCalls[0].Name)
	assert.Equal(t, provider.CheckRunStateInProgress, cp.createCheckRunCalls[0].Status)
}

func TestExecute_BeforeAfterPlan_CheckLifecycle(t *testing.T) {
	// Set up provider registry with a capturing provider.
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	Register(cp)

	// Set up plugin registry with both before and after plan bindings.
	ClearPlugins()
	ctrl := gomock.NewController(t)
	mockPlugin := NewMockPlugin(ctrl)
	mockPlugin.EXPECT().GetType().Return("terraform").AnyTimes()
	mockPlugin.EXPECT().GetHookBindings().Return([]plugin.HookBinding{
		{
			Event:   "before.terraform.plan",
			Actions: []plugin.HookAction{plugin.ActionCheck},
		},
		{
			Event:    "after.terraform.plan",
			Actions:  []plugin.HookAction{plugin.ActionCheck},
			Template: "plan",
		},
	}).AnyTimes()
	mockPlugin.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
	mockPlugin.EXPECT().ParseOutput(gomock.Any(), "plan").Return(&plugin.OutputResult{
		HasChanges: true,
		Data: &plugin.TerraformOutputData{
			ChangedResult: "Plan: 1 to add, 0 to change, 0 to destroy.",
		},
	}, nil).AnyTimes()
	err := RegisterPlugin(mockPlugin)
	require.NoError(t, err)

	atmosConfig := &schema.AtmosConfiguration{
		CI: schema.CIConfig{
			Checks: schema.CIChecksConfig{Enabled: boolPtr(true)},
		},
	}
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev-us-east-1",
		ComponentFromArg: "vpc",
	}

	// 1. Execute "before.terraform.plan" — should create a check run.
	err = Execute(ExecuteOptions{
		Event:       "before.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: atmosConfig,
		Info:        info,
	})
	require.NoError(t, err)
	require.Len(t, cp.createCheckRunCalls, 1, "CreateCheckRun should have been called once after before event")
	createdID := cp.createCheckRunCalls[0] // capture opts for verification.
	assert.Equal(t, provider.CheckRunStateInProgress, createdID.Status)

	// 2. Execute "after.terraform.plan" — should update the check run, not create a new one.
	err = Execute(ExecuteOptions{
		Event:       "after.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: atmosConfig,
		Info:        info,
		Output:      "Terraform will perform the following actions...",
	})
	require.NoError(t, err)

	// Verify UpdateCheckRun was called (not a second CreateCheckRun).
	require.Len(t, cp.updateCheckRunCalls, 1, "UpdateCheckRun should have been called once after after event")
	assert.Equal(t, int64(1), cp.updateCheckRunCalls[0].CheckRunID, "UpdateCheckRun should use the ID from CreateCheckRun")
	assert.Equal(t, provider.CheckRunStateSuccess, cp.updateCheckRunCalls[0].Status)
	assert.Equal(t, "success", cp.updateCheckRunCalls[0].Conclusion)
	assert.Equal(t, "Plan: 1 to add, 0 to change, 0 to destroy.", cp.updateCheckRunCalls[0].Title)

	// CreateCheckRun should NOT have been called again.
	assert.Len(t, cp.createCheckRunCalls, 1, "CreateCheckRun should not be called during after event when ID is stored")
}

func TestExecute_CIEnabledInConfig_NoForceFlag_NoActions(t *testing.T) {
	// When CI.Enabled=true in atmos.yaml but --ci flag is NOT passed and
	// no CI platform is detected, Execute should be a no-op.
	// ci.enabled in config means "CI features are available" but requires
	// either --ci flag or actual CI platform detection to activate.
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	Register(cp)

	ClearPlugins()
	ctrl := gomock.NewController(t)
	mockPlugin := NewMockPlugin(ctrl)
	mockPlugin.EXPECT().GetType().Return("terraform").AnyTimes()
	mockPlugin.EXPECT().GetHookBindings().Return([]plugin.HookBinding{
		{
			Event:   "before.terraform.plan",
			Actions: []plugin.HookAction{plugin.ActionCheck},
		},
	}).AnyTimes()
	mockPlugin.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
	mockPlugin.EXPECT().ParseOutput("", "plan").Return(&plugin.OutputResult{}, nil).AnyTimes()
	err := RegisterPlugin(mockPlugin)
	require.NoError(t, err)

	// Execute WITHOUT ForceCIMode — CI is enabled only via config.
	err = Execute(ExecuteOptions{
		Event:       "before.terraform.plan",
		ForceCIMode: false,
		AtmosConfig: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Enabled: true,
				Checks:  schema.CIChecksConfig{Enabled: boolPtr(true)},
			},
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev-us-east-1",
			ComponentFromArg: "vpc",
		},
	})
	require.NoError(t, err)

	// No CI actions should run — ci.enabled alone does not trigger the generic fallback.
	assert.Empty(t, cp.createCheckRunCalls, "CreateCheckRun should NOT be called without --ci flag even when CI.Enabled=true in config")
}

// capturingOutputWriter records all WriteSummary and WriteOutput calls.
type capturingOutputWriter struct {
	summaries []string
	outputs   map[string]string
}

func newCapturingOutputWriter() *capturingOutputWriter {
	return &capturingOutputWriter{outputs: make(map[string]string)}
}

func (w *capturingOutputWriter) WriteSummary(content string) error {
	w.summaries = append(w.summaries, content)
	return nil
}

func (w *capturingOutputWriter) WriteOutput(key, value string) error {
	w.outputs[key] = value
	return nil
}

// fullCapturingProvider captures all CI operations: check runs AND output writes.
type fullCapturingProvider struct {
	mockProvider
	createCheckRunCalls []*provider.CreateCheckRunOptions
	updateCheckRunCalls []*provider.UpdateCheckRunOptions
	writer              *capturingOutputWriter
	nextID              int64
}

func newFullCapturingProvider(name string, detected bool) *fullCapturingProvider {
	return &fullCapturingProvider{
		mockProvider: mockProvider{name: name, detected: detected},
		writer:       newCapturingOutputWriter(),
	}
}

func (c *fullCapturingProvider) CreateCheckRun(_ context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	c.createCheckRunCalls = append(c.createCheckRunCalls, opts)
	c.nextID++
	return &provider.CheckRun{ID: c.nextID, Name: opts.Name, Status: opts.Status}, nil
}

func (c *fullCapturingProvider) UpdateCheckRun(_ context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	c.updateCheckRunCalls = append(c.updateCheckRunCalls, opts)
	return &provider.CheckRun{ID: opts.CheckRunID, Name: opts.Name, Status: opts.Status}, nil
}

func (c *fullCapturingProvider) OutputWriter() provider.OutputWriter {
	return c.writer
}

func (c *fullCapturingProvider) totalCICalls() int {
	return len(c.createCheckRunCalls) + len(c.updateCheckRunCalls) +
		len(c.writer.summaries) + len(c.writer.outputs)
}

// TestExecute_LocalDev_NoCIFlag_NoActions verifies that CI functionality
// (summaries, checks, outputs) is never triggered on a local developer
// machine when the --ci flag is not set and CI is not enabled in config.
func TestExecute_LocalDev_NoCIFlag_NoActions(t *testing.T) {
	// allBindings registers a plugin with all action types so any
	// accidental execution would be captured.
	allBindings := []plugin.HookBinding{
		{
			Event:   "before.terraform.plan",
			Actions: []plugin.HookAction{plugin.ActionCheck},
		},
		{
			Event:    "after.terraform.plan",
			Actions:  []plugin.HookAction{plugin.ActionSummary, plugin.ActionOutput, plugin.ActionUpload, plugin.ActionCheck},
			Template: "plan",
		},
	}

	events := []string{"before.terraform.plan", "after.terraform.plan"}

	t.Run("no CI flag, no CI config — no actions executed", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		cp := newFullCapturingProvider("generic", false)
		Register(cp)

		ClearPlugins()
		ctrl := gomock.NewController(t)
		mp := NewMockPlugin(ctrl)
		mp.EXPECT().GetType().Return("terraform").AnyTimes()
		mp.EXPECT().GetHookBindings().Return(allBindings).AnyTimes()
		mp.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
		mp.EXPECT().ParseOutput(gomock.Any(), gomock.Any()).Return(&plugin.OutputResult{}, nil).AnyTimes()
		require.NoError(t, RegisterPlugin(mp))

		for _, event := range events {
			err := Execute(ExecuteOptions{
				Event:       event,
				ForceCIMode: false,
				AtmosConfig: &schema.AtmosConfiguration{},
				Info: &schema.ConfigAndStacksInfo{
					Stack:            "dev-us-east-1",
					ComponentFromArg: "vpc",
				},
			})
			assert.NoError(t, err)
		}

		assert.Zero(t, cp.totalCICalls(),
			"No CI actions should execute on local dev without --ci flag")
	})

	t.Run("no CI flag, CI.Enabled explicitly false — no actions executed", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		cp := newFullCapturingProvider("generic", false)
		Register(cp)

		ClearPlugins()
		ctrl := gomock.NewController(t)
		mp := NewMockPlugin(ctrl)
		mp.EXPECT().GetType().Return("terraform").AnyTimes()
		mp.EXPECT().GetHookBindings().Return(allBindings).AnyTimes()
		mp.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
		mp.EXPECT().ParseOutput(gomock.Any(), gomock.Any()).Return(&plugin.OutputResult{}, nil).AnyTimes()
		require.NoError(t, RegisterPlugin(mp))

		for _, event := range events {
			err := Execute(ExecuteOptions{
				Event:       event,
				ForceCIMode: false,
				AtmosConfig: &schema.AtmosConfiguration{
					CI: schema.CIConfig{Enabled: false},
				},
				Info: &schema.ConfigAndStacksInfo{
					Stack:            "dev-us-east-1",
					ComponentFromArg: "vpc",
				},
			})
			assert.NoError(t, err)
		}

		assert.Zero(t, cp.totalCICalls(),
			"No CI actions should execute when CI is explicitly disabled")
	})

	t.Run("no CI flag, nil AtmosConfig — no actions executed", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		cp := newFullCapturingProvider("generic", false)
		Register(cp)

		ClearPlugins()
		ctrl := gomock.NewController(t)
		mp := NewMockPlugin(ctrl)
		mp.EXPECT().GetType().Return("terraform").AnyTimes()
		mp.EXPECT().GetHookBindings().Return(allBindings).AnyTimes()
		mp.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
		mp.EXPECT().ParseOutput(gomock.Any(), gomock.Any()).Return(&plugin.OutputResult{}, nil).AnyTimes()
		require.NoError(t, RegisterPlugin(mp))

		for _, event := range events {
			err := Execute(ExecuteOptions{
				Event:       event,
				ForceCIMode: false,
				AtmosConfig: nil,
				Info: &schema.ConfigAndStacksInfo{
					Stack:            "dev-us-east-1",
					ComponentFromArg: "vpc",
				},
			})
			assert.NoError(t, err)
		}

		assert.Zero(t, cp.totalCICalls(),
			"No CI actions should execute when AtmosConfig is nil")
	})

	t.Run("no CI flag, checks enabled but CI.Enabled false — no actions executed", func(t *testing.T) {
		// A user might enable checks in config but not CI itself.
		// Without --ci flag, nothing should run.
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		cp := newFullCapturingProvider("generic", false)
		Register(cp)

		ClearPlugins()
		ctrl := gomock.NewController(t)
		mp := NewMockPlugin(ctrl)
		mp.EXPECT().GetType().Return("terraform").AnyTimes()
		mp.EXPECT().GetHookBindings().Return(allBindings).AnyTimes()
		mp.EXPECT().GetDefaultTemplates().Return(embed.FS{}).AnyTimes()
		mp.EXPECT().ParseOutput(gomock.Any(), gomock.Any()).Return(&plugin.OutputResult{}, nil).AnyTimes()
		require.NoError(t, RegisterPlugin(mp))

		for _, event := range events {
			err := Execute(ExecuteOptions{
				Event:       event,
				ForceCIMode: false,
				AtmosConfig: &schema.AtmosConfiguration{
					CI: schema.CIConfig{
						Enabled: false,
						Checks:  schema.CIChecksConfig{Enabled: boolPtr(true)},
						Summary: schema.CISummaryConfig{Enabled: boolPtr(true)},
						Output:  schema.CIOutputConfig{Enabled: boolPtr(true)},
					},
				},
				Info: &schema.ConfigAndStacksInfo{
					Stack:            "dev-us-east-1",
					ComponentFromArg: "vpc",
				},
			})
			assert.NoError(t, err)
		}

		assert.Zero(t, cp.totalCICalls(),
			"No CI actions should execute when CI.Enabled is false, even if individual actions are enabled")
	})
}

func TestExecute(t *testing.T) {
	t.Run("returns nil when platform not detected and force mode disabled", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		err := Execute(ExecuteOptions{
			Event:       "after.terraform.plan",
			ForceCIMode: false,
		})
		assert.NoError(t, err)
	})

	t.Run("uses generic provider when force mode enabled and no platform detected", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)
		// Register a mock "generic" provider so Get("generic") succeeds.
		Register(&mockProvider{name: "generic", detected: false})

		// Force CI mode should use generic provider.
		err := Execute(ExecuteOptions{
			Event:       "after.terraform.plan",
			ForceCIMode: true,
			Info: &schema.ConfigAndStacksInfo{
				ComponentFromArg: "vpc",
				Stack:            "dev",
			},
		})
		// Should not error - generic provider will handle it.
		assert.NoError(t, err)
	})
}

func TestExtractComponentType(t *testing.T) {
	tests := []struct {
		event    string
		expected string
	}{
		{"after.terraform.plan", "terraform"},
		{"before.terraform.apply", "terraform"},
		{"after.helmfile.diff", "helmfile"},
		{"invalid", ""},
		{"single", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			result := extractComponentType(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractCommand(t *testing.T) {
	tests := []struct {
		event    string
		expected string
	}{
		{"after.terraform.plan", "plan"},
		{"before.terraform.apply", "apply"},
		{"after.helmfile.diff", "diff"},
		{"invalid", ""},
		{"single.part", ""},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			result := extractCommand(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// Note: mockComponentProvider is already defined in component_registry_test.go.

func TestHookActionConstants(t *testing.T) {
	// Verify action constant values match expected strings.
	assert.Equal(t, plugin.HookAction("summary"), plugin.ActionSummary)
	assert.Equal(t, plugin.HookAction("output"), plugin.ActionOutput)
	assert.Equal(t, plugin.HookAction("upload"), plugin.ActionUpload)
	assert.Equal(t, plugin.HookAction("download"), plugin.ActionDownload)
	assert.Equal(t, plugin.HookAction("check"), plugin.ActionCheck)
}

func TestOutputResult(t *testing.T) {
	t.Run("default values", func(t *testing.T) {
		result := &plugin.OutputResult{}
		assert.Equal(t, 0, result.ExitCode)
		assert.False(t, result.HasChanges)
		assert.False(t, result.HasErrors)
		assert.Nil(t, result.Errors)
		assert.Nil(t, result.Data)
	})

	t.Run("with terraform data", func(t *testing.T) {
		result := &plugin.OutputResult{
			ExitCode:   0,
			HasChanges: true,
			Data: &plugin.TerraformOutputData{
				ResourceCounts: plugin.ResourceCounts{
					Create:  5,
					Change:  3,
					Destroy: 1,
				},
			},
		}
		assert.True(t, result.HasChanges)
		tfData, ok := result.Data.(*plugin.TerraformOutputData)
		require.True(t, ok)
		assert.Equal(t, 5, tfData.ResourceCounts.Create)
		assert.Equal(t, 3, tfData.ResourceCounts.Change)
		assert.Equal(t, 1, tfData.ResourceCounts.Destroy)
	})
}

func TestResourceCounts(t *testing.T) {
	counts := plugin.ResourceCounts{
		Create:  10,
		Change:  5,
		Replace: 2,
		Destroy: 3,
	}

	assert.Equal(t, 10, counts.Create)
	assert.Equal(t, 5, counts.Change)
	assert.Equal(t, 2, counts.Replace)
	assert.Equal(t, 3, counts.Destroy)
}

func TestTemplateContext(t *testing.T) {
	ctx := &plugin.TemplateContext{
		Component:     "vpc",
		ComponentType: "terraform",
		Stack:         "dev-us-east-1",
		Command:       "plan",
		CI: &provider.Context{
			Provider: "github-actions",
			SHA:      "abc123",
		},
		Result: &plugin.OutputResult{
			HasChanges: true,
		},
		Output: "terraform plan output...",
		Custom: map[string]any{
			"custom_key": "custom_value",
		},
	}

	assert.Equal(t, "vpc", ctx.Component)
	assert.Equal(t, "terraform", ctx.ComponentType)
	assert.Equal(t, "dev-us-east-1", ctx.Stack)
	assert.Equal(t, "plan", ctx.Command)
	assert.NotNil(t, ctx.CI)
	assert.Equal(t, "github-actions", ctx.CI.Provider)
	assert.NotNil(t, ctx.Result)
	assert.True(t, ctx.Result.HasChanges)
	assert.Equal(t, "custom_value", ctx.Custom["custom_key"])
}

func TestMovedResource(t *testing.T) {
	moved := plugin.MovedResource{
		From: "aws_instance.old",
		To:   "aws_instance.new",
	}

	assert.Equal(t, "aws_instance.old", moved.From)
	assert.Equal(t, "aws_instance.new", moved.To)
}

func TestTerraformOutput(t *testing.T) {
	t.Run("string output", func(t *testing.T) {
		output := plugin.TerraformOutput{
			Value:     "vpc-12345",
			Type:      "string",
			Sensitive: false,
		}
		assert.Equal(t, "vpc-12345", output.Value)
		assert.Equal(t, "string", output.Type)
		assert.False(t, output.Sensitive)
	})

	t.Run("sensitive output", func(t *testing.T) {
		output := plugin.TerraformOutput{
			Value:     "secret-password",
			Type:      "string",
			Sensitive: true,
		}
		assert.True(t, output.Sensitive)
	})
}

func TestReleaseInfo(t *testing.T) {
	release := plugin.ReleaseInfo{
		Name:      "my-app",
		Namespace: "production",
		Status:    "deployed",
	}

	assert.Equal(t, "my-app", release.Name)
	assert.Equal(t, "production", release.Namespace)
	assert.Equal(t, "deployed", release.Status)
}

func TestHelmfileOutputData(t *testing.T) {
	data := &plugin.HelmfileOutputData{
		Releases: []plugin.ReleaseInfo{
			{Name: "app1", Namespace: "default", Status: "deployed"},
			{Name: "app2", Namespace: "kube-system", Status: "pending"},
		},
	}

	assert.Len(t, data.Releases, 2)
	assert.Equal(t, "app1", data.Releases[0].Name)
	assert.Equal(t, "app2", data.Releases[1].Name)
}

func TestTerraformOutputData(t *testing.T) {
	data := &plugin.TerraformOutputData{
		ResourceCounts: plugin.ResourceCounts{Create: 5, Change: 3, Destroy: 1},
		CreatedResources: []string{
			"aws_vpc.main",
			"aws_subnet.private[0]",
		},
		UpdatedResources: []string{
			"aws_security_group.web",
		},
		ReplacedResources: []string{
			"aws_instance.web",
		},
		DeletedResources: []string{
			"aws_eip.old",
		},
		MovedResources: []plugin.MovedResource{
			{From: "aws_instance.old", To: "module.compute.aws_instance.main"},
		},
		ImportedResources: []string{
			"aws_s3_bucket.existing",
		},
		Outputs: map[string]plugin.TerraformOutput{
			"vpc_id": {Value: "vpc-123", Type: "string"},
		},
		ChangedResult: "Plan: 5 to add, 3 to change, 1 to destroy.",
	}

	assert.Equal(t, 5, data.ResourceCounts.Create)
	assert.Len(t, data.CreatedResources, 2)
	assert.Len(t, data.UpdatedResources, 1)
	assert.Len(t, data.ReplacedResources, 1)
	assert.Len(t, data.DeletedResources, 1)
	assert.Len(t, data.MovedResources, 1)
	assert.Len(t, data.ImportedResources, 1)
	assert.Len(t, data.Outputs, 1)
	assert.Contains(t, data.ChangedResult, "5 to add")
}

func TestIsActionEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		action   plugin.HookAction
		expected bool
	}{
		{
			name:     "nil config - summary enabled by default",
			config:   nil,
			action:   plugin.ActionSummary,
			expected: true,
		},
		{
			name:     "nil config - output enabled by default",
			config:   nil,
			action:   plugin.ActionOutput,
			expected: true,
		},
		{
			name:     "nil config - check disabled by default",
			config:   nil,
			action:   plugin.ActionCheck,
			expected: false,
		},
		{
			name:     "nil config - upload always enabled",
			config:   nil,
			action:   plugin.ActionUpload,
			expected: true,
		},
		{
			name:     "nil config - download always enabled",
			config:   nil,
			action:   plugin.ActionDownload,
			expected: true,
		},
		{
			name: "summary explicitly enabled",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Summary: schema.CISummaryConfig{Enabled: boolPtr(true)},
				},
			},
			action:   plugin.ActionSummary,
			expected: true,
		},
		{
			name: "summary explicitly disabled",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
				},
			},
			action:   plugin.ActionSummary,
			expected: false,
		},
		{
			name: "output explicitly enabled",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Output: schema.CIOutputConfig{Enabled: boolPtr(true)},
				},
			},
			action:   plugin.ActionOutput,
			expected: true,
		},
		{
			name: "output explicitly disabled",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Output: schema.CIOutputConfig{Enabled: boolPtr(false)},
				},
			},
			action:   plugin.ActionOutput,
			expected: false,
		},
		{
			name: "check explicitly enabled",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Checks: schema.CIChecksConfig{Enabled: boolPtr(true)},
				},
			},
			action:   plugin.ActionCheck,
			expected: true,
		},
		{
			name: "check explicitly disabled",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Checks: schema.CIChecksConfig{Enabled: boolPtr(false)},
				},
			},
			action:   plugin.ActionCheck,
			expected: false,
		},
		{
			name: "upload always enabled regardless of config",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{},
			},
			action:   plugin.ActionUpload,
			expected: true,
		},
		{
			name: "download always enabled regardless of config",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{},
			},
			action:   plugin.ActionDownload,
			expected: true,
		},
		{
			name: "unknown action enabled by default",
			config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{},
			},
			action:   plugin.HookAction("unknown"),
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isActionEnabled(tt.config, tt.action)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExtractEventPrefix(t *testing.T) {
	tests := []struct {
		event    string
		expected string
	}{
		{"before.terraform.plan", "before"},
		{"after.terraform.plan", "after"},
		{"after.helmfile.diff", "after"},
		{"single", "single"},
		{"", ""},
	}

	for _, tt := range tests {
		t.Run(tt.event, func(t *testing.T) {
			result := extractEventPrefix(tt.event)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestExecuteCheckAction_BeforeEvent(t *testing.T) {
	mp := &mockProvider{name: "test", detected: true}

	ctx := &actionContext{
		Opts: ExecuteOptions{
			Event: "before.terraform.plan",
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev-us-east-1",
				ComponentFromArg: "vpc",
			},
		},
		Platform: mp,
		CICtx: &provider.Context{
			RepoOwner: "owner",
			RepoName:  "repo",
			SHA:       "abc123",
		},
		Command: "plan",
	}

	err := executeCheckAction(ctx)
	require.NoError(t, err)

	// Verify the check run ID was stored.
	key := buildCheckRunKey(ctx)
	idVal, ok := checkRunIDs.LoadAndDelete(key)
	assert.True(t, ok)
	assert.Equal(t, int64(1), idVal)
}

func TestExecuteCheckAction_AfterEvent_WithStoredID(t *testing.T) {
	mp := &mockProvider{name: "test", detected: true}

	ctx := &actionContext{
		Opts: ExecuteOptions{
			Event: "after.terraform.plan",
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev-us-east-1",
				ComponentFromArg: "vpc",
			},
		},
		Platform: mp,
		CICtx: &provider.Context{
			RepoOwner: "owner",
			RepoName:  "repo",
		},
		Command: "plan",
		Result: &plugin.OutputResult{
			HasChanges: true,
			Data: &plugin.TerraformOutputData{
				ChangedResult: "Plan: 3 to add, 1 to change, 0 to destroy.",
			},
		},
	}

	// Pre-store a check run ID (simulating a "before" event).
	key := buildCheckRunKey(ctx)
	checkRunIDs.Store(key, int64(42))

	err := executeCheckAction(ctx)
	require.NoError(t, err)

	// Verify the stored ID was consumed.
	_, ok := checkRunIDs.Load(key)
	assert.False(t, ok)
}

func TestExecuteCheckAction_AfterEvent_NoStoredID(t *testing.T) {
	mp := &mockProvider{name: "test", detected: true}

	ctx := &actionContext{
		Opts: ExecuteOptions{
			Event: "after.terraform.plan",
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "prod-us-west-2",
				ComponentFromArg: "rds",
			},
		},
		Platform: mp,
		CICtx: &provider.Context{
			RepoOwner: "owner",
			RepoName:  "repo",
			SHA:       "def456",
		},
		Command: "plan",
		Result:  &plugin.OutputResult{},
	}

	// No pre-stored ID — should fall back to creating a completed check run.
	err := executeCheckAction(ctx)
	require.NoError(t, err)
}

func TestBuildCheckTitle(t *testing.T) {
	t.Run("with terraform changed result", func(t *testing.T) {
		ctx := &actionContext{
			Command: "plan",
			Result: &plugin.OutputResult{
				HasChanges: true,
				Data: &plugin.TerraformOutputData{
					ChangedResult: "Plan: 5 to add, 2 to change, 1 to destroy.",
				},
			},
		}
		assert.Equal(t, "Plan: 5 to add, 2 to change, 1 to destroy.", buildCheckTitle(ctx))
	})

	t.Run("with changes but no terraform data", func(t *testing.T) {
		ctx := &actionContext{
			Command: "plan",
			Result: &plugin.OutputResult{
				HasChanges: true,
			},
		}
		assert.Equal(t, "plan: changes detected", buildCheckTitle(ctx))
	})

	t.Run("no changes", func(t *testing.T) {
		ctx := &actionContext{
			Command: "plan",
			Result:  &plugin.OutputResult{},
		}
		assert.Equal(t, "plan: no changes", buildCheckTitle(ctx))
	})

	t.Run("nil result", func(t *testing.T) {
		ctx := &actionContext{
			Command: "plan",
		}
		assert.Equal(t, "plan: no changes", buildCheckTitle(ctx))
	})
}

func TestBuildCheckSummary(t *testing.T) {
	t.Run("with terraform changed result", func(t *testing.T) {
		ctx := &actionContext{
			Result: &plugin.OutputResult{
				Data: &plugin.TerraformOutputData{
					ChangedResult: "Plan: 3 to add, 0 to change, 0 to destroy.",
				},
			},
		}
		assert.Equal(t, "Plan: 3 to add, 0 to change, 0 to destroy.", buildCheckSummary(ctx))
	})

	t.Run("nil result returns empty", func(t *testing.T) {
		ctx := &actionContext{}
		assert.Empty(t, buildCheckSummary(ctx))
	})

	t.Run("no terraform data returns empty", func(t *testing.T) {
		ctx := &actionContext{
			Result: &plugin.OutputResult{},
		}
		assert.Empty(t, buildCheckSummary(ctx))
	})
}

func TestResolveCheckResult(t *testing.T) {
	t.Run("success when no command error", func(t *testing.T) {
		ctx := &actionContext{
			Result: &plugin.OutputResult{HasChanges: true},
		}
		status, conclusion := resolveCheckResult(ctx)
		assert.Equal(t, provider.CheckRunStateSuccess, status)
		assert.Equal(t, "success", conclusion)
	})

	t.Run("failure when command error is set", func(t *testing.T) {
		ctx := &actionContext{
			Opts: ExecuteOptions{
				CommandError: fmt.Errorf("terraform plan failed"),
			},
			Result: &plugin.OutputResult{HasErrors: true},
		}
		status, conclusion := resolveCheckResult(ctx)
		assert.Equal(t, provider.CheckRunStateFailure, status)
		assert.Equal(t, "failure", conclusion)
	})
}

func TestBuildCheckRunKey(t *testing.T) {
	ctx := &actionContext{
		Opts: ExecuteOptions{
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev-us-east-1",
				ComponentFromArg: "vpc",
			},
		},
		Command: "plan",
	}
	assert.Equal(t, "dev-us-east-1/vpc/plan", buildCheckRunKey(ctx))
}

func TestFilterVariables(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]string
		allowed  []string
		expected map[string]string
	}{
		{
			name: "empty allowed list returns all vars",
			vars: map[string]string{
				"has_changes":  "true",
				"artifact_key": "my-key",
			},
			allowed: []string{},
			expected: map[string]string{
				"has_changes":  "true",
				"artifact_key": "my-key",
			},
		},
		{
			name: "nil allowed list returns all vars",
			vars: map[string]string{
				"has_changes":  "true",
				"artifact_key": "my-key",
			},
			allowed: nil,
			expected: map[string]string{
				"has_changes":  "true",
				"artifact_key": "my-key",
			},
		},
		{
			name: "filters to allowed variables only",
			vars: map[string]string{
				"has_changes":   "true",
				"artifact_key":  "my-key",
				"has_additions": "true",
				"plan_summary":  "3 to add",
			},
			allowed: []string{"has_changes", "plan_summary"},
			expected: map[string]string{
				"has_changes":  "true",
				"plan_summary": "3 to add",
			},
		},
		{
			name: "allowed variable not in vars is not added",
			vars: map[string]string{
				"has_changes": "true",
			},
			allowed: []string{"has_changes", "nonexistent"},
			expected: map[string]string{
				"has_changes": "true",
			},
		},
		{
			name:     "empty vars returns empty result",
			vars:     map[string]string{},
			allowed:  []string{"has_changes"},
			expected: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterVariables(tt.vars, tt.allowed)
			assert.Equal(t, tt.expected, result)
		})
	}
}
