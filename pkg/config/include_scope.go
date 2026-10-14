package config

import (
	"os"
	"path/filepath"

	goyaml "go.yaml.in/yaml/v3"

	"github.com/cloudposse/atmos/pkg/yaml/includescope"
)

// scriptSourceKey is the internal key that carries the file a custom command step's `script`
// was read from through the merged command configuration (see schema.Task.ScriptSource).
const scriptSourceKey = "script_source"

const scriptKey = "script"

// commandIncludeScope returns the scope that anchors local !include paths in the custom command
// definitions of sourceFile, so they resolve identically wherever Atmos runs from:
// "./x" and "../x" against the directory of sourceFile, bare "x/y" against the project base path.
func commandIncludeScope(sourceFile string) includescope.Scope {
	absFile, err := filepath.Abs(sourceFile)
	if err != nil {
		absFile = sourceFile
	}
	return includescope.Scope{File: absFile, BasePath: commandIncludeBasePath(absFile)}
}

// commandIncludeBasePath returns the absolute project base path for the config file that
// defines custom commands. The commands are decoded while the configuration is still being
// assembled, so the final base path is not known yet; it is derived the same way: the runtime
// override first (the --base-path flag, then ATMOS_BASE_PATH, read from os.Args and the
// environment like the other config-selection flags), then the `base_path` the file itself
// declares, and otherwise the directory of the config file (the parent of the enclosing
// `atmos.d` directory). Dot-relative runtime values resolve against the working directory.
func commandIncludeBasePath(sourceFile string) string {
	configDir := filepath.Dir(sourceFile)
	fallback := includeBasePathForSourceFile(sourceFile)
	if runtimePath := getConfigSelectionFromFlagsOrEnv().BasePath; runtimePath != "" {
		if resolved, err := resolveAbsolutePath(runtimePath, configDir, basePathSourceRuntime); err == nil {
			return resolved
		}
		return fallback
	}
	declared := declaredBasePath(sourceFile)
	if declared == "" {
		return fallback
	}
	resolved, err := resolveAbsolutePath(declared, configDir, "")
	if err != nil {
		return fallback
	}
	return resolved
}

// declaredBasePath returns the plain `base_path` scalar declared at the top level of the file,
// or "" when it declares none or computes it with a YAML function.
func declaredBasePath(file string) string {
	content, err := os.ReadFile(file)
	if err != nil {
		return ""
	}
	var root goyaml.Node
	if err := goyaml.Unmarshal(content, &root); err != nil || len(root.Content) == 0 {
		return ""
	}
	mapping := root.Content[0]
	if mapping.Kind != goyaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		value := mapping.Content[i+1]
		if mapping.Content[i].Value == "base_path" && value.Kind == goyaml.ScalarNode && !hasCustomTag(value.Tag) {
			return value.Value
		}
	}
	return ""
}

// recordScriptSource adds the internal script_source key to a decoded step mapping whose
// `script` came from a local !include or !include.raw file, and drops any script_source the
// user wrote: only the loader may set it.
func recordScriptSource(node *goyaml.Node, decoded map[string]interface{}, sourceFile string) {
	if sourceFile == "" {
		return
	}
	scriptNode := mappingValueNode(node, scriptKey)
	if scriptNode == nil {
		return
	}
	delete(decoded, scriptSourceKey)
	if scriptNode.Kind != goyaml.ScalarNode || !isIncludeTag(scriptNode.Tag) {
		return
	}
	resolved := commandIncludeScope(sourceFile).Resolve(scriptNode.Value)
	if resolved.LocalPath != "" && !resolved.HasQuery {
		decoded[scriptSourceKey] = resolved.LocalPath
	}
}

func mappingValueNode(node *goyaml.Node, key string) *goyaml.Node {
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}
