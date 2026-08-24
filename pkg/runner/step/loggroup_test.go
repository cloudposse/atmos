package step

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ci"
	githubprovider "github.com/cloudposse/atmos/pkg/ci/providers/github"
	"github.com/cloudposse/atmos/pkg/schema"
)

type recordingGitHubProvider struct {
	*githubprovider.Provider
	started []string
	ended   int
}

func (p *recordingGitHubProvider) StartLogGroup(name string) error {
	p.started = append(p.started, name)
	return nil
}

func (p *recordingGitHubProvider) EndLogGroup() error {
	p.ended++
	return nil
}

func TestGroupLabel(t *testing.T) {
	tests := []struct {
		name    string
		stepN   string
		command string
		want    string
	}{
		{name: "name preferred", stepN: "init", command: "terraform init", want: "init"},
		{name: "command fallback when name empty", stepN: "", command: "terraform init", want: "terraform init"},
		{name: "name whitespace falls back to command", stepN: "   ", command: "apply", want: "apply"},
		{name: "both empty", stepN: "", command: "", want: ""},
		{name: "trims name", stepN: "  deploy  ", command: "c", want: "deploy"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, groupLabel(tt.stepN, tt.command))
		})
	}
}

func TestRunGrouped_PassthroughWhenInactive(t *testing.T) {
	// With CI grouping inactive (empty config, ci disabled), RunGrouped must run
	// fn and propagate its result unchanged.
	sentinel := errors.New("boom")
	err := RunGrouped(&schema.AtmosConfiguration{}, "name", "command", func() error { return sentinel })
	require.ErrorIs(t, err, sentinel)

	called := false
	require.NoError(t, RunGrouped(&schema.AtmosConfiguration{}, "name", "command", func() error {
		called = true
		return nil
	}))
	assert.True(t, called)
}

func TestRunGrouped_ActiveGroupingUsesStepLabel(t *testing.T) {
	restore := ci.SwapRegistryForTest()
	t.Cleanup(restore)
	provider := &recordingGitHubProvider{Provider: githubprovider.NewProvider()}
	ci.Register(provider)
	t.Setenv("GITHUB_ACTIONS", "true")
	// This test binary can itself run as a child of a CI-grouped step (this
	// repo's own atmos.yaml sets ci.enabled: true, and CI runs `go test` via
	// an `atmos test acceptance` custom-command step that calls RunGrouped
	// itself), in which case the ci package's nesting sentinel is already
	// present in the process environment. That would make ci.Group see a
	// false "parent already grouping" and silently no-op, so this assertion
	// on emitted markers would never actually exercise the grouped path.
	// Clear it so the test is deterministic regardless of how the test
	// binary itself was launched. The sentinel's key=value form is minted by
	// ci.LogGroupSentinelEnv(); the key alone is unexported, so it's
	// hardcoded here rather than imported.
	t.Setenv("ATMOS_CI_LOG_GROUP_ACTIVE", "")

	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Enabled = true
	cfg.CI.Groups.Mode = ci.GroupModeAuto

	called := false
	require.NoError(t, RunGrouped(cfg, "  deploy  ", "echo deploy", func() error {
		called = true
		return nil
	}))

	assert.True(t, called)
	assert.Equal(t, []string{groupLabel("  deploy  ", "echo deploy")}, provider.started)
	assert.Equal(t, 1, provider.ended)
}

func TestRunGroupedForType_ExecRunsBare(t *testing.T) {
	called := false
	require.NoError(t, RunGroupedForType(&schema.AtmosConfiguration{}, "name", "command", schema.TaskTypeExec, func() error {
		called = true
		return nil
	}))
	assert.True(t, called)
}
