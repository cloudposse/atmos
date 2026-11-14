package terraform

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/component/provideroptions"
)

// Before-hooks describe the component through discovery, so the hook context carries the provider
// options the user set, exactly as RunE resolves them.
func TestPrepareHookContextResolvesProviderOptions(t *testing.T) {
	t.Chdir("../../tests/fixtures/scenarios/terraform-apply-all-dependencies")
	root := &cobra.Command{Use: "terraform"}
	cmd := newHookTestCmd()
	root.AddCommand(cmd)
	options, err := provideroptions.Parse("prerun", []any{
		map[string]any{"name": "feature", "type": "map", "description": "Features.", "commands": []any{"plan"}},
	})
	require.NoError(t, err)
	require.NoError(t, provideroptions.Register(root, []provideroptions.Provider{{Section: "prerun", Name: "prerun", Mode: provideroptions.ModeNative, Options: options}}))
	require.NoError(t, cmd.ParseFlags([]string{"--stack", "reported-order-alias", "--prerun-feature", "tier=premium"}))

	ctx, err := prepareHookContext(cmd, []string{"platform-kms"})

	require.NoError(t, err)
	assert.Equal(t, map[string]any{"tier": "premium"}, ctx.info.ComponentProviderOptions["prerun"]["feature"].Value)
}
