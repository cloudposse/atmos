package config

import (
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/template/delimiters"
)

// SelectStaticStackFiles preserves physical and possible logical matches from
// discovery. Only an explicit, static manifest name proves a file unrelated;
// templates, imports, and component overrides may determine any other name.
// The reader must return source text without evaluating it.
func SelectStaticStackFiles(config *schema.AtmosConfiguration, stack string, files []string, read func(string) (string, error)) ([]string, error) {
	defer perf.Track(config, "config.SelectStaticStackFiles")()

	if stack == "" {
		return files, nil
	}
	delims, err := delimiters.Parse(config.Templates.Settings.Delimiters)
	if err != nil {
		return nil, err
	}
	selected := make([]string, 0, len(files))
	for _, file := range files {
		if physicalStackName(config.StacksBaseAbsolutePath, file) == stack {
			selected = append(selected, file)
			continue
		}
		source, err := read(file)
		if err != nil {
			return nil, err
		}
		name, known := staticManifestName(source, delims)
		if !known || name == stack {
			selected = append(selected, file)
		}
	}
	return selected, nil
}

func physicalStackName(base, file string) string {
	if base == "" {
		base = filepath.Dir(file)
	}
	relative, err := filepath.Rel(base, file)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(strings.TrimSuffix(filepath.ToSlash(relative), ".yaml"), ".yml")
}

// staticManifestName reads only a plain explicit name. Unknown or malformed
// source remains a candidate so the normal static loader can diagnose it.
func staticManifestName(source string, delims delimiters.Delimiters) (string, bool) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source), &document); err != nil || len(document.Content) != 1 {
		return "", false
	}
	name := uniqueManifestNameNode(document.Content[0])
	if name == nil || name.Kind != yaml.ScalarNode || name.ShortTag() != "!!str" || name.Value == "" ||
		delims.ContainsAny(name.Value) || strings.HasPrefix(name.Value, "!") {
		return "", false
	}
	return name.Value, true
}

func uniqueManifestNameNode(root *yaml.Node) *yaml.Node {
	if root.Kind != yaml.MappingNode {
		return nil
	}
	var name *yaml.Node
	for i := 0; i+1 < len(root.Content); i += 2 {
		if root.Content[i].Value == "name" {
			if name != nil {
				return nil
			}
			name = root.Content[i+1]
		}
	}
	return name
}
