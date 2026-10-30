package list

import (
	"path/filepath"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
)

// instanceEngine describes the selected executor without running or locating it.
// A configuration provider can declare an engine independently of its own name.
func instanceEngine(config *schema.AtmosConfiguration, kind string, component map[string]any) string {
	settings, _ := component["settings"].(map[string]any)
	provider, _ := settings["configuration"].(map[string]any)
	if engine, ok := provider["engine"].(string); ok && engine != "" {
		return engine
	}
	if kind != "terraform" {
		return kind
	}
	command, _ := component["command"].(string)
	if command == "" {
		command = config.Components.Terraform.Command
	}
	if strings.TrimSuffix(filepath.Base(command), ".exe") == "tofu" {
		return "opentofu"
	}
	return "terraform"
}

// populateInstanceEngines uses the same resolved component command as execution.
// It does not start plugins, resolve credentials, or invoke a toolchain binary.
func populateInstanceEngines(config *schema.AtmosConfiguration, stacks map[string]any, instances []schema.Instance) {
	for i := range instances {
		instance := &instances[i]
		stack, _ := stacks[instance.Stack].(map[string]any)
		components, _ := stack["components"].(map[string]any)
		family, _ := components[instance.ComponentType].(map[string]any)
		component, _ := family[instance.Component].(map[string]any)
		instance.Engine = instanceEngine(config, instance.ComponentType, component)
	}
}
