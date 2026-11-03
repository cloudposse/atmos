package ci

import (
	"os"
	"sync"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/viperguard"
)

func TestReportingGates(t *testing.T) {
	boolPtr := func(value bool) *bool { return &value }
	tests := []struct {
		name                          string
		config                        *schema.AtmosConfiguration
		enabled, annotations, results bool
	}{
		{name: "nil config"},
		{name: "defaults", config: &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}, enabled: true, annotations: true},
		{name: "explicit settings", config: &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true, Annotations: schema.CIAnnotationsConfig{Enabled: boolPtr(false)}, Results: schema.CIResultsConfig{Enabled: boolPtr(true)}}}, enabled: true, results: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.enabled, Enabled(tt.config))
			assert.Equal(t, tt.annotations, AnnotationsEnabled(tt.config))
			assert.Equal(t, tt.results, ResultsEnabled(tt.config))
		})
	}
}

func TestModeEnabledFromFlag(t *testing.T) {
	// The external test files in this package register real providers at init; an empty
	// registry keeps IsCI() false on a CI runner so only the flag decides.
	restore := SwapRegistryForTest()
	t.Cleanup(restore)

	cmd := &cobra.Command{}
	cmd.Flags().Bool("ci", false, "")
	assert.False(t, ModeEnabled(cmd))
	assert.NoError(t, cmd.Flags().Set("ci", "true"))
	assert.True(t, ModeEnabled(cmd))
}

func TestModeEnabledFromProvider(t *testing.T) {
	t.Cleanup(SwapRegistryForTest())
	p := &mockProvider{name: "mode", detected: false}
	Register(p)
	cmd := &cobra.Command{}
	cmd.Flags().Bool("ci", false, "")

	assert.False(t, ModeEnabled(cmd))
	assert.False(t, ModeEnabled(nil))
	p.detected = true
	assert.True(t, ModeEnabled(cmd))
	assert.True(t, ModeEnabled(nil))
}

func TestFeatureGates(t *testing.T) {
	boolPtr := func(value bool) *bool { return &value }

	// gate describes one feature gate: its accessor, how to set its flag, and its nil default.
	type gate struct {
		name       string
		fn         func(*schema.AtmosConfiguration) bool
		set        func(*schema.CIConfig, *bool)
		defaultVal bool
	}
	gates := []gate{
		{name: "summary", fn: SummaryEnabled, set: func(c *schema.CIConfig, v *bool) { c.Summary.Enabled = v }, defaultVal: true},
		{name: "output", fn: OutputEnabled, set: func(c *schema.CIConfig, v *bool) { c.Output.Enabled = v }, defaultVal: true},
		{name: "checks", fn: ChecksEnabled, set: func(c *schema.CIConfig, v *bool) { c.Checks.Enabled = v }, defaultVal: false},
		{name: "comments", fn: CommentsEnabled, set: func(c *schema.CIConfig, v *bool) { c.Comments.Enabled = v }, defaultVal: false},
	}

	for _, g := range gates {
		t.Run(g.name, func(t *testing.T) {
			build := func(master bool, flag *bool) *schema.AtmosConfiguration {
				cfg := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: master}}
				g.set(&cfg.CI, flag)
				return cfg
			}
			tests := []struct {
				name string
				cfg  *schema.AtmosConfiguration
				want bool
			}{
				{name: "nil config", cfg: nil, want: false},
				{name: "master switch off, flag true", cfg: build(false, boolPtr(true)), want: false},
				{name: "master switch off, flag unset", cfg: build(false, nil), want: false},
				{name: "flag unset uses default", cfg: build(true, nil), want: g.defaultVal},
				{name: "flag true", cfg: build(true, boolPtr(true)), want: true},
				{name: "flag false", cfg: build(true, boolPtr(false)), want: false},
			}
			for _, tt := range tests {
				t.Run(tt.name, func(t *testing.T) {
					assert.Equal(t, tt.want, g.fn(tt.cfg))
				})
			}
		})
	}
}

func TestWorkerModeOverridesAutomaticCIWithoutDiscardingMetadata(t *testing.T) {
	for _, env := range []string{"ATMOS_CI", "CI", "GITHUB_ACTIONS"} {
		t.Run(env, func(t *testing.T) {
			t.Setenv(env, "true")
			t.Setenv(WorkerModeEnv, "false")
			assert.False(t, ResolveMode(true))
			assert.False(t, ModeEnabled(nil))
			assert.Equal(t, "true", os.Getenv(env))
			cmd := &cobra.Command{Use: "test"}
			cmd.Flags().Bool("ci", false, "")
			require.NoError(t, cmd.Flags().Set("ci", "true"))
			assert.True(t, ModeEnabled(cmd), "explicit child flag wins")
			t.Setenv(WorkerModeEnv, "true")
			require.NoError(t, cmd.Flags().Set("ci", "false"))
			assert.False(t, ModeEnabled(cmd))
			assert.True(t, ResolveMode(false))
		})
	}
}

func TestWorkerModeDisablesProviderGroupingAndReporter(t *testing.T) {
	p := registerGrouping(t)
	config := &schema.AtmosConfiguration{CI: schema.CIConfig{Enabled: true}}
	t.Setenv(WorkerModeEnv, "false")
	ran := false
	require.NoError(t, Group(config, DimensionPhase, "hidden", func() error { ran = true; return nil }))
	assert.True(t, ran)
	assert.Empty(t, p.started)
	reporter := NewReporter(config).(*reporter)
	assert.Nil(t, reporter.detected)
	assert.False(t, reporter.forced)
	// Provider detection remains available independently of the output mode.
	assert.NotNil(t, Detect())
}

// CI reporters can resolve mode while another goroutine renders themed output.
func TestModeEnabledConcurrentThemeBinding(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Cleanup(SwapRegistryForTest())
	t.Setenv(WorkerModeEnv, "")
	t.Setenv("ATMOS_CI_MODE_RACE_TEST", "false")
	t.Setenv("ATMOS_THEME", "dark")
	require.NoError(t, viperguard.BindEnv(modeFlag, "ATMOS_CI_MODE_RACE_TEST"))

	const iterations = 200
	start := make(chan struct{})
	var workers sync.WaitGroup
	workers.Add(2)
	go func() {
		defer workers.Done()
		<-start
		for range iterations {
			assert.NoError(t, viperguard.BindEnv("settings.terminal.theme", "ATMOS_THEME", "THEME"))
		}
	}()
	go func() {
		defer workers.Done()
		<-start
		for range iterations {
			assert.False(t, ModeEnabled(nil))
		}
	}()
	close(start)
	workers.Wait()

	require.Equal(t, "dark", viperguard.GetString("settings.terminal.theme"))
	t.Setenv("ATMOS_CI_MODE_RACE_TEST", "true")
	require.True(t, ModeEnabled(nil), "guarded reads must still honor configured CI mode")
}
