package validate

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/flags"
)

func TestValidateStacksCommand(t *testing.T) {
	opts := &flags.StandardOptions{}
	err := e.ExecuteValidateStacksCmd(opts)
	assert.NotNil(t, err)
}

func TestValidateStacksCommandWithAtmosManifestJsonSchema(t *testing.T) {
	opts := &flags.StandardOptions{
		SchemasAtmosManifest: "../../internal/exec/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json",
	}
	err := e.ExecuteValidateStacksCmd(opts)
	assert.NotNil(t, err)
}

func TestValidateStacksCommandWithRemoteAtmosManifestJsonSchema(t *testing.T) {
	opts := &flags.StandardOptions{
		SchemasAtmosManifest: "https://atmos.tools/schemas/atmos/atmos-manifest/1.0/atmos-manifest.json",
	}
	err := e.ExecuteValidateStacksCmd(opts)
	assert.NotNil(t, err)
}
