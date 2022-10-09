package atlantis

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestAtlantisGenerateRepoConfig(t *testing.T) {
	Config, err := c.InitCliConfig(c.ConfigAndStacksInfo{})
	assert.Nil(t, err)

	err = utils.PrintAsYAML(Config)
	assert.Nil(t, err)

	atlantisConfig := Config.Integrations.Atlantis
	configTemplateName := "config-1"
	configTemplate := atlantisConfig.ConfigTemplates[configTemplateName]
	projectTemplateName := "project-1"
	projectTemplate := atlantisConfig.ProjectTemplates[projectTemplateName]
	workflowTemplateName := "workflow-1"
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

func TestExecuteAtlantisGenerateRepoConfig(t *testing.T) {
	Config, err := c.InitCliConfig(c.ConfigAndStacksInfo{})
	assert.Nil(t, err)

	err = e.ExecuteAtlantisGenerateRepoConfig(
		Config,
		"/dev/stdout",
		"config-1",
		"project-1",
		"workflow-1",
		nil,
		nil,
	)

	assert.Nil(t, err)
}
