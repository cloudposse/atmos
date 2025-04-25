package exec

import (
	"os"
	"path/filepath"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/stretchr/testify/assert"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

func TestComponentFunc(t *testing.T) {
	log.SetLevel(log.DebugLevel)
	log.SetOutput(os.Stdout)

	// Capture the starting working directory
	startingDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("Failed to get the current working directory: %v", err)
	}

	defer func() {
		// Delete the generated files and folders after the test
		err := os.RemoveAll(filepath.Join("components", "terraform", "mock", ".terraform"))
		assert.NoError(t, err)

		err = os.RemoveAll(filepath.Join("components", "terraform", "mock", "terraform.tfstate.d"))
		assert.NoError(t, err)

		// Change back to the original working directory after the test
		if err = os.Chdir(startingDir); err != nil {
			t.Fatalf("Failed to change back to the starting directory: %v", err)
		}
	}()

	// Define the working directory
	workDir := "../../tests/fixtures/scenarios/stack-templates-3"
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("Failed to change directory to %q: %v", workDir, err)
	}

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

	err = ExecuteTerraform(info)
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
