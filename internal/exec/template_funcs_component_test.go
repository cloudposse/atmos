package exec

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestComponentFunc(t *testing.T) {
	// Skip if terraform is not installed
	if _, err := exec.LookPath("terraform"); err != nil {
		t.Skip("Terraform not found in PATH, skipping test")
	}

	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-3"
	t.Chdir(workDir)

	info := schema.ConfigAndStacksInfo{
		StackFromArg:     "",
		Stack:            "nonprod",
		StackFile:        "",
		ComponentType:    "terraform",
		ComponentFromArg: "component-1",
		SubCommand:       "deploy",
		ProcessTemplates: true,
		ProcessFunctions: true,
	}

	err := ExecuteTerraform(info)
	if err != nil {
		t.Fatalf("Failed to execute 'ExecuteTerraform': %v", err)
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	assert.NoError(t, err)

	// Test terraform component `component-2`
	d, err := componentFunc(&atmosConfig, "component-2", "nonprod")
	assert.NoError(t, err)

	y, err := u.ConvertToYAML(d)
	assert.NoError(t, err)

	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-b--component-1-c")

	// Test helmfile component `component-3`
	d, err = componentFunc(&atmosConfig, "component-3", "nonprod")
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(d)
	assert.NoError(t, err)

	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	assert.Contains(t, y, "baz: component-1-b")

	// Test helmfile component `component-4`
	d, err = componentFunc(&atmosConfig, "component-4", "nonprod")
	assert.NoError(t, err)

	y, err = u.ConvertToYAML(d)
	assert.NoError(t, err)

	assert.Contains(t, y, "foo: component-1-a")
	assert.Contains(t, y, "bar: component-1-b")
	// Helmfile components don't have `outputs` (terraform output) - this should result in `<no value>`
	assert.Contains(t, y, "baz: <no value>")
}
