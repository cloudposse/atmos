package config

import (
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"
)

// preserveEmptyGitPushRetry prevents Viper from dropping an explicit empty policy.
// Empty shared policies mean one attempt, rather than the omitted legacy defaults.
func preserveEmptyGitPushRetry(node *yaml.Node, v *viper.Viper, path string) {
	if !strings.HasPrefix(path, "git.repositories.") || !strings.HasSuffix(path, ".push.retry") {
		return
	}
	if node != nil && node.Kind == yaml.AliasNode {
		node = node.Alias
	}
	if node != nil && node.Kind == yaml.MappingNode && len(node.Content) == 0 {
		v.Set(path+".max_attempts", 1)
	}
}
