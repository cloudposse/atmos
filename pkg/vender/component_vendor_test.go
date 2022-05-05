package vender

import (
	e "github.com/cloudposse/atmos/internal/exec"
	c "github.com/cloudposse/atmos/pkg/config"
	"github.com/stretchr/testify/assert"
	"os"
	"path"
	"testing"
)

func TestVendorComponentPullCommand(t *testing.T) {
	component := "infra/vpc-flow-logs-bucket"
	componentType := "terraform"
	vendorCommand := "pull"

	err := c.InitConfig()
	assert.Nil(t, err)

	componentConfig, componentPath, err := e.ReadAndProcessComponentConfigFile(component, componentType)
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
}
