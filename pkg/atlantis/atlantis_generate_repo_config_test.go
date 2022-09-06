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

	atlantisConfig := c.Config.Integrations.Atlantis.AtlantisRepoConfig
	templateName := "atlantis-template-1"
	template := atlantisConfig.Templates[templateName]

	atlantisYaml := c.AtlantisRepoConfigOutput{}
	atlantisYaml.Version = template.Version
	atlantisYaml.Automerge = template.Automerge
	atlantisYaml.DeleteSourceBranchOnMerge = template.DeleteSourceBranchOnMerge
	atlantisYaml.ParallelPlan = template.ParallelPlan
	atlantisYaml.ParallelApply = template.ParallelApply
	atlantisYaml.Workflows = template.Workflows
	atlantisYaml.AllowedRegexpPrefixes = template.AllowedRegexpPrefixes
	atlantisYaml.Projects = []c.AtlantisProjectConfig{atlantisConfig.ProjectTemplate}

	err = utils.PrintAsYAML(atlantisYaml)
	assert.Nil(t, err)
}
