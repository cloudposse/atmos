package validate

import (
	"github.com/cloudposse/atmos/pkg/schema"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/cmd"
	e "github.com/cloudposse/atmos/internal/exec"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestValidateStacksCommand(t *testing.T) {
	err := e.ExecuteValidateStacksCmd(cmd.ValidateStacksCmd, nil)
	u.LogError(schema.CliConfiguration{}, err)
	assert.NotNil(t, err)
}

func TestValidateStacksCommandWithAtmosManifestJsonSchema(t *testing.T) {
	err := e.ExecuteValidateStacksCmd(cmd.ValidateStacksCmd, []string{"--schemas-atmos-manifest", "../quick-start-advanced/stacks/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"})
	u.LogError(schema.CliConfiguration{}, err)
	assert.NotNil(t, err)
}
