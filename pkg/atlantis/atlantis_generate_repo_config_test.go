package atlantis

import (
	"github.com/cloudposse/atmos/pkg/utils"
	"github.com/stretchr/testify/assert"
	"testing"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestAtlantisGenerateRepoConfig(t *testing.T) {
	cliConfig, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = utils.PrintAsYAML(cliConfig)
	assert.Nil(t, err)

	atlantisConfig := cliConfig.Integrations.Atlantis
	configTemplateName := "config-1"
	configTemplate := atlantisConfig.ConfigTemplates[configTemplateName]
	projectTemplateName := "project-1"
	projectTemplate := atlantisConfig.ProjectTemplates[projectTemplateName]
	workflowTemplateName := "workflow-1"
	workflowTemplate := atlantisConfig.ProjectTemplates[workflowTemplateName]
	projectTemplate.Workflow = workflowTemplateName

	atlantisYaml := cfg.AtlantisConfigOutput{}
	atlantisYaml.Version = configTemplate.Version
	atlantisYaml.Automerge = configTemplate.Automerge
	atlantisYaml.DeleteSourceBranchOnMerge = configTemplate.DeleteSourceBranchOnMerge
	atlantisYaml.ParallelPlan = configTemplate.ParallelPlan
	atlantisYaml.ParallelApply = configTemplate.ParallelApply
	atlantisYaml.Workflows = map[string]any{workflowTemplateName: workflowTemplate}
	atlantisYaml.AllowedRegexpPrefixes = configTemplate.AllowedRegexpPrefixes
	atlantisYaml.Projects = []cfg.AtlantisProjectConfig{projectTemplate}

	err = utils.PrintAsYAML(atlantisYaml)
	assert.Nil(t, err)
}

func TestExecuteAtlantisGenerateRepoConfig(t *testing.T) {
	cliConfig, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = e.ExecuteAtlantisGenerateRepoConfig(
		cliConfig,
		"/dev/stdout",
		"config-1",
		"project-1",
		nil,
		nil,
	)

	assert.Nil(t, err)
}

func TestExecuteAtlantisGenerateRepoConfig2(t *testing.T) {
	cliConfig, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = e.ExecuteAtlantisGenerateRepoConfig(
		cliConfig,
		"/dev/stdout",
		"",
		"",
		nil,
		nil,
	)

	assert.Nil(t, err)
}
