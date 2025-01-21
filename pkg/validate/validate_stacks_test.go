package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/cloudposse/atmos/cmd"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestValidateStacksCommand(t *testing.T) {
	err := e.ExecuteValidateStacksCmd(cmd.ValidateStacksCmd, nil)
	u.LogError(schema.AtmosConfiguration{}, err)
	assert.NotNil(t, err)
}

func TestValidateStacksCommandWithAtmosManifestJsonSchema(t *testing.T) {
	err := e.ExecuteValidateStacksCmd(cmd.ValidateStacksCmd, []string{"--schemas-atmos-manifest", "../../internal/exec/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"})
	u.LogError(schema.AtmosConfiguration{}, err)
	assert.NotNil(t, err)
}

func TestValidateStacksCommandWithRemoteAtmosManifestJsonSchema(t *testing.T) {
	err := e.ExecuteValidateStacksCmd(cmd.ValidateStacksCmd, []string{"--schemas-atmos-manifest", "https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json"})
	u.LogError(schema.AtmosConfiguration{}, err)
	assert.NotNil(t, err)
}
