package config

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Retain every loaded layer's security requirements after ordinary Viper merging.
// Each publisher set is an OR-list; layers are ANDed against the same signer.
// This preserves wildcard narrowing without trying to intersect glob strings.
func applyPluginPolicy(v *viper.Viper, config *schema.AtmosConfiguration) error {
	policy := &config.Plugins.Policy
	policy.PublisherConstraints = nil
	for _, source := range mergedFilesReg.snapshot(v) {
		layer, err := pluginPolicyFromSource(source)
		if err != nil {
			return fmt.Errorf("%w: plugin policy in %s: %w", errUtils.ErrPluginInvalidConfig, source.path, err)
		}
		mergePluginPolicy(policy, layer)
	}
	return nil
}

func pluginPolicyFromSource(source mergedConfigSource) (*schema.PluginPolicy, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source.content), &document); err != nil {
		return nil, err
	}
	node := policyMappingValue(&document, "plugins")
	policy := &schema.PluginPolicy{}
	if node == nil {
		return policy, nil
	}
	value, err := decodePluginPolicy(node, source.path)
	if err != nil {
		return nil, err
	}
	layer := viper.New()
	if err := layer.MergeConfigMap(map[string]any{"policy": value}); err != nil {
		return nil, err
	}
	if err := layer.UnmarshalKey("policy", policy, atmosDecodeHook()); err != nil {
		return nil, err
	}
	return policy, nil
}

func decodePluginPolicy(node *yaml.Node, path string) (any, error) {
	// Evaluate only the policy when the plugin section is a literal mapping;
	// otherwise resolve a section supplied by !include through the same loader.
	section := node.Kind != yaml.MappingNode
	if !section {
		node = policyMappingValue(node, "policy")
	}
	value, err := decodeNodeWithYamlFunctionsForFile(node, path)
	if err != nil {
		return nil, err
	}
	if value == nil {
		return nil, nil
	}
	if section {
		plugins, ok := value.(map[string]any)
		if !ok {
			return nil, errUtils.ErrPluginInvalidConfig
		}
		value = nil
		for key, item := range plugins {
			if strings.EqualFold(key, "policy") {
				value = item
			}
		}
	}
	return value, nil
}

// Match Viper's case-insensitive configuration keys while preserving values.
func policyMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node.Kind == yaml.DocumentNode && len(node.Content) != 0 {
		node = node.Content[0]
	}
	if node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if strings.EqualFold(node.Content[i].Value, key) {
			return node.Content[i+1]
		}
	}
	return nil
}

func mergePluginPolicy(policy, layer *schema.PluginPolicy) {
	policy.RequireSignature = policy.RequireSignature || layer.RequireSignature || len(layer.TrustedPublishers) != 0
	policy.Offline = policy.Offline || layer.Offline
	if len(layer.TrustedPublishers) != 0 {
		policy.PublisherConstraints = append(policy.PublisherConstraints, append([]schema.PluginPublisher(nil), layer.TrustedPublishers...))
	}
}
