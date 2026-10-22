package config

import (
	"fmt"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/schema"
)

// importContextYAML retains function identity in sections decoded before imports.
// Viper otherwise discards native tags, making a deferred map indistinguishable
// from an invalid static string. Only this temporary view is rewritten.
func importContextYAML(content []byte) ([]byte, error) {
	var doc yaml.Node
	if err := yaml.Unmarshal(content, &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return content, nil
	}
	seen := make(map[*yaml.Node]bool)
	root := doc.Content[0]
	for i := 0; i+1 < len(root.Content); i += 2 {
		switch root.Content[i].Value {
		case "auth", "settings", "imports":
			preserveImportContextTags(root.Content[i+1], seen)
		}
	}
	return yaml.Marshal(&doc)
}

// preserveImportContextTags rewrites deferred values without following alias cycles.
func preserveImportContextTags(node *yaml.Node, seen map[*yaml.Node]bool) {
	if node == nil || seen[node] {
		return
	}
	seen[node] = true
	if node.Kind == yaml.ScalarNode && tag.IsAtmosConfigYAML(node.Tag) {
		node.Value = node.Tag + " " + node.Value
		node.Tag = "!!str"
	}
	for _, child := range node.Content {
		preserveImportContextTags(child, seen)
	}
	preserveImportContextTags(node.Alias, seen)
}

// decodeConfigForImports reads only the configuration needed to resolve imports.
// Other sections can still contain YAML functions returning maps or lists; decoding
// them into the full schema before preprocessing would prevent imports from loading.
// Keep authentication and downloader settings available without evaluating functions
// earlier than the normal configuration-loading phase.
func decodeConfigForImports(v *viper.Viper) (schema.AtmosConfiguration, error) {
	src := schema.AtmosConfiguration{BasePath: v.GetString("base_path")}
	// Remote import resolution can initialize the shared stack importer.
	src.Stacks.BasePath = v.GetString("stacks.base_path")
	var err error
	src.Import, err = NormalizeConfigImports(v.Get("import"))
	if err != nil {
		return src, err
	}
	hook := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(deferImportContextFunction, getAtmosDecodeHookFunc()))
	if err = v.UnmarshalKey("imports", &src.Imports, hook); err != nil {
		return src, fmt.Errorf("%w: decode import options: %w", errUtils.ErrMergeConfiguration, err)
	}
	if err = v.UnmarshalKey("settings", &src.Settings, hook); err != nil {
		return src, fmt.Errorf("%w: decode import settings: %w", errUtils.ErrMergeConfiguration, err)
	}
	if err = v.UnmarshalKey("auth", &src.Auth, hook); err != nil {
		return src, fmt.Errorf("%w: decode import auth: %w", errUtils.ErrMergeConfiguration, err)
	}
	return src, nil
}

// deferImportContextFunction leaves deferred containers out of the temporary
// resolver context. The source Viper remains intact: normal preprocessing later
// evaluates and validates these values without an extra evaluation pass here.
// Static siblings are retained.
func deferImportContextFunction(_ reflect.Type, to reflect.Type, data any) (any, error) {
	value, ok := data.(string)
	if !ok || to.Kind() == reflect.String || to.Kind() == reflect.Interface {
		return data, nil
	}
	for _, name := range tag.AtmosConfigYAML() {
		if tag.MatchesTag(value, name) {
			return reflect.Zero(to).Interface(), nil
		}
	}
	return data, nil
}
