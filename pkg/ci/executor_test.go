package ci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// stubPlugin is a simple Plugin for tests that uses Handler callbacks.
type stubPlugin struct {
	componentType string
	bindings      []plugin.HookBinding
}

func (s *stubPlugin) GetType() string {
	return s.componentType
}

func (s *stubPlugin) GetHookBindings() []plugin.HookBinding {
	return s.bindings
}

func TestExecute_BeforeTerraformPlan_TriggersCheckCreate(t *testing.T) {
	// Set up provider registry with a capturing provider.
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	Register(cp)

	// Set up plugin with a handler that creates a check run.
	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{
				Event: "before.terraform.plan",
				Handler: func(ctx *plugin.HookContext) error {
					// Simulate check run creation.
					name := provider.FormatCheckRunName(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
					opts := &provider.CreateCheckRunOptions{
						Name:   name,
						Status: provider.CheckRunStateInProgress,
						Title:  fmt.Sprintf("Running %s...", ctx.Command),
					}
					if ctx.CICtx != nil {
						opts.Owner = ctx.CICtx.RepoOwner
						opts.Repo = ctx.CICtx.RepoName
						opts.SHA = ctx.CICtx.SHA
					}
					checkRun, err := ctx.Provider.CreateCheckRun(context.Background(), opts)
					if err != nil {
						return err
					}
					ctx.CheckRunStore.Store(ctx.Info.Stack+"/"+ctx.Info.ComponentFromArg+"/"+ctx.Command, checkRun.ID)
					return nil
				},
			},
		},
	}
	err := RegisterPlugin(sp)
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

	// Set up plugin with before and after plan handlers that manage check runs.
	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{
				Event: "before.terraform.plan",
				Handler: func(ctx *plugin.HookContext) error {
					name := provider.FormatCheckRunName(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
					checkRun, err := ctx.Provider.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{
						Name:   name,
						Status: provider.CheckRunStateInProgress,
					})
					if err != nil {
						return err
					}
					ctx.CheckRunStore.Store(ctx.Info.Stack+"/"+ctx.Info.ComponentFromArg+"/"+ctx.Command, checkRun.ID)
					return nil
				},
			},
			{
				Event: "after.terraform.plan",
				Handler: func(ctx *plugin.HookContext) error {
					name := provider.FormatCheckRunName(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
					key := ctx.Info.Stack + "/" + ctx.Info.ComponentFromArg + "/" + ctx.Command
					checkRunID, ok := ctx.CheckRunStore.LoadAndDelete(key)
					if !ok {
						return fmt.Errorf("no check run ID found")
					}
					_, err := ctx.Provider.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{
						CheckRunID: checkRunID,
						Name:       name,
						Status:     provider.CheckRunStateSuccess,
						Conclusion: "success",
						Title:      "Plan: 1 to add, 0 to change, 0 to destroy.",
					})
					return err
				},
			},
		},
	}
	err := RegisterPlugin(sp)
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
	assert.Equal(t, provider.CheckRunStateInProgress, cp.createCheckRunCalls[0].Status)

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
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	Register(cp)

	ClearPlugins()
	handlerCalled := false
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{
				Event: "before.terraform.plan",
				Handler: func(_ *plugin.HookContext) error {
					handlerCalled = true
					return nil
				},
			},
		},
	}
	err := RegisterPlugin(sp)
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
	assert.False(t, handlerCalled, "Handler should NOT be called without --ci flag even when CI.Enabled=true in config")
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
	events := []string{"before.terraform.plan", "after.terraform.plan"}

	createAllBindings := func() []plugin.HookBinding {
		handlerCalled := false
		return []plugin.HookBinding{
			{
				Event: "before.terraform.plan",
				Handler: func(_ *plugin.HookContext) error {
					handlerCalled = true
					_ = handlerCalled
					return nil
				},
			},
			{
				Event: "after.terraform.plan",
				Handler: func(_ *plugin.HookContext) error {
					handlerCalled = true
					_ = handlerCalled
					return nil
				},
			},
		}
	}

	t.Run("no CI flag, no CI config — no actions executed", func(t *testing.T) {
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		cp := newFullCapturingProvider("generic", false)
		Register(cp)

		ClearPlugins()
		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, RegisterPlugin(sp))

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
		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, RegisterPlugin(sp))

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
		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, RegisterPlugin(sp))

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
		backup := testSaveAndClearRegistry()
		defer testRestoreRegistry(backup)

		cp := newFullCapturingProvider("generic", false)
		Register(cp)

		ClearPlugins()
		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, RegisterPlugin(sp))

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

func TestGetPluginAndBinding_EmptyEvent(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		},
	}
	require.NoError(t, RegisterPlugin(sp))

	// Empty event should return nil.
	pl, binding := getPluginAndBinding(ExecuteOptions{Event: ""})
	assert.Nil(t, pl)
	assert.Nil(t, binding)
}

func TestGetPluginAndBinding_UnregisteredComponentType(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)
	ClearPlugins()

	// No plugins registered.
	pl, binding := getPluginAndBinding(ExecuteOptions{Event: "after.helmfile.diff"})
	assert.Nil(t, pl)
	assert.Nil(t, binding)
}

func TestGetPluginAndBinding_EventNotHandled(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		},
	}
	require.NoError(t, RegisterPlugin(sp))

	// Event exists for plugin type but specific event not handled.
	pl, binding := getPluginAndBinding(ExecuteOptions{Event: "after.terraform.init"})
	assert.Nil(t, pl)
	assert.Nil(t, binding)
}

func TestGetPluginAndBinding_ComponentTypeOverride(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		},
	}
	require.NoError(t, RegisterPlugin(sp))

	// Use ComponentType override instead of extracting from event.
	pl, binding := getPluginAndBinding(ExecuteOptions{
		Event:         "after.terraform.plan",
		ComponentType: "terraform",
	})
	assert.NotNil(t, pl)
	assert.NotNil(t, binding)
}

func TestExecute_NilHandler(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	Register(&mockProvider{name: "generic", detected: false})

	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: nil}, // No handler.
		},
	}
	require.NoError(t, RegisterPlugin(sp))

	err := Execute(ExecuteOptions{
		Event:       "after.terraform.plan",
		ForceCIMode: true,
		Info:        &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
	})
	assert.NoError(t, err)
}

func TestExecute_HandlerError_DoesNotPropagateToExecute(t *testing.T) {
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	Register(&mockProvider{name: "generic", detected: false})

	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{
				Event: "after.terraform.plan",
				Handler: func(_ *plugin.HookContext) error {
					return fmt.Errorf("handler failed")
				},
			},
		},
	}
	require.NoError(t, RegisterPlugin(sp))

	// Handler errors are logged as warnings, not propagated.
	err := Execute(ExecuteOptions{
		Event:       "after.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
	})
	assert.NoError(t, err)
}

func TestDetectStoreFromEnv(t *testing.T) {
	t.Run("no env vars returns nil", func(t *testing.T) {
		// Clear relevant env vars.
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		result := detectStoreFromEnv()
		assert.Nil(t, result)
	})

	t.Run("S3 bucket configured", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "my-bucket")
		t.Setenv("ATMOS_PLANFILE_PREFIX", "plans/")
		t.Setenv("AWS_REGION", "us-east-1")
		t.Setenv("GITHUB_ACTIONS", "")

		result := detectStoreFromEnv()
		require.NotNil(t, result)
		assert.Equal(t, "s3", result.Type)
		assert.Equal(t, "my-bucket", result.Options["bucket"])
		assert.Equal(t, "plans/", result.Options["prefix"])
		assert.Equal(t, "us-east-1", result.Options["region"])
	})

	t.Run("GitHub Actions detected", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "true")

		result := detectStoreFromEnv()
		require.NotNil(t, result)
		assert.Equal(t, "github-artifacts", result.Type)
	})

	t.Run("S3 takes precedence over GitHub", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "bucket")
		t.Setenv("GITHUB_ACTIONS", "true")

		result := detectStoreFromEnv()
		require.NotNil(t, result)
		assert.Equal(t, "s3", result.Type)
	})
}

func TestCreatePlanfileStore(t *testing.T) {
	t.Run("defaults to local store", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		store, err := createPlanfileStore(ExecuteOptions{
			AtmosConfig: &schema.AtmosConfiguration{},
		})
		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, "local", store.Name())
	})

	t.Run("nil config defaults to local store", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		store, err := createPlanfileStore(ExecuteOptions{
			AtmosConfig: nil,
		})
		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, "local", store.Name())
	})
}

func TestBuildHookContext(t *testing.T) {
	mp := &mockProvider{name: "test", detected: true}

	opts := ExecuteOptions{
		Event:       "after.terraform.plan",
		AtmosConfig: &schema.AtmosConfiguration{},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
		Output:       "some output",
		CommandError: fmt.Errorf("some error"),
	}

	ctx := buildHookContext(opts, mp)

	assert.Equal(t, "after.terraform.plan", ctx.Event)
	assert.Equal(t, "plan", ctx.Command)
	assert.Equal(t, "after", ctx.EventPrefix)
	assert.Equal(t, opts.AtmosConfig, ctx.Config)
	assert.Equal(t, opts.Info, ctx.Info)
	assert.Equal(t, "some output", ctx.Output)
	assert.Equal(t, opts.CommandError, ctx.CommandError)
	assert.Equal(t, mp, ctx.Provider)
	assert.NotNil(t, ctx.TemplateLoader)
	assert.NotNil(t, ctx.CheckRunStore)
	assert.NotNil(t, ctx.CreatePlanfileStore)
}

func TestExecute_SummaryContentFlowsToOutput(t *testing.T) {
	// Verify that the rendered summary markdown from the summary handler
	// is available as a "summary" output variable.

	// Create a temp directory with a template file.
	tmpDir := t.TempDir()
	terraformDir := filepath.Join(tmpDir, "terraform")
	require.NoError(t, os.MkdirAll(terraformDir, 0o755))
	templateContent := "## Plan for `{{.Component}}` in `{{.Stack}}`\n"
	require.NoError(t, os.WriteFile(filepath.Join(terraformDir, "plan.md"), []byte(templateContent), 0o644))

	// Set up provider registry with a full capturing provider.
	backup := testSaveAndClearRegistry()
	defer testRestoreRegistry(backup)

	fcp := newFullCapturingProvider("generic", false)
	Register(fcp)

	// Set up plugin with a handler that renders summary and writes outputs.
	ClearPlugins()
	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{
				Event: "after.terraform.plan",
				Handler: func(ctx *plugin.HookContext) error {
					writer := ctx.Provider.OutputWriter()
					if writer == nil {
						return nil
					}

					// Render summary template.
					tmplCtx := map[string]string{"Component": ctx.Info.ComponentFromArg, "Stack": ctx.Info.Stack}
					rendered, err := ctx.TemplateLoader.LoadAndRender("terraform", "plan", nil, tmplCtx)
					if err != nil {
						return err
					}

					// Write summary.
					if err := writer.WriteSummary(rendered); err != nil {
						return err
					}

					// Write outputs.
					vars := map[string]string{
						"has_changes": "true",
						"summary":     rendered,
						"stack":       ctx.Info.Stack,
						"component":   ctx.Info.ComponentFromArg,
						"command":     ctx.Command,
					}
					for key, value := range vars {
						if err := writer.WriteOutput(key, value); err != nil {
							return err
						}
					}

					return nil
				},
			},
		},
	}
	require.NoError(t, RegisterPlugin(sp))

	// Execute with CI forced, using template base_path pointing to temp dir.
	err := Execute(ExecuteOptions{
		Event:       "after.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Templates: schema.CITemplatesConfig{
					BasePath: tmpDir,
				},
			},
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev-us-east-1",
			ComponentFromArg: "vpc",
		},
		Output: "some terraform output",
	})
	require.NoError(t, err)

	// Verify summary was written.
	require.Len(t, fcp.writer.summaries, 1, "WriteSummary should have been called once")
	assert.Contains(t, fcp.writer.summaries[0], "vpc")
	assert.Contains(t, fcp.writer.summaries[0], "dev")

	// Verify the "summary" output variable contains the rendered summary markdown.
	summaryOutput, ok := fcp.writer.outputs["summary"]
	assert.True(t, ok, "output variables should include 'summary'")
	assert.Equal(t, fcp.writer.summaries[0], summaryOutput, "summary output should match rendered summary")
}
