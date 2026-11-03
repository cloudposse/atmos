package config

import (
	"encoding/hex"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/schema"
)

// Viper normalizes dictionary keys. Encode executable-input dictionaries in
// devcontainer specs and MCP environments, including distinct names such as FOO/foo.
const (
	devcontainerKeyPrefix = "__atmos_devcontainer_key_"
	devcontainerConfigKey = "devcontainer"
)

func devcontainerCaseMap(path string) bool {
	path = strings.ToLower(path)
	return (strings.HasPrefix(path, "devcontainer.") &&
		(strings.HasSuffix(path, ".containerenv") || strings.HasSuffix(path, ".build.args"))) ||
		(strings.HasPrefix(path, "mcp.servers.") && strings.HasSuffix(path, ".env"))
}

func encodeDevcontainerKeys(node *yaml.Node, path string) {
	encodeDevcontainerNode(node, path, make(map[*yaml.Node]bool), make(map[*yaml.Node]bool))
}

func encodeDevcontainerNode(node *yaml.Node, path string, active, encoded map[*yaml.Node]bool) {
	if active[node] {
		return
	}
	active[node] = true
	defer delete(active, node)
	expandDevcontainerAlias(node, active)

	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content); i += 2 {
			key := *node.Content[i]
			childPath := path + "." + key.Value
			if key.Tag == "!!merge" {
				// A merge contributes keys to this mapping, not to a child named <<.
				childPath = path
			} else if devcontainerCaseMap(path) && !encoded[node.Content[i]] && !strings.HasPrefix(key.Value, devcontainerKeyPrefix) {
				key.Value = devcontainerKeyPrefix + hex.EncodeToString([]byte(key.Value))
				node.Content[i] = &key
				encoded[&key] = true
			}
			encodeDevcontainerNode(node.Content[i+1], childPath, active, encoded)
		}
	case yaml.SequenceNode, yaml.DocumentNode:
		for _, child := range node.Content {
			encodeDevcontainerNode(child, path, active, encoded)
		}
	}
}

func expandDevcontainerAlias(node *yaml.Node, active map[*yaml.Node]bool) {
	if node.Kind == yaml.AliasNode && node.Alias != nil && !active[node.Alias] {
		source := node.Alias
		copyNode := *source
		copyNode.Content = append([]*yaml.Node(nil), source.Content...)
		*node = copyNode
	}
}

// prepareDevcontainerKeys replaces the raw, case-folded dictionaries loaded by
// Viper before function evaluation. Existing encoded import values survive the
// merge; the current source overrides only the keys it actually declares.
func prepareDevcontainerKeys(root *yaml.Node, v *viper.Viper) error {
	if len(root.Content) == 0 || root.Content[0].Kind != yaml.MappingNode {
		return nil
	}
	mapping := root.Content[0]
	for i := 0; i < len(mapping.Content); i += 2 {
		section := mapping.Content[i].Value
		if section != devcontainerConfigKey && section != "mcp" {
			continue
		}
		node := mapping.Content[i+1]
		encodeDevcontainerKeys(node, section)
		var value any
		if err := node.Decode(&value); err != nil {
			return err
		}
		v.Set(section, mergeDevcontainerCaseValues(v.Get(section), value, section))
	}
	return nil
}

func mergeDevcontainerCaseValues(existing, incoming any, path string) any {
	current, currentOK := existing.(map[string]any)
	next, nextOK := incoming.(map[string]any)
	if !nextOK {
		return incoming
	}
	result := make(map[string]any, len(current)+len(next))
	if currentOK {
		for key, value := range current {
			if !devcontainerCaseMap(path) || strings.HasPrefix(key, devcontainerKeyPrefix) {
				result[key] = value
			}
		}
	}
	for key, value := range next {
		key = strings.ToLower(key)
		result[key] = mergeDevcontainerCaseValues(result[key], value, path+"."+key)
	}
	return result
}

func restoreDevcontainerKeys(value any) {
	restoreDevcontainerCaseValues(value, devcontainerConfigKey)
}

func restoreDevcontainerCaseValues(value any, path string) {
	switch data := value.(type) {
	case map[string]any:
		restored := make(map[string]any)
		for key, child := range data {
			restoreDevcontainerCaseValues(child, path+"."+key)
			if !devcontainerCaseMap(path) || !strings.HasPrefix(key, devcontainerKeyPrefix) {
				continue
			}
			original, err := hex.DecodeString(strings.TrimPrefix(key, devcontainerKeyPrefix))
			if err == nil {
				restored[string(original)] = child
			}
		}
		if len(restored) > 0 {
			clear(data)
			for key, child := range restored {
				data[key] = child
			}
		}
	case []any:
		for _, child := range data {
			restoreDevcontainerCaseValues(child, path)
		}
	}
}

// restoreMCPEnvironmentKeys restores the typed map after configuration decoding.
func restoreMCPEnvironmentKeys(servers map[string]schema.MCPServerConfig) {
	for name := range servers {
		server := servers[name]
		restored := make(map[string]string, len(server.Env))
		for key, value := range server.Env {
			if !strings.HasPrefix(key, devcontainerKeyPrefix) {
				continue
			}
			original, err := hex.DecodeString(strings.TrimPrefix(key, devcontainerKeyPrefix))
			if err == nil {
				restored[string(original)] = value
			}
		}
		if len(restored) > 0 {
			server.Env = restored
			servers[name] = server
		}
	}
}
