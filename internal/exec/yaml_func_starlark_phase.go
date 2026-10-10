package exec

import (
	"encoding/json"
	"slices"
	"strconv"

	"github.com/cloudposse/atmos/pkg/deferred"
	"github.com/cloudposse/atmos/pkg/function/starlarksource"
	"github.com/cloudposse/atmos/pkg/schema"
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
	collectConfigurationSources(nil, selected, sources)
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

func collectConfigurationSources(path []string, value any, sources map[string]string) {
	switch value := value.(type) {
	case string:
		if starlarksource.Is(value) {
			sources[configurationPathKey(path)] = value
		}
	case map[string]any:
		for key, child := range value {
			collectConfigurationSources(append(slices.Clone(path), key), child, sources)
		}
	case []any:
		for index, child := range value {
			collectConfigurationSources(append(slices.Clone(path), strconv.Itoa(index)), child, sources)
		}
	}
}

func configurationPathKey(path []string) string {
	encoded, _ := json.Marshal(path)
	return string(encoded)
}
