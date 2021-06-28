package spacelift

import (
	"gopkg.in/yaml.v2"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestSpaceliftStackProcessor(t *testing.T) {
	filePaths := []string{
		"../../examples/data-sources/utils_spacelift_stack_config/stacks/uw2-dev.yaml",
		"../../examples/data-sources/utils_spacelift_stack_config/stacks/uw2-prod.yaml",
		"../../examples/data-sources/utils_spacelift_stack_config/stacks/uw2-staging.yaml",
		"../../examples/data-sources/utils_spacelift_stack_config/stacks/uw2-uat.yaml",
		"../../examples/data-sources/utils_spacelift_stack_config/stacks/gbl-dns.yaml",
	}

	processStackDeps := true
	processComponentDeps := true
	processImports := true
	stackConfigPathTemplate := "stacks/%s.yaml"

	var spaceliftStacks, err = CreateSpaceliftStacks(filePaths, processStackDeps, processComponentDeps, processImports, stackConfigPathTemplate)
	assert.Nil(t, err)

	yamlSpaceliftStacks, err := yaml.Marshal(spaceliftStacks)
	assert.Nil(t, err)
	t.Log(string(yamlSpaceliftStacks))
}
