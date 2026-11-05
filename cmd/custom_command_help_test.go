package cmd

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
)

func TestCustomCommandUse(t *testing.T) {
	cfg := &schema.Command{
		Name: "deploy",
		Arguments: []schema.CommandArgument{
			{Name: "component", Required: true},
			{Name: "region"},
			{Name: "tier", Required: true, Default: "free"},
		},
	}
	assert.Equal(t, "deploy <component> [region] [tier]", customCommandUse(cfg))
	assert.Equal(t, "plain", customCommandUse(&schema.Command{Name: "plain"}))
}

func TestCustomCommandUseDoesNotChangeTheCommandName(t *testing.T) {
	_ = NewTestKit(t)
	parent := &cobra.Command{Use: "atmos"}
	cmd, err := createCustomCommand(&schema.AtmosConfiguration{}, &schema.Command{
		Name:      "deploy",
		Arguments: []schema.CommandArgument{{Name: "component", Required: true}},
	}, parent)
	require.NoError(t, err)
	assert.Equal(t, "deploy", cmd.Name())
	parent.AddCommand(cmd)
	assert.Contains(t, cmd.UseLine(), "deploy <component>")
}

func TestPrintCustomCommandArguments(t *testing.T) {
	_ = NewTestKit(t)
	cmd, err := createCustomCommand(&schema.AtmosConfiguration{}, &schema.Command{
		Name: "deploy",
		Arguments: []schema.CommandArgument{
			{Name: "component", Description: "Component to deploy", Required: true},
			{Name: "region", Description: "Target region", Default: "us-east-2", Values: []string{"us-east-2", "eu-west-1"}},
		},
	}, &cobra.Command{Use: "atmos"})
	require.NoError(t, err)

	var out bytes.Buffer
	styles := createHelpStyles(nil)
	printCustomCommandArguments(&out, cmd, &styles)
	got := out.String()
	assert.Contains(t, got, "ARGUMENTS")
	assert.Contains(t, got, "Component to deploy (required)")
	assert.Contains(t, got, `Target region (default "us-east-2"; one of: us-east-2, eu-west-1)`)

	var none bytes.Buffer
	printCustomCommandArguments(&none, &cobra.Command{Use: "x"}, &styles)
	assert.Empty(t, none.String())
}

func TestCustomCommandStringFlagDefaultIsQuoted(t *testing.T) {
	_ = NewTestKit(t)
	cmd, err := createCustomCommand(&schema.AtmosConfiguration{}, &schema.Command{
		Name: "scale",
		Flags: []schema.CommandFlag{
			{Name: "replicas", Default: "2", Description: "Replica count as a string"},
			{Name: "count", Type: "int", Default: 3, Description: "Count"},
		},
	}, &cobra.Command{Use: "atmos"})
	require.NoError(t, err)

	assert.Equal(t, `Replica count as a string (default "2")`, buildFlagDescription(cmd.PersistentFlags().Lookup("replicas")))
	assert.Equal(t, "Count (default `3`)", buildFlagDescription(cmd.PersistentFlags().Lookup("count")))
}
