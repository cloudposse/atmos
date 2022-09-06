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

	atlantisConfig := c.Config.Integrations.Atlantis
	configTemplateName := "config-template-1"
	configTemplate := atlantisConfig.ConfigTemplates[configTemplateName]
	projectTemplateName := "project-template-1"
	projectTemplate := atlantisConfig.ProjectTemplates[projectTemplateName]
	workflowTemplateName := "workflow-template-1"
	workflowTemplate := atlantisConfig.ProjectTemplates[workflowTemplateName]
	projectTemplate.Workflow = workflowTemplateName

	atlantisYaml := c.AtlantisConfigOutput{}
	atlantisYaml.Version = configTemplate.Version
	atlantisYaml.Automerge = configTemplate.Automerge
	atlantisYaml.DeleteSourceBranchOnMerge = configTemplate.DeleteSourceBranchOnMerge
	atlantisYaml.ParallelPlan = configTemplate.ParallelPlan
	atlantisYaml.ParallelApply = configTemplate.ParallelApply
	atlantisYaml.Workflows = map[string]any{workflowTemplateName: workflowTemplate}
	atlantisYaml.AllowedRegexpPrefixes = configTemplate.AllowedRegexpPrefixes
	atlantisYaml.Projects = []c.AtlantisProjectConfig{projectTemplate}

	err = utils.PrintAsYAML(atlantisYaml)
	assert.Nil(t, err)
}
