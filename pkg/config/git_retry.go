package config

import (
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// preserveEmptyGitPushRetry prevents Viper from dropping an explicit empty policy.
// Empty shared policies mean one attempt, rather than the omitted legacy defaults.
func preserveEmptyGitPushRetry(node *yaml.Node, v *viper.Viper, path string) error {
	if !strings.HasPrefix(path, "git.repositories.") || !strings.HasSuffix(path, ".push.retry") {
		return nil
	}
	if node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node != nil && node.Kind == yaml.MappingNode && len(node.Content) == 0 {
		// Store normalization in the config register so later files and !unset win.
		v.Set(path, nil)
		value := map[string]any{"max_attempts": 1}
		segments := strings.Split(path, ".")
		for i := len(segments) - 1; i >= 0; i-- {
			value = map[string]any{segments[i]: value}
		}
		return v.MergeConfigMap(value)
	}
	return nil
}
