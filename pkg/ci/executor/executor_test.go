package executor

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/ci/internal/plugin"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/schema"
)

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

func TestDetectStoreFromEnv(t *testing.T) {
	t.Run("no env vars returns nil", func(t *testing.T) {
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
		assert.Equal(t, "aws/s3", result.Type)
		assert.Equal(t, "my-bucket", result.Options["bucket"])
		assert.Equal(t, "plans/", result.Options["prefix"])
		assert.Equal(t, "us-east-1", result.Options["region"])
	})

	t.Run("GitHub Actions detected", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "true")

		result := detectStoreFromEnv()
		require.NotNil(t, result)
		assert.Equal(t, "github/artifacts", result.Type)
	})

	t.Run("S3 takes precedence over GitHub", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "bucket")
		t.Setenv("GITHUB_ACTIONS", "true")

		result := detectStoreFromEnv()
		require.NotNil(t, result)
		assert.Equal(t, "aws/s3", result.Type)
	})
}

func TestCreatePlanfileStore(t *testing.T) {
	t.Run("defaults to local store", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		store, err := createPlanfileStore(&ci.ExecuteOptions{
			AtmosConfig: &schema.AtmosConfiguration{},
		})
		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, "local/dir", store.Name())
	})

	t.Run("nil config defaults to local store", func(t *testing.T) {
		t.Setenv("ATMOS_PLANFILE_BUCKET", "")
		t.Setenv("GITHUB_ACTIONS", "")

		store, err := createPlanfileStore(&ci.ExecuteOptions{
			AtmosConfig: nil,
		})
		require.NoError(t, err)
		assert.NotNil(t, store)
		assert.Equal(t, "local/dir", store.Name())
	})
}

// mockProvider is a test provider for buildHookContext tests.
type mockProvider struct {
	name     string
	detected bool
}

func (m *mockProvider) Name() string                        { return m.name }
func (m *mockProvider) Detect() bool                        { return m.detected }
func (m *mockProvider) Context() (*provider.Context, error) { return &provider.Context{}, nil }
func (m *mockProvider) OutputWriter() provider.OutputWriter { return nil }
func (m *mockProvider) GetStatus(_ context.Context, _ provider.StatusOptions) (*provider.Status, error) {
	return nil, nil
}

func (m *mockProvider) CreateCheckRun(_ context.Context, _ *provider.CreateCheckRunOptions) (*provider.CheckRun, error) {
	return nil, nil
}

func (m *mockProvider) UpdateCheckRun(_ context.Context, _ *provider.UpdateCheckRunOptions) (*provider.CheckRun, error) {
	return nil, nil
}

func (m *mockProvider) ResolveBase() (*provider.BaseResolution, error) {
	return nil, nil
}

func TestBuildHookContext(t *testing.T) {
	mp := &mockProvider{name: "test", detected: true}

	opts := &ci.ExecuteOptions{
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
	assert.NotNil(t, ctx.CreatePlanfileStore)
}

func TestGetPluginAndBinding_EmptyEvent(t *testing.T) {
	ci.Reset()
	ci.ClearPlugins()
	defer ci.Reset()
	defer ci.ClearPlugins()

	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		},
	}
	require.NoError(t, ci.RegisterPlugin(sp))

	pl, binding := getPluginAndBinding(&ci.ExecuteOptions{Event: ""})
	assert.Nil(t, pl)
	assert.Nil(t, binding)
}

func TestGetPluginAndBinding_UnregisteredComponentType(t *testing.T) {
	ci.Reset()
	ci.ClearPlugins()
	defer ci.Reset()
	defer ci.ClearPlugins()

	pl, binding := getPluginAndBinding(&ci.ExecuteOptions{Event: "after.helmfile.diff"})
	assert.Nil(t, pl)
	assert.Nil(t, binding)
}

func TestGetPluginAndBinding_EventNotHandled(t *testing.T) {
	ci.Reset()
	ci.ClearPlugins()
	defer ci.Reset()
	defer ci.ClearPlugins()

	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		},
	}
	require.NoError(t, ci.RegisterPlugin(sp))

	pl, binding := getPluginAndBinding(&ci.ExecuteOptions{Event: "after.terraform.init"})
	assert.Nil(t, pl)
	assert.Nil(t, binding)
}

func TestGetPluginAndBinding_ComponentTypeOverride(t *testing.T) {
	ci.Reset()
	ci.ClearPlugins()
	defer ci.Reset()
	defer ci.ClearPlugins()

	sp := &stubPlugin{
		componentType: "terraform",
		bindings: []plugin.HookBinding{
			{Event: "after.terraform.plan", Handler: func(_ *plugin.HookContext) error { return nil }},
		},
	}
	require.NoError(t, ci.RegisterPlugin(sp))

	pl, binding := getPluginAndBinding(&ci.ExecuteOptions{
		Event:         "after.terraform.plan",
		ComponentType: "terraform",
	})
	assert.NotNil(t, pl)
	assert.NotNil(t, binding)
}

// stubPlugin is a simple Plugin for tests that uses Handler callbacks.
type stubPlugin struct {
	componentType string
	bindings      []plugin.HookBinding
}

func (s *stubPlugin) GetType() string                       { return s.componentType }
func (s *stubPlugin) GetHookBindings() []plugin.HookBinding { return s.bindings }

// Ensure test cleanup sets PATH back (for token tests that might run in the same binary).
func TestMain(m *testing.M) {
	os.Exit(m.Run())
}
