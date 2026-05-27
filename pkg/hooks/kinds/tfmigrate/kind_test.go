package tfmigrate

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	tfmigrate "github.com/cloudposse/atmos/pkg/terraform/tfmigrate"
)

func TestKindRegistered(t *testing.T) {
	kind, ok := hooks.GetKind("tfmigrate")
	require.True(t, ok)
	assert.Equal(t, tfmigrate.Command, kind.Command)
	assert.Equal(t, hooks.OnFailureFail, kind.OnFailure)
}

func TestAtmosArgs(t *testing.T) {
	args, err := atmosArgs(&hooks.ExecContext{
		Hook: &hooks.Hook{
			Migration:     "migrations/001.hcl",
			Config:        ".tfmigrate.hcl",
			BackendConfig: []string{"bucket=state"},
		},
		Info: &schema.ConfigAndStacksInfo{
			ComponentFromArg: "vpc",
			Stack:            "plat-ue2-dev",
			Identity:         "dev",
		},
	}, tfmigrate.ActionApply)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"terraform", "migrate", "apply",
		"vpc",
		"--stack", "plat-ue2-dev",
		"--identity", "dev",
		"--migration", "migrations/001.hcl",
		"--tfmigrate-config", ".tfmigrate.hcl",
		"--backend-config", "bucket=state",
	}, args)
}
