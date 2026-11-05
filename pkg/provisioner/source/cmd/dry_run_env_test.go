package cmd

import (
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceDryRun_EnvironmentPrecedence(t *testing.T) {
	for _, verb := range []string{"pull", "delete"} {
		for _, tt := range []struct {
			name      string
			env       string
			cli       []string
			inherited bool
			dryRun    bool
		}{
			{name: "environment true", env: "true", inherited: true, dryRun: true},
			{name: "environment false", env: "false", inherited: true},
			{name: "invalid environment", env: "invalid", inherited: true},
			{name: "empty environment", inherited: true},
			{name: "explicit false", env: "true", cli: []string{"--dry-run=false"}, inherited: true},
			{name: "explicit true", env: "false", cli: []string{"--dry-run"}, inherited: true, dryRun: true},
			{name: "absent flag", env: "true"},
		} {
			t.Run(verb+"/"+tt.name, func(t *testing.T) {
				t.Setenv("ATMOS_DRY_RUN", tt.env)
				env := newDryRunEnv(t, dryRunScenario{section: vpcSection(nil), existing: true, provisioned: true})
				viper.Set("dry-run", !tt.dryRun)
				root := mountSourceCommand(t, verb, tt.inherited)
				root.SetArgs(append([]string{"source", verb, "vpc", "--stack", "dev", "--force"}, tt.cli...))
				require.NoError(t, root.Execute())
				switch {
				case tt.dryRun:
					assert.Zero(t, env.provisions)
					assert.DirExists(t, env.targetDir)
				case verb == "pull":
					assert.Equal(t, 1, env.provisions)
				default:
					assert.NoDirExists(t, env.targetDir)
				}
			})
		}
	}
}
