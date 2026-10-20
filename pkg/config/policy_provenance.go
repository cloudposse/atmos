package config

import (
	"context"

	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/policy/admission"
	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// rememberConfigOrigins records raw scalars before evaluation or Viper merging.
func rememberConfigOrigins(node *yaml.Node, file string) {
	if node == nil {
		return
	}
	if node.Kind == yaml.ScalarNode {
		guard.RememberValue(node.Tag+" "+node.Value, guard.FileOrigin(file))
		guard.RememberValue(node.Value, guard.FileOrigin(file))
	}
	for _, child := range node.Content {
		rememberConfigOrigins(child, file)
	}
}

func admitConfigInclude(file, target string) error {
	if err := admission.FunctionAt(context.Background(), "include", file); err != nil {
		return err
	}
	return guard.Source(context.Background(), "include", target, file)
}
