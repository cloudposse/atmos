package exec

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	provWorkdir "github.com/cloudposse/atmos/pkg/provisioner/workdir"
	"github.com/cloudposse/atmos/pkg/schema"
)

// TestPrintAndWriteVarFiles_WorkspaceSubcommand_PassVars covers the case where the workspace subcommand
// still runs `terraform init -var-file <varfile>` because init.pass_vars is enabled. The varfile must
// exist by then, or init fails with "Failed to read variables file" on a component that was never planned.
// Without pass_vars the varfile is unused, so workspace must keep skipping the write.
func TestPrintAndWriteVarFiles_WorkspaceSubcommand_PassVars(t *testing.T) {
	tests := []struct {
		name        string
		passVars    bool
		subCommand  string
		expectWrite bool
	}{
		{name: "workspace with pass_vars writes the varfile", passVars: true, subCommand: subcommandWorkspace, expectWrite: true},
		{name: "workspace without pass_vars does not write the varfile", passVars: false, subCommand: subcommandWorkspace, expectWrite: false},
		{name: "plan with pass_vars still writes the varfile", passVars: true, subCommand: "plan", expectWrite: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			workdir := t.TempDir()

			atmosConfig := schema.AtmosConfiguration{}
			atmosConfig.Components.Terraform.Init.PassVars = tt.passVars

			info := schema.ConfigAndStacksInfo{
				SubCommand:           tt.subCommand,
				ContextPrefix:        "dev",
				Component:            "demo",
				ComponentVarsSection: map[string]any{"greeting": "hello"},
				ComponentSection:     map[string]any{provWorkdir.WorkdirPathKey: workdir},
			}

			require.NoError(t, printAndWriteVarFiles(&atmosConfig, &info))

			varFilePath := filepath.Join(workdir, constructTerraformComponentVarfileName(&info))
			_, err := os.Stat(varFilePath)
			if !tt.expectWrite {
				assert.ErrorIs(t, err, os.ErrNotExist, "varfile must not be written")
				return
			}
			require.NoError(t, err, "varfile must exist for init -var-file")

			content, err := os.ReadFile(varFilePath)
			require.NoError(t, err)
			assert.Contains(t, string(content), `"greeting": "hello"`)
		})
	}
}
