package executor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
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
	c.nextID++
	return &provider.CheckRun{
		ID:     c.nextID,
		Name:   opts.Name,
		Status: opts.Status,
	}, nil
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

func newFullCapturingProvider() *fullCapturingProvider {
	return &fullCapturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
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
	c.nextID++
	return &provider.CheckRun{ID: c.nextID, Name: opts.Name, Status: opts.Status}, nil
}

func (c *fullCapturingProvider) OutputWriter() provider.OutputWriter {
	return c.writer
}

func (c *fullCapturingProvider) totalCICalls() int {
	return len(c.createCheckRunCalls) + len(c.updateCheckRunCalls) +
		len(c.writer.summaries) + len(c.writer.outputs)
}

// resetRegistries clears both provider and plugin registries for test isolation.
func resetRegistries(t *testing.T) {
	t.Helper()
	ci.Reset()
	ci.ClearPlugins()
	t.Cleanup(func() {
		ci.Reset()
		ci.ClearPlugins()
	})
}

func TestExecute_BeforeTerraformPlan_TriggersCheckCreate(t *testing.T) {
	resetRegistries(t)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	ci.Register(cp)

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
					_, err := ctx.Provider.CreateCheckRun(context.Background(), opts)
					return err
				},
			},
		},
	}
	require.NoError(t, ci.RegisterPlugin(sp))

	// Execute with CI forced, checks enabled.
	err := ci.Execute(&ci.ExecuteOptions{
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
	resetRegistries(t)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	ci.Register(cp)

	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{
				Event: "before.terraform.plan",
				Handler: func(ctx *plugin.HookContext) error {
					name := provider.FormatCheckRunName(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
					_, err := ctx.Provider.CreateCheckRun(context.Background(), &provider.CreateCheckRunOptions{
						Name:   name,
						Status: provider.CheckRunStateInProgress,
					})
					return err
				},
			},
			{
				Event: "after.terraform.plan",
				Handler: func(ctx *plugin.HookContext) error {
					name := provider.FormatCheckRunName(ctx.Command, ctx.Info.Stack, ctx.Info.ComponentFromArg)
					_, err := ctx.Provider.UpdateCheckRun(context.Background(), &provider.UpdateCheckRunOptions{
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
	require.NoError(t, ci.RegisterPlugin(sp))

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
	err := ci.Execute(&ci.ExecuteOptions{
		Event:       "before.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: atmosConfig,
		Info:        info,
	})
	require.NoError(t, err)
	require.Len(t, cp.createCheckRunCalls, 1, "CreateCheckRun should have been called once after before event")
	assert.Equal(t, provider.CheckRunStateInProgress, cp.createCheckRunCalls[0].Status)

	// 2. Execute "after.terraform.plan" — should update the check run, not create a new one.
	err = ci.Execute(&ci.ExecuteOptions{
		Event:       "after.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: atmosConfig,
		Info:        info,
		Output:      "Terraform will perform the following actions...",
	})
	require.NoError(t, err)

	// Verify UpdateCheckRun was called (not a second CreateCheckRun).
	require.Len(t, cp.updateCheckRunCalls, 1, "UpdateCheckRun should have been called once after after event")
	assert.Equal(t, provider.CheckRunStateSuccess, cp.updateCheckRunCalls[0].Status)
	assert.Equal(t, "success", cp.updateCheckRunCalls[0].Conclusion)
	assert.Equal(t, "Plan: 1 to add, 0 to change, 0 to destroy.", cp.updateCheckRunCalls[0].Title)

	// CreateCheckRun should NOT have been called again.
	assert.Len(t, cp.createCheckRunCalls, 1, "CreateCheckRun should not be called during after event when ID is stored")
}

func TestExecute_CIEnabledInConfig_NoForceFlag_NoActions(t *testing.T) {
	// When CI.Enabled=true in atmos.yaml but --ci flag is NOT passed and
	// no CI platform is detected, Execute should be a no-op.
	resetRegistries(t)

	cp := &capturingProvider{
		mockProvider: mockProvider{name: "generic", detected: false},
	}
	ci.Register(cp)

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
	require.NoError(t, ci.RegisterPlugin(sp))

	// Execute WITHOUT ForceCIMode — CI is enabled only via config.
	err := ci.Execute(&ci.ExecuteOptions{
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
		resetRegistries(t)

		cp := newFullCapturingProvider()
		ci.Register(cp)

		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, ci.RegisterPlugin(sp))

		for _, event := range events {
			err := ci.Execute(&ci.ExecuteOptions{
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
		resetRegistries(t)

		cp := newFullCapturingProvider()
		ci.Register(cp)

		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, ci.RegisterPlugin(sp))

		for _, event := range events {
			err := ci.Execute(&ci.ExecuteOptions{
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
		resetRegistries(t)

		cp := newFullCapturingProvider()
		ci.Register(cp)

		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, ci.RegisterPlugin(sp))

		for _, event := range events {
			err := ci.Execute(&ci.ExecuteOptions{
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
		resetRegistries(t)

		cp := newFullCapturingProvider()
		ci.Register(cp)

		sp := &stubPlugin{componentType: "terraform", bindings: createAllBindings()}
		require.NoError(t, ci.RegisterPlugin(sp))

		for _, event := range events {
			err := ci.Execute(&ci.ExecuteOptions{
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
		resetRegistries(t)

		err := ci.Execute(&ci.ExecuteOptions{
			Event:       "after.terraform.plan",
			ForceCIMode: false,
		})
		assert.NoError(t, err)
	})

	t.Run("uses generic provider when force mode enabled and no platform detected", func(t *testing.T) {
		resetRegistries(t)
		// Register a mock "generic" provider so Get("generic") succeeds.
		ci.Register(&mockProvider{name: "generic", detected: false})

		// Force CI mode should use generic provider.
		err := ci.Execute(&ci.ExecuteOptions{
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

func TestExecute_NilHandler(t *testing.T) {
	resetRegistries(t)

	ci.Register(&mockProvider{name: "generic", detected: false})

	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: nil}, // No handler.
		},
	}
	require.NoError(t, ci.RegisterPlugin(sp))

	err := ci.Execute(&ci.ExecuteOptions{
		Event:       "after.terraform.plan",
		ForceCIMode: true,
		Info:        &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
	})
	assert.NoError(t, err)
}

func TestExecute_HandlerError_DoesNotPropagateToExecute(t *testing.T) {
	resetRegistries(t)

	ci.Register(&mockProvider{name: "generic", detected: false})

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
	require.NoError(t, ci.RegisterPlugin(sp))

	// Handler errors are logged as warnings, not propagated.
	err := ci.Execute(&ci.ExecuteOptions{
		Event:       "after.terraform.plan",
		ForceCIMode: true,
		AtmosConfig: &schema.AtmosConfiguration{},
		Info:        &schema.ConfigAndStacksInfo{Stack: "dev", ComponentFromArg: "vpc"},
	})
	assert.NoError(t, err)
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
	resetRegistries(t)

	fcp := newFullCapturingProvider()
	ci.Register(fcp)

	// Set up plugin with a handler that renders summary and writes outputs.
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
	require.NoError(t, ci.RegisterPlugin(sp))

	// Execute with CI forced, using template base_path pointing to temp dir.
	err := ci.Execute(&ci.ExecuteOptions{
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
