package cmd

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/pkg/script"
	starlarkengine "github.com/cloudposse/atmos/pkg/script/starlark"
)

// registerScriptCommands wires the completed host command tree into the engine.
func registerScriptCommands(root *cobra.Command) {
	root.InitDefaultHelpCmd()
	script.Register("starlark", starlarkengine.New(starlarkengine.WithAtmosCommands(internal.ScriptCommandCatalog(root))))
}
