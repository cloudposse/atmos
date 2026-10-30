package config

import (
	"fmt"
	"path/filepath"
	"reflect"

	"github.com/go-viper/mapstructure/v2"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/tag"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
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
		case AuthSectionName, "settings", "imports":
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
		node.Value = (yamlsource.Source{Tag: node.Tag, Argument: node.Value, Line: node.Line, Column: node.Column}).Encode()
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
	src := schema.AtmosConfiguration{BasePath: v.GetString("base_path"), Edition: resolveEditionPin(v)}
	// Remote import resolution can initialize the shared stack importer.
	src.Stacks.BasePath = v.GetString("stacks.base_path")
	var err error
	src.Import, err = NormalizeConfigImports(v.Get("import"), &src)
	if err != nil {
		return src, err
	}
	strict, err := u.ExplicitYAMLTags(&src)
	if err != nil {
		return src, err
	}
	hook := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(deferImportContextFunction(strict), getAtmosDecodeHookFunc(strict)))
	if err = v.UnmarshalKey("imports", &src.Imports, hook); err != nil {
		return src, fmt.Errorf("%w: decode import options: %w", errUtils.ErrMergeConfiguration, err)
	}
	if err = v.UnmarshalKey("settings", &src.Settings, hook); err != nil {
		return src, fmt.Errorf("%w: decode import settings: %w", errUtils.ErrMergeConfiguration, err)
	}
	if err = v.UnmarshalKey(AuthSectionName, &src.Auth, hook); err != nil {
		return src, fmt.Errorf("%w: decode import auth: %w", errUtils.ErrMergeConfiguration, err)
	}
	return src, nil
}

// deferImportContextFunction leaves deferred containers out of the temporary
// resolver context. The source Viper remains intact: normal preprocessing later
// evaluates and validates these values without an extra evaluation pass here.
// Static siblings are retained.
func deferImportContextFunction(strict bool) mapstructure.DecodeHookFunc {
	return func(_ reflect.Type, to reflect.Type, data any) (any, error) {
		value, ok := data.(string)
		if !ok || to.Kind() == reflect.String || to.Kind() == reflect.Interface {
			return data, nil
		}
		if source, native := yamlsource.Decode(value); native && tag.IsAtmosConfigYAML(source.Tag) {
			return reflect.Zero(to).Interface(), nil
		}
		if !strict {
			for _, name := range tag.AtmosConfigYAML() {
				if tag.MatchesTag(value, name) {
					return reflect.Zero(to).Interface(), nil
				}
			}
		}
		return data, nil
	}
}

// mergeDeferredProvisionedIdentities loads cached identities for providers whose names only
// become available during normal YAML evaluation. Existing manual/imported auth wins over
// the cache, and the provider expressions are never evaluated a second time.
func mergeDeferredProvisionedIdentities(v *viper.Viper, previousProviders map[string]any, configFile string) error {
	providers := v.GetStringMap("auth.providers")
	hasNewProvider := false
	for name := range providers {
		if _, existed := previousProviders[name]; !existed {
			hasNewProvider = true
			break
		}
	}
	if !hasNewProvider {
		return nil
	}
	src, err := decodeConfigForImports(v)
	if err != nil {
		return err
	}
	for name := range previousProviders {
		delete(src.Auth.Providers, name)
	}
	src.Import = nil
	// Cache imports are absolute; use the source directory as the resolver context.
	src.BasePath = filepath.Dir(configFile)
	if err := injectProvisionedIdentityImports(&src); err != nil {
		return fmt.Errorf("%w: discover deferred provider identities: %w", errUtils.ErrMergeConfiguration, err)
	}
	if len(src.Import) == 0 {
		return nil
	}
	imported := viper.New()
	imported.SetConfigType("yaml")
	defer mergedFilesReg.attachLowerPriority(v, imported)()
	if err := processConfigImports(&src, imported); err != nil {
		return err
	}
	// AllSettings includes static siblings that a Viper override at auth.providers
	// would hide from Get(AuthSectionName). Preserve them when layering the cache underneath.
	if err := imported.MergeConfigMap(map[string]any{AuthSectionName: v.AllSettings()[AuthSectionName]}); err != nil {
		return err
	}
	v.Set(AuthSectionName, imported.Get(AuthSectionName))
	return nil
}
