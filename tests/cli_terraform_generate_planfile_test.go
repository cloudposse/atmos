package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudposse/atmos/cmd"
	"github.com/stretchr/testify/assert"
)

func TestExecuteTerraformGeneratePlanfile(t *testing.T) {
	stacksPath := "./fixtures/scenarios/terraform-generate-planfile"
	componentPath := filepath.Join(stacksPath, "..", "..", "components", "terraform", "mock")
	component := "component-1"
	stack := "nonprod"

	t.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
	t.Setenv("ATMOS_BASE_PATH", stacksPath)
	oldArgs := os.Args
	defer func() {
		os.Args = oldArgs
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join(componentPath, ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join(componentPath, "terraform.tfstate.d"))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.terraform.tfvars.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/new-planfile.json", componentPath))
		assert.NoError(t, err)

		err = os.Remove(fmt.Sprintf("%s/planfiles/new-planfile.yaml", componentPath))
		assert.NoError(t, err)
	}()

	os.Args = []string{"atmos", "terraform", "generate", "planfile", component, "-s", stack, "--format", "json", "--process-templates", "--process-functions"}
	err := cmd.Execute()
	assert.NoError(t, err)

	filePath := fmt.Sprintf("%s/%s-%s.planfile.json", componentPath, stack, component)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	os.Args = []string{"atmos", "terraform", "generate", "planfile", component, "-s", stack, "--format", "yaml", "--process-templates", "--process-functions"}
	err = cmd.Execute()
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/%s-%s.planfile.yaml", componentPath, stack, component)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	os.Args = []string{"atmos", "terraform", "generate", "planfile", component, "-s", stack, "--format", "json", "--file", "new-planfile.json", "--process-templates", "--process-functions"}
	err = cmd.Execute()
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/new-planfile.json", componentPath)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}

	os.Args = []string{"atmos", "terraform", "generate", "planfile", component, "-s", stack, "--format", "yaml", "--file", "planfiles/new-planfile.yaml", "--process-templates", "--process-functions"}
	err = cmd.Execute()
	assert.NoError(t, err)

	filePath = fmt.Sprintf("%s/planfiles/new-planfile.yaml", componentPath)
	if _, err = os.Stat(filePath); os.IsNotExist(err) {
		t.Errorf("Generated planfile does not exist: %s", filePath)
	} else if err != nil {
		t.Errorf("Error checking file: %v", err)
	}
}
