package exec

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/function/yamlsource"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// prepareConfigurationValues defers original Starlark sources until existing
// YAML functions have completed their typed merges. Remembering source paths
// prevents returned strings from being interpreted as new configuration code.
func prepareConfigurationValues(config *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, skip, sections []string) ([]string, func() error) {
	noop := func() error { return nil }
	if skipFunc(skip, starlarksource.Tag) {
		return skip, noop
	}
	selected, _ := deferred.SplitSectionsByRequirement(info.ComponentSection, sections)
	selected, _ = deferred.SplitEvaluationFields(selected, info.EvaluationPaths)
	if !containsStarlark(selected) {
		return skip, noop
	}
	sources := make(map[string]string)
	strict, policyErr := u.ExplicitYAMLTags(config)
	if policyErr != nil {
		return skip, func() error { return policyErr }
	}
	collectConfigurationSources(nil, selected, sources, skip, strict)
	if len(sources) == 0 {
		return skip, noop
	}
	return append(slices.Clone(skip), "starlark"), func() error {
		resolver := newConfigurationResolver(config, info.ComponentSection, withConfigurationContext(info.Stack, info), withConfigurationFunctions(skip, nil))
		resolver.sources = sources
		resolved, err := resolver.resolveMap(nil, info.ComponentSection)
		if err != nil {
			return err
		}
		info.ComponentSection = resolved
		return nil
	}
}

// collectConfigurationSources records each encoded Starlark source by its path. List indexes
// are computed as they will appear after !unset items are dropped, because the resolver runs
// against the compacted list.
func collectConfigurationSources(path []string, value any, sources map[string]string, skip []string, strict ...bool) {
	switch value := value.(type) {
	case string:
		if starlarksource.IsEncoded(value) {
			sources[configurationPathKey(path)] = value
		}
	case map[string]any:
		for key, child := range value {
			collectConfigurationSources(append(slices.Clone(path), key), child, sources, skip, strict...)
		}
	case []any:
		index := 0
		for _, child := range value {
			if configurationListItemUnset(child, skip, strict...) {
				continue
			}
			collectConfigurationSources(append(slices.Clone(path), strconv.Itoa(index)), child, sources, skip, strict...)
			index++
		}
	}
}

// configurationListItemUnset matches the edition-aware list compaction performed
// before Starlark evaluation, keeping recorded source indexes aligned.
func configurationListItemUnset(value any, skip []string, strict ...bool) bool {
	if len(strict) == 0 || !strict[0] {
		return isUnsetListItem(value, skip)
	}
	if marker, ok := value.(UnsetMarker); ok {
		return marker.IsUnset
	}
	text, _ := value.(string)
	source, tagged := yamlsource.Decode(text)
	return tagged && source.Tag == "!unset" && !skipFunc(skip, "!unset")
}

func configurationPathKey(path []string) string {
	encoded, _ := json.Marshal(path)
	return string(encoded)
}
