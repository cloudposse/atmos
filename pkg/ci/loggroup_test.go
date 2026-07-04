package ci

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

// mockGroupingProvider is a mockProvider that also implements provider.LogGrouper.
type mockGroupingProvider struct {
	*mockProvider
	started []string
	ended   int
}

func (m *mockGroupingProvider) StartLogGroup(name string) error {
	m.started = append(m.started, name)
	return nil
}

func (m *mockGroupingProvider) EndLogGroup() error {
	m.ended++
	return nil
}

// registerGrouping installs a detected, grouping-capable provider for the duration of t.
func registerGrouping(t *testing.T) *mockGroupingProvider {
	t.Helper()
	restore := SwapRegistryForTest()
	t.Cleanup(restore)
	p := &mockGroupingProvider{mockProvider: &mockProvider{name: "mock-grouping", detected: true}}
	Register(p)
	return p
}

// modeConfig returns a CI-enabled config with the given groups mode.
func modeConfig(mode string) *schema.AtmosConfiguration {
	cfg := &schema.AtmosConfiguration{}
	cfg.CI.Enabled = true
	cfg.CI.Groups.Mode = mode
	return cfg
}

func TestGroup_EmitsMarkersWhenStepDimensionActive(t *testing.T) {
	p := registerGrouping(t)

	called := false
	// Default (empty) mode resolves to auto, under which DimensionStep is active.
	err := Group(modeConfig(""), DimensionStep, "terraform init", func() error {
		assert.Equal(t, []string{"terraform init"}, p.started)
		assert.Zero(t, p.ended)
		called = true
		return nil
	})

	require.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []string{"terraform init"}, p.started)
	assert.Equal(t, 1, p.ended)
}

func TestGroup_EndMarkerEmittedOnError(t *testing.T) {
	p := registerGrouping(t)

	sentinel := errors.New("step failed")
	err := Group(modeConfig(GroupModeAuto), DimensionStep, "apply", func() error { return sentinel })

	require.ErrorIs(t, err, sentinel)
	assert.Equal(t, []string{"apply"}, p.started)
	assert.Equal(t, 1, p.ended)
}

func TestGroup_DimensionVsMode(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		dim     Dimension
		grouped bool
	}{
		{name: "auto + step → grouped", mode: GroupModeAuto, dim: DimensionStep, grouped: true},
		{name: "auto + phase → grouped", mode: GroupModeAuto, dim: DimensionPhase, grouped: true},
		{name: "auto + invocation → NOT grouped", mode: GroupModeAuto, dim: DimensionInvocation, grouped: false},
		{name: "invocation + invocation → grouped", mode: GroupModeInvocation, dim: DimensionInvocation, grouped: true},
		{name: "invocation + step → NOT grouped", mode: GroupModeInvocation, dim: DimensionStep, grouped: false},
		{name: "invocation + phase → NOT grouped", mode: GroupModeInvocation, dim: DimensionPhase, grouped: false},
		{name: "off + step → NOT grouped", mode: GroupModeOff, dim: DimensionStep, grouped: false},
		{name: "off + invocation → NOT grouped", mode: GroupModeOff, dim: DimensionInvocation, grouped: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := registerGrouping(t)

			called := false
			require.NoError(t, Group(modeConfig(tt.mode), tt.dim, "x", func() error { called = true; return nil }))
			assert.True(t, called, "fn must always run")
			if tt.grouped {
				assert.Equal(t, []string{"x"}, p.started)
				assert.Equal(t, 1, p.ended)
			} else {
				assert.Empty(t, p.started)
				assert.Zero(t, p.ended)
			}
		})
	}
}

func TestDimensionActive_UnknownDimension(t *testing.T) {
	assert.False(t, dimensionActive(GroupModeAuto, Dimension(99)))
}

func TestGroup_NoNestedGroups(t *testing.T) {
	p := registerGrouping(t)

	inner := false
	err := Group(modeConfig(GroupModeAuto), DimensionStep, "outer", func() error {
		return Group(modeConfig(GroupModeAuto), DimensionStep, "inner", func() error {
			inner = true
			return nil
		})
	})

	require.NoError(t, err)
	assert.True(t, inner)
	assert.Equal(t, []string{"outer"}, p.started)
	assert.Equal(t, 1, p.ended)
}

func TestGroup_NoOpCases(t *testing.T) {
	tests := []struct {
		name       string
		register   bool
		config     *schema.AtmosConfiguration
		sentinelOn bool
	}{
		{name: "nil config", register: true, config: nil},
		{name: "ci disabled", register: true, config: &schema.AtmosConfiguration{}},
		{name: "mode off", register: true, config: modeConfig(GroupModeOff)},
		{name: "no provider registered", register: false, config: modeConfig(GroupModeAuto)},
		{name: "nesting sentinel already set", register: true, config: modeConfig(GroupModeAuto), sentinelOn: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var p *mockGroupingProvider
			if tt.register {
				p = registerGrouping(t)
			} else {
				restore := SwapRegistryForTest()
				t.Cleanup(restore)
			}
			if tt.sentinelOn {
				t.Setenv(logGroupSentinelEnvVar, "1")
			}

			called := false
			err := Group(tt.config, DimensionStep, "step", func() error { called = true; return nil })

			require.NoError(t, err)
			assert.True(t, called, "fn must still run when grouping is a no-op")
			if p != nil {
				assert.Empty(t, p.started, "no markers when grouping inactive")
				assert.Zero(t, p.ended, "no markers when grouping inactive")
			}
		})
	}
}

func TestGroup_NoOpWhenProviderLacksGrouping(t *testing.T) {
	restore := SwapRegistryForTest()
	t.Cleanup(restore)
	// mockProvider (no LogGrouper) is detected but cannot group.
	Register(&mockProvider{name: "mock-plain", detected: true})

	called := false
	err := Group(modeConfig(GroupModeAuto), DimensionStep, "step", func() error { called = true; return nil })

	require.NoError(t, err)
	assert.True(t, called)
}

func TestGroupingEnabled(t *testing.T) {
	t.Run("enabled in auto mode with grouping provider", func(t *testing.T) {
		registerGrouping(t)
		assert.True(t, GroupingEnabled(modeConfig(GroupModeAuto)))
	})

	t.Run("enabled in invocation mode (any non-off mode)", func(t *testing.T) {
		registerGrouping(t)
		assert.True(t, GroupingEnabled(modeConfig(GroupModeInvocation)))
	})

	t.Run("disabled when mode off", func(t *testing.T) {
		registerGrouping(t)
		assert.False(t, GroupingEnabled(modeConfig(GroupModeOff)))
	})

	t.Run("disabled when ci disabled", func(t *testing.T) {
		registerGrouping(t)
		assert.False(t, GroupingEnabled(&schema.AtmosConfiguration{}))
	})

	t.Run("disabled when no grouping-capable provider", func(t *testing.T) {
		restore := SwapRegistryForTest()
		t.Cleanup(restore)
		assert.False(t, GroupingEnabled(modeConfig(GroupModeAuto)))
	})
}

func TestShouldPropagateLogGroupSentinel(t *testing.T) {
	tests := []struct {
		name string
		mode string
		dim  Dimension
		want bool
	}{
		{name: "auto step boundary will group", mode: GroupModeAuto, dim: DimensionStep, want: true},
		{name: "auto phase boundary will group", mode: GroupModeAuto, dim: DimensionPhase, want: true},
		{name: "auto invocation boundary will not group", mode: GroupModeAuto, dim: DimensionInvocation, want: false},
		{name: "invocation step boundary will not group by itself", mode: GroupModeInvocation, dim: DimensionStep, want: false},
		{name: "invocation boundary will group", mode: GroupModeInvocation, dim: DimensionInvocation, want: true},
		{name: "off boundary will not group", mode: GroupModeOff, dim: DimensionStep, want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			registerGrouping(t)
			assert.Equal(t, tt.want, ShouldPropagateLogGroupSentinel(modeConfig(tt.mode), tt.dim))
		})
	}
}

func TestShouldPropagateLogGroupSentinel_WhenGroupAlreadyOpen(t *testing.T) {
	registerGrouping(t)

	err := Group(modeConfig(GroupModeInvocation), DimensionInvocation, "atmos workflow deploy", func() error {
		assert.True(t, ShouldPropagateLogGroupSentinel(modeConfig(GroupModeInvocation), DimensionStep))
		return nil
	})

	require.NoError(t, err)
}

func TestLogGroupSentinelEnv(t *testing.T) {
	assert.Equal(t, "ATMOS_CI_LOG_GROUP_ACTIVE=1", LogGroupSentinelEnv())
}

func TestResolveGroupMode(t *testing.T) {
	tests := []struct {
		name string
		cfg  *schema.AtmosConfiguration
		want string
	}{
		{name: "nil", cfg: nil, want: GroupModeOff},
		{name: "ci disabled", cfg: &schema.AtmosConfiguration{}, want: GroupModeOff},
		{name: "empty → auto", cfg: modeConfig(""), want: GroupModeAuto},
		{name: "auto", cfg: modeConfig("auto"), want: GroupModeAuto},
		{name: "invocation", cfg: modeConfig("invocation"), want: GroupModeInvocation},
		{name: "off", cfg: modeConfig("off"), want: GroupModeOff},
		{name: "none alias → off", cfg: modeConfig("none"), want: GroupModeOff},
		{name: "uppercase normalized", cfg: modeConfig("INVOCATION"), want: GroupModeInvocation},
		{name: "unknown → auto (safe default)", cfg: modeConfig("bogus"), want: GroupModeAuto},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, resolveGroupMode(tt.cfg))
		})
	}
}
