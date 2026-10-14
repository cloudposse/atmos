package cmd

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/script"
	starlarkengine "github.com/cloudposse/atmos/pkg/script/starlark"
)

func TestScriptCatalogUsesRegisteredCommandsAndFlags(t *testing.T) {
	NewTestKit(t)
	RootCmd.InitDefaultHelpCmd()
	catalog := internal.ScriptCommandCatalog(RootCmd)
	for _, providers := range internal.ListProviders() {
		for _, provider := range providers {
			assert.Contains(t, catalog.Names(), provider.GetCommand().Name())
		}
	}
	assert.Equal(t, "-detailed-exitcode", catalog.FlagName([]string{"terraform", "plan", "app"}, "detailed-exitcode"))
	assert.Equal(t, "-var", catalog.FlagName([]string{"terraform", "plan", "app"}, "var"))
	assert.Equal(t, "--stack", catalog.FlagName([]string{"terraform", "plan", "app"}, "s"))
	for _, name := range []string{"support", "validate", "help"} {
		assert.Contains(t, catalog.Names(), name)
	}
	var stdout bytes.Buffer
	result, err := starlarkengine.New(starlarkengine.WithAtmosCommands(catalog)).Execute(context.Background(), script.Spec{
		Source: `output = hasattr(atmos, "version") and hasattr(atmos, "describe") and not hasattr(atmos, "not_registered")`,
		Stdout: &stdout,
	})
	require.NoError(t, err)
	assert.Equal(t, "true", result.Value)
}
