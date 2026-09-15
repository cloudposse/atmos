package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExperimentalDailyWarningsAcrossInvocations(t *testing.T) {
	_ = NewTestKit(t)
	t.Setenv("ATMOS_EXPERIMENTAL", "warn-daily")
	var notices []string
	original := writeExperimentalNotice
	writeExperimentalNotice = func(feature string) { notices = append(notices, feature) }
	t.Cleanup(func() { writeExperimentalNotice = original })

	for _, name := range []string{"daily-first", "daily-second"} {
		family := &cobra.Command{Use: name, Annotations: map[string]string{"experimental": "true"}}
		for _, sub := range []string{"list", "show"} {
			family.AddCommand(&cobra.Command{Use: sub, Run: func(*cobra.Command, []string) {}})
		}
		RootCmd.AddCommand(family)
		t.Cleanup(func() { RootCmd.RemoveCommand(family) })
	}
	for _, args := range [][]string{
		{"daily-first", "list"},
		{"daily-first", "show"},
		{"daily-second", "list"},
		{"daily-second", "show"},
	} {
		// Simulate independent child invocations with an inherited startup marker.
		t.Setenv("ATMOS_STARTUP_NOTICES_SHOWN", "1")
		RootCmd.SetArgs(args)
		require.NoError(t, Execute())
	}
	assert.Equal(t, []string{"daily-first", "daily-second"}, notices)

	config := &schema.AtmosConfiguration{Edition: "2026-09-14", Settings: schema.AtmosSettings{
		Experimental: "warn-daily", YAML: schema.AtmosYAMLSettings{KeyDelimiter: "."},
	}}
	checkExperimentalSettings(config)
	checkExperimentalSettings(config)
	assert.Equal(t, []string{"daily-first", "daily-second", "settings.yaml.key_delimiter", "edition"}, notices)
}

func TestExperimentalWarningKeyDistinguishesCommandPaths(t *testing.T) {
	_ = NewTestKit(t)
	root := &cobra.Command{Use: "atmos"}
	for _, name := range []string{"terraform", "helmfile"} {
		parent := &cobra.Command{Use: name}
		backend := &cobra.Command{Use: "backend"}
		child := &cobra.Command{Use: "generate"}
		root.AddCommand(parent)
		parent.AddCommand(backend)
		backend.AddCommand(child)
		assert.Equal(t, name+" backend", experimentalWarningKey(child, "backend"))
	}
}
