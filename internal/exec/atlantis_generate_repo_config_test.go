package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestExecuteAtlantisGenerateRepoConfig(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = ExecuteAtlantisGenerateRepoConfig(
		atmosConfig,
		"/dev/stdout",
		"config-1",
		"project-1",
		nil,
		nil,
	)

	assert.Nil(t, err)
}
