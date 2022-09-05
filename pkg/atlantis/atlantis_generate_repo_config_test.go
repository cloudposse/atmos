package atlantis

import (
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAtlantisGenerateRepoConfig(t *testing.T) {
	err := c.InitConfig()
	assert.Nil(t, err)

	err = utils.PrintAsYAML(c.Config)
	assert.Nil(t, err)
}
