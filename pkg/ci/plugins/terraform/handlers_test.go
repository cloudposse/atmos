package terraform

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/ci/templates"
	"github.com/cloudposse/atmos/pkg/schema"
)

// boolPtr returns a pointer to a bool value.
func boolPtr(b bool) *bool {
	return &b
}

// mockOutputWriter captures WriteSummary and WriteOutput calls.
type mockOutputWriter struct {
	summaries []string
	outputs   map[string]string
}

func newMockOutputWriter() *mockOutputWriter {
	return &mockOutputWriter{outputs: make(map[string]string)}
}

func (w *mockOutputWriter) WriteSummary(content string) error {
	w.summaries = append(w.summaries, content)
	return nil
}

func (w *mockOutputWriter) WriteOutput(key, value string) error {
	w.outputs[key] = value
	return nil
}

// mockProvider implements provider.Provider for testing.
type mockProvider struct {
	writer         *mockOutputWriter
	checkRunCalls  []*provider.CreateCheckRunOptions
	updateRunCalls []*provider.UpdateCheckRunOptions
	nextID         int64
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		writer: newMockOutputWriter(),
	}
}

func (m *mockProvider) Name() string                        { return "test" }
func (m *mockProvider) Detect() bool                        { return true }
func (m *mockProvider) Context() (*provider.Context, error) { return &provider.Context{}, nil }
func (m *mockProvider) OutputWriter() provider.OutputWriter { return m.writer }
func (m *mockProvider) GetStatus(_ context.Context, _ provider.StatusOptions) (*provider.Status, error) {
	return &provider.Status{}, nil
}

func (m *mockProvider) CreateCheckRun(_ context.Context, opts *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	m.checkRunCalls = append(m.checkRunCalls, opts)
	m.nextID++
	return &provider.CheckRun{ID: m.nextID, Name: opts.Name, Status: opts.Status}, nil
}

func (m *mockProvider) UpdateCheckRun(_ context.Context, opts *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	m.updateRunCalls = append(m.updateRunCalls, opts)
	m.nextID++
	return &provider.CheckRun{ID: m.nextID, Name: opts.Name, Status: opts.Status}, nil
}

func (m *mockProvider) ResolveBase() (*provider.BaseResolution, error) {
	return nil, nil
}

func TestIsSummaryEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{"nil config - enabled by default", nil, true},
		{"nil enabled - enabled by default", &schema.AtmosConfiguration{}, true},
		{"explicitly enabled", &schema.AtmosConfiguration{
			CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: boolPtr(true)}},
		}, true},
		{"explicitly disabled", &schema.AtmosConfiguration{
			CI: schema.CIConfig{Summary: schema.CISummaryConfig{Enabled: boolPtr(false)}},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isSummaryEnabled(tt.config))
		})
	}
}

func TestIsOutputEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{"nil config - enabled by default", nil, true},
		{"nil enabled - enabled by default", &schema.AtmosConfiguration{}, true},
		{"explicitly enabled", &schema.AtmosConfiguration{
			CI: schema.CIConfig{Output: schema.CIOutputConfig{Enabled: boolPtr(true)}},
		}, true},
		{"explicitly disabled", &schema.AtmosConfiguration{
			CI: schema.CIConfig{Output: schema.CIOutputConfig{Enabled: boolPtr(false)}},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isOutputEnabled(tt.config))
		})
	}
}

func TestIsCheckEnabled(t *testing.T) {
	tests := []struct {
		name     string
		config   *schema.AtmosConfiguration
		expected bool
	}{
		{"nil config - disabled by default", nil, false},
		{"nil enabled - disabled by default", &schema.AtmosConfiguration{}, false},
		{"explicitly enabled", &schema.AtmosConfiguration{
			CI: schema.CIConfig{Checks: schema.CIChecksConfig{Enabled: boolPtr(true)}},
		}, true},
		{"explicitly disabled", &schema.AtmosConfiguration{
			CI: schema.CIConfig{Checks: schema.CIChecksConfig{Enabled: boolPtr(false)}},
		}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, isCheckEnabled(tt.config))
		})
	}
}

func TestFilterVariables(t *testing.T) {
	tests := []struct {
		name     string
		vars     map[string]string
		allowed  []string
		expected map[string]string
	}{
		{
			name:     "empty allowed returns all",
			vars:     map[string]string{"a": "1", "b": "2"},
			allowed:  nil,
			expected: map[string]string{"a": "1", "b": "2"},
		},
		{
			name:     "filters to allowed only",
			vars:     map[string]string{"a": "1", "b": "2", "c": "3"},
			allowed:  []string{"a", "c"},
			expected: map[string]string{"a": "1", "c": "3"},
		},
		{
			name:     "allowed not in vars is not added",
			vars:     map[string]string{"a": "1"},
			allowed:  []string{"a", "nonexistent"},
			expected: map[string]string{"a": "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := filterVariables(tt.vars, tt.allowed)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestResolveCheckResult(t *testing.T) {
	t.Run("success when no command error", func(t *testing.T) {
		ctx := &plugin.HookContext{}
		status, conclusion := resolveCheckResult(ctx)
		assert.Equal(t, provider.CheckRunStateSuccess, status)
		assert.Equal(t, "success", conclusion)
	})

	t.Run("failure when command error is set", func(t *testing.T) {
		ctx := &plugin.HookContext{
			CommandError: fmt.Errorf("terraform plan failed"),
		}
		status, conclusion := resolveCheckResult(ctx)
		assert.Equal(t, provider.CheckRunStateFailure, status)
		assert.Equal(t, "failure", conclusion)
	})
}

func TestBuildStatusDescription(t *testing.T) {
	t.Run("with terraform changed result", func(t *testing.T) {
		result := &plugin.OutputResult{
			HasChanges: true,
			Data: &plugin.TerraformOutputData{
				ChangedResult: "5 to add, 2 to change, 1 to destroy",
			},
		}
		assert.Equal(t, "5 to add, 2 to change, 1 to destroy", buildStatusDescription("plan", result))
	})

	t.Run("with changes but no terraform data", func(t *testing.T) {
		result := &plugin.OutputResult{HasChanges: true}
		assert.Equal(t, "Changes detected", buildStatusDescription("plan", result))
	})

	t.Run("no changes", func(t *testing.T) {
		result := &plugin.OutputResult{}
		assert.Equal(t, "No changes", buildStatusDescription("plan", result))
	})

	t.Run("nil result", func(t *testing.T) {
		assert.Equal(t, "No changes", buildStatusDescription("plan", nil))
	})

	t.Run("with errors", func(t *testing.T) {
		result := &plugin.OutputResult{HasErrors: true}
		assert.Equal(t, "Failed", buildStatusDescription("plan", result))
	})
}

func TestGetContextPrefix(t *testing.T) {
	t.Run("default when nil config", func(t *testing.T) {
		assert.Equal(t, "atmos", getContextPrefix(nil))
	})

	t.Run("default when empty prefix", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		assert.Equal(t, "atmos", getContextPrefix(cfg))
	})

	t.Run("custom prefix", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{
			CI: schema.CIConfig{Checks: schema.CIChecksConfig{ContextPrefix: "myorg"}},
		}
		assert.Equal(t, "myorg", getContextPrefix(cfg))
	})
}

func TestIsStatusEnabled(t *testing.T) {
	t.Run("nil defaults to true", func(t *testing.T) {
		assert.True(t, isStatusEnabled(nil))
	})

	t.Run("explicitly true", func(t *testing.T) {
		assert.True(t, isStatusEnabled(boolPtr(true)))
	})

	t.Run("explicitly false", func(t *testing.T) {
		assert.False(t, isStatusEnabled(boolPtr(false)))
	})
}

func TestFormatResourceCount(t *testing.T) {
	assert.Equal(t, "1 resource", formatResourceCount(1))
	assert.Equal(t, "3 resources", formatResourceCount(3))
	assert.Equal(t, "0 resources", formatResourceCount(0))
}

func TestParseOutputWithError(t *testing.T) {
	p := &Plugin{}

	t.Run("plan exit 0 with text changes - text-detected changes preserved", func(t *testing.T) {
		// Plan WITHOUT -detailed-exitcode: terraform always returns 0 even for
		// changes; text parsing is the only HasChanges signal. Exit code is 0
		// so HasErrors must remain false.
		ctx := &plugin.HookContext{
			Output:   "Plan: 1 to add, 0 to change, 0 to destroy.",
			Command:  "plan",
			ExitCode: 0,
		}
		result := p.parseOutputWithError(ctx)
		assert.True(t, result.HasChanges)
		assert.False(t, result.HasErrors)
	})

	t.Run("plan exit 1 with empty output - HasErrors from exit code only", func(t *testing.T) {
		// Auth failure: terraform never ran, no parseable "Error:" line, but
		// exit code 1 must still flip HasErrors=true.
		ctx := &plugin.HookContext{
			Output:   "",
			Command:  "plan",
			ExitCode: 1,
		}
		result := p.parseOutputWithError(ctx)
		assert.True(t, result.HasErrors, "exit code 1 must mark plan as errored")
		assert.Equal(t, 1, result.ExitCode)
	})

	t.Run("plan exit 2 with empty output - HasChanges from exit code only", func(t *testing.T) {
		// Plan with -detailed-exitcode and changes: exit 2 is authoritative
		// even when text parsing finds no "Plan: X to add" markers.
		ctx := &plugin.HookContext{
			Output:   "",
			Command:  "plan",
			ExitCode: 2,
		}
		result := p.parseOutputWithError(ctx)
		assert.True(t, result.HasChanges, "exit code 2 must mark plan as having changes")
		assert.False(t, result.HasErrors, "exit code 2 is success, not error")
	})

	t.Run("apply exit 1 with empty output - HasErrors from exit code only", func(t *testing.T) {
		// Auth failure on `terraform deploy/apply`: same fix as the plan
		// equivalent above.
		ctx := &plugin.HookContext{
			Output:   "",
			Command:  "apply",
			ExitCode: 1,
		}
		result := p.parseOutputWithError(ctx)
		assert.True(t, result.HasErrors, "non-zero exit must mark apply as errored")
		assert.Equal(t, 1, result.ExitCode)
	})

	t.Run("apply exit 0 with stray Error in output - exit code wins", func(t *testing.T) {
		// Exit code is authoritative: if terraform exited 0, the apply
		// succeeded, even if the captured output contains a stray "Error:"
		// line (e.g., from a pre-condition warning that did not block).
		ctx := &plugin.HookContext{
			Output:   "Error: stray noise that should not flip status\n\nApply complete! Resources: 1 added, 0 changed, 0 destroyed.",
			Command:  "apply",
			ExitCode: 0,
		}
		result := p.parseOutputWithError(ctx)
		assert.False(t, result.HasErrors, "apply with exit 0 must be success regardless of stray Error: text")
	})

	t.Run("with command error - defensive fallback when exit code unset", func(t *testing.T) {
		// Defensive: if a caller forgets to set ExitCode but does pass
		// CommandError, the result still reports failure with a default
		// exit code of 1 and the error message.
		ctx := &plugin.HookContext{
			Output:       "",
			Command:      "plan",
			CommandError: fmt.Errorf("terraform plan failed"),
		}
		result := p.parseOutputWithError(ctx)
		assert.True(t, result.HasErrors)
		assert.Equal(t, 1, result.ExitCode)
		assert.Equal(t, []string{"terraform plan failed"}, result.Errors)
	})

	t.Run("exit 1 surfaces command error message", func(t *testing.T) {
		// Production path: cmd/terraform/utils.go sets both CommandError and
		// ExitCode. Confirm the error message is surfaced for the template.
		ctx := &plugin.HookContext{
			Output:       "",
			Command:      "apply",
			CommandError: fmt.Errorf("authentication failed"),
			ExitCode:     1,
		}
		result := p.parseOutputWithError(ctx)
		assert.True(t, result.HasErrors)
		assert.Equal(t, 1, result.ExitCode)
		assert.Equal(t, []string{"authentication failed"}, result.Errors)
	})
}

func TestOnBeforePlan_CheckDisabled(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config:   &schema.AtmosConfiguration{}, // Checks disabled by default.
		Provider: mp,
		Command:  "plan",

		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	err := p.onBeforePlan(ctx)
	require.NoError(t, err)

	// No check run should be created.
	assert.Empty(t, mp.checkRunCalls)
}

func TestOnBeforePlan_CheckEnabled(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{Checks: schema.CIChecksConfig{Enabled: boolPtr(true)}},
		},
		Provider: mp,
		Command:  "plan",
		CICtx: &provider.Context{
			RepoOwner: "owner",
			RepoName:  "repo",
			SHA:       "abc123",
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	err := p.onBeforePlan(ctx)
	require.NoError(t, err)

	// Check run should be created.
	require.Len(t, mp.checkRunCalls, 1)
	assert.Equal(t, provider.CheckRunStateInProgress, mp.checkRunCalls[0].Status)
	assert.Equal(t, "atmos/plan/dev/vpc", mp.checkRunCalls[0].Name)
}

func TestOnAfterApply_WritesOutputs(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
				Output:  schema.CIOutputConfig{Enabled: boolPtr(true)},
			},
		},
		Provider: mp,
		Command:  "apply",

		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
		Output: "Apply complete! Resources: 1 added, 0 changed, 0 destroyed.",
	}

	err := p.onAfterApply(ctx)
	require.NoError(t, err)

	// Outputs should be written.
	assert.Equal(t, "dev", mp.writer.outputs["stack"])
	assert.Equal(t, "vpc", mp.writer.outputs["component"])
	assert.Equal(t, "apply", mp.writer.outputs["command"])
	assert.Equal(t, "true", mp.writer.outputs["has_changes"])
}

func TestOnAfterApply_BothSummaryAndOutputDisabled(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
				Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
			},
		},
		Provider: mp,
		Command:  "apply",

		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
		Output: "Apply complete!",
	}

	err := p.onAfterApply(ctx)
	require.NoError(t, err)

	// Nothing should be written.
	assert.Empty(t, mp.writer.summaries)
	assert.Empty(t, mp.writer.outputs)
}

func TestOnAfterPlan_AllDisabled_NoPlanfile(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
				Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
				Checks:  schema.CIChecksConfig{Enabled: boolPtr(false)},
			},
		},
		Provider: mp,
		Command:  "plan",

		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
			PlanFile:         "", // No planfile.
		},
		Output: "No changes.",
		CreatePlanfileStore: func() (any, error) {
			return nil, fmt.Errorf("should not be called")
		},
	}

	err := p.onAfterPlan(ctx)
	require.NoError(t, err)

	// Nothing should be written or called.
	assert.Empty(t, mp.writer.summaries)
	assert.Empty(t, mp.writer.outputs)
	assert.Empty(t, mp.checkRunCalls)
	assert.Empty(t, mp.updateRunCalls)
}

func TestOnAfterPlan_OutputEnabled_WritesVariables(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
				Output:  schema.CIOutputConfig{Enabled: boolPtr(true)},
				Checks:  schema.CIChecksConfig{Enabled: boolPtr(false)},
			},
		},
		Provider: mp,
		Command:  "plan",

		Info: &schema.ConfigAndStacksInfo{
			Stack:            "prod",
			ComponentFromArg: "rds",
			PlanFile:         "", // No planfile path.
		},
		Output: "Plan: 3 to add, 1 to change, 0 to destroy.",
	}

	err := p.onAfterPlan(ctx)
	require.NoError(t, err)

	// Outputs should be written with parsed result data.
	assert.Equal(t, "prod", mp.writer.outputs["stack"])
	assert.Equal(t, "rds", mp.writer.outputs["component"])
	assert.Equal(t, "plan", mp.writer.outputs["command"])
	assert.Equal(t, "true", mp.writer.outputs["has_changes"])
	assert.Equal(t, "3", mp.writer.outputs["resources_to_create"])
	assert.Equal(t, "1", mp.writer.outputs["resources_to_change"])
	assert.Equal(t, "0", mp.writer.outputs["resources_to_destroy"])
}

func TestOnAfterPlan_CheckEnabled_UpdatesCheckRun(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
				Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
				Checks:  schema.CIChecksConfig{Enabled: boolPtr(true)},
			},
		},
		Provider: mp,
		Command:  "plan",
		CICtx: &provider.Context{
			RepoOwner: "owner",
			RepoName:  "repo",
			SHA:       "abc123",
		},
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
			PlanFile:         "", // No planfile.
		},
		Output: "Plan: 2 to add, 0 to change, 0 to destroy.",
	}

	err := p.onAfterPlan(ctx)
	require.NoError(t, err)

	// Check run should have been updated.
	require.Len(t, mp.updateRunCalls, 1)
	assert.Equal(t, provider.CheckRunStateSuccess, mp.updateRunCalls[0].Status)
	assert.Equal(t, "owner", mp.updateRunCalls[0].Owner)
	assert.Equal(t, "repo", mp.updateRunCalls[0].Repo)
	assert.Equal(t, "abc123", mp.updateRunCalls[0].SHA)
	assert.Equal(t, "atmos/plan/dev/vpc", mp.updateRunCalls[0].Name)
}

func TestOnAfterPlan_WithCommandError_FailureCheckRun(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
				Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
				Checks:  schema.CIChecksConfig{Enabled: boolPtr(true)},
			},
		},
		Provider:     mp,
		Command:      "plan",
		CommandError: fmt.Errorf("terraform plan failed"),
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
			PlanFile:         "",
		},
		Output: "",
	}

	err := p.onAfterPlan(ctx)
	require.NoError(t, err)

	// Check run should be updated with failure.
	require.Len(t, mp.updateRunCalls, 1)
	assert.Equal(t, provider.CheckRunStateFailure, mp.updateRunCalls[0].Status)
}

func TestOnAfterPlan_PerOperationStatuses(t *testing.T) {
	t.Run("creates add/change/destroy statuses when counts > 0", func(t *testing.T) {
		p := &Plugin{}
		mp := newMockProvider()

		ctx := &plugin.HookContext{
			Config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
					Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
					Checks:  schema.CIChecksConfig{Enabled: boolPtr(true)},
				},
			},
			Provider: mp,
			Command:  "plan",
			CICtx: &provider.Context{
				RepoOwner: "owner",
				RepoName:  "repo",
				SHA:       "abc123",
			},
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "vpc",
			},
			Output: "Plan: 3 to add, 1 to change, 2 to destroy.",
		}

		err := p.onAfterPlan(ctx)
		require.NoError(t, err)

		// Component-level status + 3 per-operation statuses.
		// updateRunCalls has the component-level update.
		require.Len(t, mp.updateRunCalls, 1)
		assert.Equal(t, "atmos/plan/dev/vpc", mp.updateRunCalls[0].Name)

		// Per-operation statuses are created via CreateCheckRun.
		// 1 from onBeforePlan is not called here, so these are only per-op statuses.
		require.Len(t, mp.checkRunCalls, 3)

		// Verify per-operation status names and descriptions.
		names := make(map[string]string)
		for _, call := range mp.checkRunCalls {
			names[call.Name] = call.Title
		}
		assert.Equal(t, "3 resources", names["atmos/plan/dev/vpc/add"])
		assert.Equal(t, "1 resource", names["atmos/plan/dev/vpc/change"])
		assert.Equal(t, "2 resources", names["atmos/plan/dev/vpc/destroy"])

		// All per-operation statuses should be success.
		for _, call := range mp.checkRunCalls {
			assert.Equal(t, provider.CheckRunStateSuccess, call.Status)
		}
	})

	t.Run("skips statuses when counts are 0", func(t *testing.T) {
		p := &Plugin{}
		mp := newMockProvider()

		ctx := &plugin.HookContext{
			Config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
					Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
					Checks:  schema.CIChecksConfig{Enabled: boolPtr(true)},
				},
			},
			Provider: mp,
			Command:  "plan",
			CICtx: &provider.Context{
				RepoOwner: "owner",
				RepoName:  "repo",
				SHA:       "abc123",
			},
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "vpc",
			},
			Output: "No changes. Your infrastructure matches the configuration.",
		}

		err := p.onAfterPlan(ctx)
		require.NoError(t, err)

		// Component-level update only.
		require.Len(t, mp.updateRunCalls, 1)
		// No per-operation statuses.
		assert.Empty(t, mp.checkRunCalls)
	})

	t.Run("respects statuses config to disable specific operations", func(t *testing.T) {
		p := &Plugin{}
		mp := newMockProvider()

		ctx := &plugin.HookContext{
			Config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
					Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
					Checks: schema.CIChecksConfig{
						Enabled: boolPtr(true),
						Statuses: schema.CIChecksStatusesConfig{
							Add:     boolPtr(false), // Disable add status.
							Destroy: boolPtr(false), // Disable destroy status.
						},
					},
				},
			},
			Provider: mp,
			Command:  "plan",
			CICtx: &provider.Context{
				RepoOwner: "owner",
				RepoName:  "repo",
				SHA:       "abc123",
			},
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "vpc",
			},
			Output: "Plan: 3 to add, 1 to change, 2 to destroy.",
		}

		err := p.onAfterPlan(ctx)
		require.NoError(t, err)

		// Only change status should be created (add and destroy disabled).
		require.Len(t, mp.checkRunCalls, 1)
		assert.Equal(t, "atmos/plan/dev/vpc/change", mp.checkRunCalls[0].Name)
	})

	t.Run("custom context prefix", func(t *testing.T) {
		p := &Plugin{}
		mp := newMockProvider()

		ctx := &plugin.HookContext{
			Config: &schema.AtmosConfiguration{
				CI: schema.CIConfig{
					Summary: schema.CISummaryConfig{Enabled: boolPtr(false)},
					Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
					Checks: schema.CIChecksConfig{
						Enabled:       boolPtr(true),
						ContextPrefix: "myorg",
					},
				},
			},
			Provider: mp,
			Command:  "plan",
			CICtx: &provider.Context{
				RepoOwner: "owner",
				RepoName:  "repo",
				SHA:       "abc123",
			},
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "vpc",
			},
			Output: "Plan: 1 to add, 0 to change, 0 to destroy.",
		}

		err := p.onAfterPlan(ctx)
		require.NoError(t, err)

		// Component-level status uses custom prefix.
		require.Len(t, mp.updateRunCalls, 1)
		assert.Equal(t, "myorg/plan/dev/vpc", mp.updateRunCalls[0].Name)

		// Per-operation status uses custom prefix.
		require.Len(t, mp.checkRunCalls, 1)
		assert.Equal(t, "myorg/plan/dev/vpc/add", mp.checkRunCalls[0].Name)
	})
}

func TestOnBeforeApply_NoPlanfilePath(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config:   &schema.AtmosConfiguration{},
		Provider: mp,
		Command:  "apply",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "",
			ComponentFromArg: "",
			PlanFile:         "",
		},
	}

	// Should return nil (no planfile to download).
	err := p.onBeforeApply(ctx)
	require.NoError(t, err)
}

func TestWriteOutputs_NilWriter(t *testing.T) {
	p := &Plugin{}

	// mockProvider with nil writer.
	nilWriterProvider := &nilWriterMockProvider{}

	ctx := &plugin.HookContext{
		Config:   &schema.AtmosConfiguration{},
		Provider: nilWriterProvider,
		Command:  "plan",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	result := &plugin.OutputResult{}
	err := p.writeOutputs(ctx, result, "")
	require.NoError(t, err)
}

// nilWriterMockProvider is a mock provider that returns nil for OutputWriter.
type nilWriterMockProvider struct {
	mockProvider
}

func (n *nilWriterMockProvider) OutputWriter() provider.OutputWriter {
	return nil
}

func TestWriteOutputs_WithFilteredVariables(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Output: schema.CIOutputConfig{
					Enabled:   boolPtr(true),
					Variables: []string{"stack", "has_changes"},
				},
			},
		},
		Provider: mp,
		Command:  "plan",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	result := &plugin.OutputResult{HasChanges: true}
	err := p.writeOutputs(ctx, result, "")
	require.NoError(t, err)

	// Only filtered variables should be present.
	assert.Equal(t, "dev", mp.writer.outputs["stack"])
	assert.Equal(t, "true", mp.writer.outputs["has_changes"])
	// These should NOT be in outputs since they're not in the allowed list.
	_, hasComponent := mp.writer.outputs["component"]
	assert.False(t, hasComponent)
	_, hasCommand := mp.writer.outputs["command"]
	assert.False(t, hasCommand)
}

func TestWriteOutputs_WithRenderedSummary(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config:   &schema.AtmosConfiguration{},
		Provider: mp,
		Command:  "plan",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	result := &plugin.OutputResult{}
	err := p.writeOutputs(ctx, result, "## Plan Summary\nNo changes")
	require.NoError(t, err)

	// Summary should be in outputs.
	assert.Equal(t, "## Plan Summary\nNo changes", mp.writer.outputs["summary"])
}

func TestUploadPlanfile_NoPlanfilePath(t *testing.T) {
	p := &Plugin{}

	ctx := &plugin.HookContext{
		Config:  &schema.AtmosConfiguration{},
		Command: "plan",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "",
			ComponentFromArg: "",
			PlanFile:         "",
		},
	}

	// Should return nil (no planfile to upload).
	err := p.uploadPlanfile(ctx)
	require.NoError(t, err)
}

func TestUploadPlanfile_PlanfileDoesNotExist(t *testing.T) {
	p := &Plugin{}

	ctx := &plugin.HookContext{
		Config:  &schema.AtmosConfiguration{},
		Command: "plan",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
			PlanFile:         "/nonexistent/path/to/planfile.tfplan",
		},
	}

	// Should return nil (file doesn't exist, skip).
	err := p.uploadPlanfile(ctx)
	require.NoError(t, err)
}

func TestDownloadPlanfile_NoPlanfilePath(t *testing.T) {
	p := &Plugin{}

	ctx := &plugin.HookContext{
		Config:  &schema.AtmosConfiguration{},
		Command: "apply",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "",
			ComponentFromArg: "",
			PlanFile:         "",
		},
	}

	err := p.downloadPlanfile(ctx)
	require.NoError(t, err)
}

func TestBuildPlanfileMetadata(t *testing.T) {
	p := &Plugin{}

	t.Run("with CI context and plan output", func(t *testing.T) {
		ctx := &plugin.HookContext{
			Config:  &schema.AtmosConfiguration{},
			Command: "plan",
			Output:  "Plan: 3 to add, 1 to change, 0 to destroy.",
			CICtx: &provider.Context{
				SHA:        "abc123",
				Branch:     "main",
				RunID:      "run-42",
				Repository: "owner/repo",
				PullRequest: &provider.PRInfo{
					Number: 123,
				},
			},
			Info: &schema.ConfigAndStacksInfo{
				Stack:                 "dev",
				ComponentFromArg:      "vpc",
				ComponentFolderPrefix: "components/terraform/vpc",
			},
		}

		metadata := p.buildPlanfileMetadata(ctx)
		assert.Equal(t, "dev", metadata.Stack)
		assert.Equal(t, "vpc", metadata.Component)
		assert.Equal(t, "abc123", metadata.SHA)
		assert.Equal(t, "main", metadata.Branch)
		assert.Equal(t, "run-42", metadata.RunID)
		assert.Equal(t, "owner/repo", metadata.Repository)
		assert.Equal(t, 123, metadata.PRNumber)
		assert.True(t, metadata.HasChanges)
		assert.Equal(t, 3, metadata.Additions)
		assert.Equal(t, 1, metadata.Changes)
		assert.Equal(t, 0, metadata.Destructions)
	})

	t.Run("without CI context", func(t *testing.T) {
		ctx := &plugin.HookContext{
			Config:  &schema.AtmosConfiguration{},
			Command: "plan",
			Output:  "No changes.",
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "vpc",
			},
		}

		metadata := p.buildPlanfileMetadata(ctx)
		assert.Equal(t, "dev", metadata.Stack)
		assert.Equal(t, "vpc", metadata.Component)
		assert.Empty(t, metadata.SHA)
		assert.False(t, metadata.HasChanges)
	})

	t.Run("with CI context but no pull request", func(t *testing.T) {
		ctx := &plugin.HookContext{
			Config:  &schema.AtmosConfiguration{},
			Command: "plan",
			Output:  "No changes.",
			CICtx: &provider.Context{
				SHA: "def456",
			},
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "vpc",
			},
		}

		metadata := p.buildPlanfileMetadata(ctx)
		assert.Equal(t, "def456", metadata.SHA)
		assert.Equal(t, 0, metadata.PRNumber)
	})
}

func TestLogArtifactOperation(t *testing.T) {
	// logArtifactOperation just logs; verify it doesn't panic.
	info := &schema.ConfigAndStacksInfo{
		Stack:            "dev",
		ComponentFromArg: "vpc",
	}

	// With path.
	logArtifactOperation("Uploaded", "dev/vpc.tfplan", "local", "/tmp/plan.tfplan", info)

	// Without path.
	logArtifactOperation("Downloaded", "dev/vpc.tfplan", "s3", "", info)
}

func TestGetOutputVariables_ApplyIncludesSuccess(t *testing.T) {
	p := &Plugin{}

	t.Run("apply success", func(t *testing.T) {
		result := &plugin.OutputResult{HasErrors: false}
		vars := p.getOutputVariables(result, "apply")
		assert.Equal(t, "true", vars["success"])
	})

	t.Run("apply failure", func(t *testing.T) {
		result := &plugin.OutputResult{HasErrors: true}
		vars := p.getOutputVariables(result, "apply")
		assert.Equal(t, "false", vars["success"])
	})

	t.Run("plan does not include success", func(t *testing.T) {
		result := &plugin.OutputResult{HasErrors: false}
		vars := p.getOutputVariables(result, "plan")
		_, hasSuccess := vars["success"]
		assert.False(t, hasSuccess)
	})
}

// TestGetTerraformOutputs was removed — terraform outputs are now parsed from
// apply stdout (via ParseApplyOutput) instead of running `terraform output`
// separately, which required backend credentials not available in PostRunE.

func TestWriteOutputs_ApplyWithTerraformOutputs(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Output: schema.CIOutputConfig{Enabled: boolPtr(true)},
			},
		},
		Provider: mp,
		Command:  "apply",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	// Outputs are now parsed from apply stdout via ParseApplyOutput.
	result := &plugin.OutputResult{
		HasChanges: true,
		Data: &plugin.TerraformOutputData{
			Outputs: map[string]plugin.TerraformOutput{
				"vpc_id":          {Value: "vpc-abc123"},
				"environment_url": {Value: "https://example.com"},
			},
		},
	}
	err := p.writeOutputs(ctx, result, "")
	require.NoError(t, err)

	// Terraform outputs should be written with "output_" prefix.
	assert.Equal(t, "vpc-abc123", mp.writer.outputs["output_vpc_id"])
	assert.Equal(t, "https://example.com", mp.writer.outputs["output_environment_url"])

	// Standard variables should also be present.
	assert.Equal(t, "dev", mp.writer.outputs["stack"])
	assert.Equal(t, "vpc", mp.writer.outputs["component"])
	assert.Equal(t, "apply", mp.writer.outputs["command"])
	assert.Equal(t, "true", mp.writer.outputs["success"])
}

func TestWriteOutputs_ApplyTerraformOutputsBypassFilter(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Output: schema.CIOutputConfig{
					Enabled:   boolPtr(true),
					Variables: []string{"stack"},
				},
			},
		},
		Provider: mp,
		Command:  "apply",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	// Outputs are parsed from apply stdout.
	result := &plugin.OutputResult{
		Data: &plugin.TerraformOutputData{
			Outputs: map[string]plugin.TerraformOutput{
				"vpc_id":    {Value: "vpc-abc123"},
				"subnet_id": {Value: "subnet-456"},
			},
		},
	}
	err := p.writeOutputs(ctx, result, "")
	require.NoError(t, err)

	// Native CI variable filtered by whitelist.
	assert.Equal(t, "dev", mp.writer.outputs["stack"])
	// Native CI variable NOT in whitelist — filtered out.
	_, hasComponent := mp.writer.outputs["component"]
	assert.False(t, hasComponent)

	// Terraform outputs bypass the whitelist — all are included.
	assert.Equal(t, "vpc-abc123", mp.writer.outputs["output_vpc_id"])
	assert.Equal(t, "subnet-456", mp.writer.outputs["output_subnet_id"])
}

func TestWriteOutputs_PlanDoesNotIncludeTerraformOutputs(t *testing.T) {
	p := &Plugin{}
	mp := newMockProvider()

	ctx := &plugin.HookContext{
		Config:   &schema.AtmosConfiguration{},
		Provider: mp,
		Command:  "plan",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}

	// Even if the result has outputs, plan command should not export them.
	result := &plugin.OutputResult{
		Data: &plugin.TerraformOutputData{
			Outputs: map[string]plugin.TerraformOutput{
				"vpc_id": {Value: "vpc-abc123"},
			},
		},
	}
	err := p.writeOutputs(ctx, result, "")
	require.NoError(t, err)

	// No output_ variables should be present for plan commands.
	for key := range mp.writer.outputs {
		assert.NotContains(t, key, "output_")
	}
}

func TestResolveArtifactPath_EmptyStackOrComponent(t *testing.T) {
	p := &Plugin{}

	t.Run("empty stack", func(t *testing.T) {
		ctx := &plugin.HookContext{
			Config:  &schema.AtmosConfiguration{},
			Command: "plan",
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "",
				ComponentFromArg: "vpc",
			},
		}
		path := p.resolveArtifactPath(ctx)
		assert.Empty(t, path)
	})

	t.Run("empty component", func(t *testing.T) {
		ctx := &plugin.HookContext{
			Config:  &schema.AtmosConfiguration{},
			Command: "plan",
			Info: &schema.ConfigAndStacksInfo{
				Stack:            "dev",
				ComponentFromArg: "",
			},
		}
		path := p.resolveArtifactPath(ctx)
		assert.Empty(t, path)
	})
}

func TestIsPlanfileStorageEnabled(t *testing.T) {
	t.Run("nil config", func(t *testing.T) {
		assert.False(t, isPlanfileStorageEnabled(nil))
	})

	t.Run("empty planfiles config", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		assert.False(t, isPlanfileStorageEnabled(cfg))
	})

	t.Run("planfiles with default set", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Components.Terraform.Planfiles.Default = "s3"
		assert.True(t, isPlanfileStorageEnabled(cfg))
	})

	t.Run("planfiles with priority list", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Components.Terraform.Planfiles.Priority = []string{"github", "s3"}
		assert.True(t, isPlanfileStorageEnabled(cfg))
	})

	t.Run("planfiles with stores defined", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Components.Terraform.Planfiles.Stores = map[string]schema.PlanfileStoreSpec{
			"s3": {Type: "s3"},
		}
		assert.True(t, isPlanfileStorageEnabled(cfg))
	})

	t.Run("planfiles with empty priority list", func(t *testing.T) {
		cfg := &schema.AtmosConfiguration{}
		cfg.Components.Terraform.Planfiles.Priority = []string{}
		assert.False(t, isPlanfileStorageEnabled(cfg))
	})
}

// newFailureSummaryHookContext builds a HookContext suited for verifying that
// summary rendering reflects an exit-code failure. Output is intentionally
// empty because the bug under test is auth (or other pre-terraform) failures
// where no terraform output exists yet. Both ExitCode and CommandError are set
// to mirror production: cmd/terraform/utils.go computes ExitCode from CmdErr
// via errUtils.GetExitCode and passes both into RunCIHooks.
func newFailureSummaryHookContext(command string, cmdErr error) *plugin.HookContext {
	return &plugin.HookContext{
		Config: &schema.AtmosConfiguration{
			CI: schema.CIConfig{
				Summary: schema.CISummaryConfig{Enabled: boolPtr(true)},
				Output:  schema.CIOutputConfig{Enabled: boolPtr(false)},
				Checks:  schema.CIChecksConfig{Enabled: boolPtr(false)},
			},
		},
		Provider:       newMockProvider(),
		TemplateLoader: templates.NewLoader(&schema.AtmosConfiguration{}),
		Command:        command,
		CommandError:   cmdErr,
		ExitCode:       1,
		Output:         "",
		Info: &schema.ConfigAndStacksInfo{
			Stack:            "dev",
			ComponentFromArg: "vpc",
		},
	}
}

func TestOnAfterPlan_WithCommandError_RendersFailureSummary(t *testing.T) {
	p := &Plugin{}
	ctx := newFailureSummaryHookContext("plan", fmt.Errorf("identity failed: assume role denied"))
	mp := ctx.Provider.(*mockProvider)

	err := p.onAfterPlan(ctx)
	require.NoError(t, err)

	require.Len(t, mp.writer.summaries, 1, "summary should have been rendered")
	rendered := mp.writer.summaries[0]

	assert.Contains(t, rendered, "Plan Failed for", "should use the failure header")
	assert.Contains(t, rendered, "PLAN-FAILED-ff0000", "should use the FAILED badge")
	assert.Contains(t, rendered, "identity failed: assume role denied", "should surface the command error")
	assert.NotContains(t, rendered, "No Changes for", "must not fall through to no-changes branch")
	assert.NotContains(t, rendered, "NO_CHANGE-inactive", "must not use the no-change badge")
}

func TestOnAfterApply_WithCommandError_RendersFailureSummary(t *testing.T) {
	p := &Plugin{}
	ctx := newFailureSummaryHookContext("apply", fmt.Errorf("identity failed: assume role denied"))
	mp := ctx.Provider.(*mockProvider)

	err := p.onAfterApply(ctx)
	require.NoError(t, err)

	require.Len(t, mp.writer.summaries, 1, "summary should have been rendered")
	rendered := mp.writer.summaries[0]

	assert.Contains(t, rendered, "Apply Failed for", "should use the failure header")
	assert.Contains(t, rendered, "APPLY-FAILED-ff0000", "should use the FAILED badge")
	assert.Contains(t, rendered, "identity failed: assume role denied", "should surface the command error")
	assert.NotContains(t, rendered, "No Changes Applied for", "must not fall through to no-changes branch")
	assert.NotContains(t, rendered, "NO_CHANGE-inactive", "must not use the no-change badge")
}

// TestOnAfterApply_WithExitCodeOnly_RendersFailureSummary verifies that the
// summary reflects failure even when only the exit code is set (no
// CommandError). This documents the contract: exit code is authoritative;
// callers do not need to also pass an error to trigger failure rendering.
func TestOnAfterApply_WithExitCodeOnly_RendersFailureSummary(t *testing.T) {
	p := &Plugin{}
	ctx := newFailureSummaryHookContext("apply", nil)
	ctx.ExitCode = 1
	mp := ctx.Provider.(*mockProvider)

	err := p.onAfterApply(ctx)
	require.NoError(t, err)

	require.Len(t, mp.writer.summaries, 1)
	rendered := mp.writer.summaries[0]
	assert.Contains(t, rendered, "Apply Failed for")
	assert.Contains(t, rendered, "APPLY-FAILED-ff0000")
	assert.NotContains(t, rendered, "No Changes Applied for")
}

// TestOnAfterDeploy_WithCommandError_RendersFailureSummary covers the original
// reported bug: `atmos terraform deploy ... --upload-status` failing at auth
// before terraform runs must produce an "Apply Failed" summary, not "No Changes
// Applied". Deploy uses the apply template internally (handlers.go onAfterDeploy
// overrides ctx.Command to "apply" for templating).
func TestOnAfterDeploy_WithCommandError_RendersFailureSummary(t *testing.T) {
	p := &Plugin{}
	ctx := newFailureSummaryHookContext("deploy", fmt.Errorf("identity failed: assume role denied"))
	mp := ctx.Provider.(*mockProvider)

	err := p.onAfterDeploy(ctx)
	require.NoError(t, err)

	require.Len(t, mp.writer.summaries, 1, "summary should have been rendered")
	rendered := mp.writer.summaries[0]

	assert.Contains(t, rendered, "Apply Failed for", "deploy must render via the apply template's failure branch")
	assert.Contains(t, rendered, "APPLY-FAILED-ff0000", "should use the FAILED badge")
	assert.Contains(t, rendered, "identity failed: assume role denied", "should surface the command error")
	assert.NotContains(t, rendered, "No Changes Applied for", "must not fall through to no-changes branch")
	assert.NotContains(t, rendered, "NO_CHANGE-inactive", "must not use the no-change badge")
}
