package spacelift

import (
	"gopkg.in/yaml.v2"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpaceliftStackProcessor(t *testing.T) {
	basePath := "../../examples/complete/stacks"

	filePaths := []string{
		"../../examples/complete/stacks/tenant1/ue2/dev.yaml",
		"../../examples/complete/stacks/tenant1/ue2/prod.yaml",
		"../../examples/complete/stacks/tenant1/ue2/staging.yaml",
		"../../examples/complete/stacks/tenant2/ue2/dev.yaml",
		"../../examples/complete/stacks/tenant2/ue2/prod.yaml",
		"../../examples/complete/stacks/tenant2/ue2/staging.yaml",
	}

	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	var spaceliftStacks, err = CreateSpaceliftStacks(basePath, filePaths, processStackDeps, processComponentDeps, processImports, stackConfigPathTemplate)
	assert.Nil(t, err)

	yamlSpaceliftStacks, err := yaml.Marshal(spaceliftStacks)
	assert.Nil(t, err)
	t.Log(string(yamlSpaceliftStacks))
}
