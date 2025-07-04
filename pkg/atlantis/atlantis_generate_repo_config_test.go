package atlantis

import (
	"testing"

	"github.com/stretchr/testify/assert"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestAtlantisGenerateRepoConfig(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = u.PrintAsYAML(&atmosConfig, atmosConfig)
	assert.Nil(t, err)

	atlantisConfig := atmosConfig.Integrations.Atlantis
	configTemplateName := "config-1"
	configTemplate := atlantisConfig.ConfigTemplates[configTemplateName]
	projectTemplateName := "project-1"
	projectTemplate := atlantisConfig.ProjectTemplates[projectTemplateName]
	workflowTemplateName := "workflow-1"
	workflowTemplate := atlantisConfig.ProjectTemplates[workflowTemplateName]
	projectTemplate.Workflow = workflowTemplateName

	atlantisYaml := schema.AtlantisConfigOutput{}
	atlantisYaml.Version = configTemplate.Version
	atlantisYaml.Automerge = configTemplate.Automerge
	atlantisYaml.DeleteSourceBranchOnMerge = configTemplate.DeleteSourceBranchOnMerge
	atlantisYaml.ParallelPlan = configTemplate.ParallelPlan
	atlantisYaml.ParallelApply = configTemplate.ParallelApply
	atlantisYaml.Workflows = map[string]any{workflowTemplateName: workflowTemplate}
	atlantisYaml.AllowedRegexpPrefixes = configTemplate.AllowedRegexpPrefixes
	atlantisYaml.Projects = []schema.AtlantisProjectConfig{projectTemplate}

	err = u.PrintAsYAML(&atmosConfig, atlantisYaml)
	assert.Nil(t, err)
}

func TestExecuteAtlantisGenerateRepoConfig(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = e.ExecuteAtlantisGenerateRepoConfig(
		atmosConfig,
		"/dev/stdout",
		"config-1",
		"project-1",
		nil,
		nil,
	)

	assert.Nil(t, err)
}

func TestExecuteAtlantisGenerateRepoConfig2(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	err = e.ExecuteAtlantisGenerateRepoConfig(
		atmosConfig,
		"/dev/stdout",
		"",
		"",
		nil,
		nil,
	)

	assert.Nil(t, err)
}

func TestExecuteAtlantisGenerateRepoConfigAffectedOnly(t *testing.T) {
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	// We are using `atmos.yaml` from this dir. This `atmos.yaml` has set base_path: "../../tests/fixtures/scenarios/complete",
	// which will be wrong for the remote repo which is cloned into a temp dir.
	// Set the correct base path for the cloned remote repo
	atmosConfig.BasePath = "./tests/fixtures/scenarios/complete"

	err = e.ExecuteAtlantisGenerateRepoConfigAffectedOnly(
		atmosConfig,
		"/dev/stdout",
		"",
		"",
		"",
		"",
		"../..",
		"",
		"",
		false,
		"",
	)

	assert.Nil(t, err)
}
