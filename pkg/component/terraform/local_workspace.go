package terraform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	stackadoption "github.com/cloudposse/atmos/pkg/stack/adoption"
	"github.com/cloudposse/atmos/pkg/terraform/backend"
)

// A standalone adopted root already has a native workspace. Set it on the
// plan process, including implicit init, so another terminal keeps its selection.
func scopeLocalPlanWorkspace(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, directory string) error {
	workspace, ok := standalonePlanWorkspace(config, info)
	if !ok {
		return nil
	}
	layout, err := backend.ResolveLocalState(directory, strings.Contains(filepath.Base(info.Command), "tofu"))
	if errors.Is(err, backend.ErrNonLocalBackend) {
		return nil
	}
	if err != nil {
		return err
	}
	// Preserve normal implicit creation on a hand-authored first deployment.
	if workspace != "default" {
		if _, err := os.Stat(filepath.Join(layout.Workspaces, workspace)); os.IsNotExist(err) {
			return nil
		} else if err != nil {
			return err
		}
	}
	if err := checkConfigurationWorkspace(info); err != nil {
		return err
	}
	info.ComponentEnvList = append(info.ComponentEnvList, "TF_WORKSPACE="+workspace)
	return nil
}

func standalonePlanWorkspace(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (string, bool) {
	workspace, _ := info.ComponentMetadataSection["terraform_workspace"].(string)
	root, _ := info.ComponentMetadataSection["component"].(string)
	return workspace, info.SubCommand == "plan" && !config.Components.Terraform.AutoGenerateBackendFile &&
		stackadoption.StandaloneStacks(&config.Stacks) && !schema.ConfigurationOwned(info.ComponentSettingsSection) &&
		root != "" && workspace != "" && workspace == info.TerraformWorkspace
}

func hasScopedWorkspace(info *schema.ConfigAndStacksInfo) bool {
	workspace, _ := envListLookup(info.ComponentEnvList)("TF_WORKSPACE")
	return workspace != ""
}
