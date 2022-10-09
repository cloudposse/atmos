package vender

import (
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"testing"

	e "github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
)

func TestVendorComponentPullCommand(t *testing.T) {
	cliConfig, err := cfg.InitCliConfig(cfg.ConfigAndStacksInfo{}, true)
	assert.Nil(t, err)

	componentType := "terraform"
	vendorCommand := "pull"

	// Test 'infra/vpc-flow-logs-bucket' component
	component := "infra/vpc-flow-logs-bucket"
	componentConfig, componentPath, err := e.ReadAndProcessComponentConfigFile(cliConfig, component, componentType)
	assert.Nil(t, err)

	err = e.ExecuteComponentVendorCommandInternal(componentConfig.Spec, component, componentPath, false, vendorCommand)
	assert.Nil(t, err)

	// Check if the correct files were pulled and written to the correct folder
	assert.FileExists(t, path.Join(componentPath, "context.tf"))
	assert.FileExists(t, path.Join(componentPath, "default.auto.tfvars"))
	assert.FileExists(t, path.Join(componentPath, "introspection.mixin.tf"))
	assert.FileExists(t, path.Join(componentPath, "main.tf"))
	assert.FileExists(t, path.Join(componentPath, "outputs.tf"))
	assert.FileExists(t, path.Join(componentPath, "providers.tf"))
	assert.FileExists(t, path.Join(componentPath, "README.md"))
	assert.FileExists(t, path.Join(componentPath, "variables.tf"))
	assert.FileExists(t, path.Join(componentPath, "versions.tf"))

	// Delete the files
	err = os.Remove(path.Join(componentPath, "context.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "default.auto.tfvars"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "introspection.mixin.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "main.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "outputs.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "providers.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "README.md"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "variables.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "versions.tf"))
	assert.Nil(t, err)

	// Test 'infra/account-map' component
	component = "infra/account-map"
	componentConfig, componentPath, err = e.ReadAndProcessComponentConfigFile(cliConfig, component, componentType)
	assert.Nil(t, err)

	err = e.ExecuteComponentVendorCommandInternal(componentConfig.Spec, component, componentPath, false, vendorCommand)
	assert.Nil(t, err)

	// Check if the correct files were pulled and written to the correct folder
	assert.FileExists(t, path.Join(componentPath, "context.tf"))
	assert.FileExists(t, path.Join(componentPath, "default.auto.tfvars"))
	assert.FileExists(t, path.Join(componentPath, "introspection.mixin.tf"))
	assert.FileExists(t, path.Join(componentPath, "main.tf"))
	assert.FileExists(t, path.Join(componentPath, "outputs.tf"))
	assert.FileExists(t, path.Join(componentPath, "providers.tf"))
	assert.FileExists(t, path.Join(componentPath, "README.md"))
	assert.FileExists(t, path.Join(componentPath, "remote-state.tf"))
	assert.FileExists(t, path.Join(componentPath, "variables.tf"))
	assert.FileExists(t, path.Join(componentPath, "versions.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "iam-roles", "context.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "iam-roles", "main.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "iam-roles", "outputs.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "iam-roles", "variables.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "roles-to-principals", "context.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "roles-to-principals", "main.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "roles-to-principals", "outputs.tf"))
	assert.FileExists(t, path.Join(componentPath, "modules", "roles-to-principals", "variables.tf"))

	// Delete the files and folders
	err = os.Remove(path.Join(componentPath, "context.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "default.auto.tfvars"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "introspection.mixin.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "main.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "outputs.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "providers.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "README.md"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "remote-state.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "variables.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "versions.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "iam-roles", "context.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "iam-roles", "main.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "iam-roles", "outputs.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "iam-roles", "variables.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "roles-to-principals", "context.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "roles-to-principals", "main.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "roles-to-principals", "outputs.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "roles-to-principals", "variables.tf"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "iam-roles"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules", "roles-to-principals"))
	assert.Nil(t, err)
	err = os.Remove(path.Join(componentPath, "modules"))
	assert.Nil(t, err)
}
