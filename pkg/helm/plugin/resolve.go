package plugin

import (
	"context"
	"fmt"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

// EnsureForComponent parses the raw plugin declarations and installs any that
// are missing, returning the managed HELM_PLUGINS directory.
//
// When no plugins are declared it returns an empty string so callers leave
// HELM_PLUGINS untouched, preserving the user's default Helm plugin behavior.
func EnsureForComponent(ctx context.Context, helmBin string, rawSpecs []string) (string, error) {
	defer perf.Track(nil, "plugin.EnsureForComponent")()

	if len(rawSpecs) == 0 {
		return "", nil
	}

	specs, err := ParseSpecs(rawSpecs)
	if err != nil {
		return "", err
	}

	return NewInstaller(helmBin).EnsurePlugins(ctx, specs)
}

// ExtractSpecs reads the `plugins` section from a component configuration map and
// returns the declared plugin specs as strings. Non-string entries are coerced
// with fmt so templated values render predictably. Returns nil when the section
// is absent or empty.
func ExtractSpecs(section map[string]any) []string {
	defer perf.Track(nil, "plugin.ExtractSpecs")()

	raw, ok := section[cfg.PluginsSectionName]
	if !ok {
		return nil
	}

	switch v := raw.(type) {
	case []string:
		return append([]string(nil), v...)
	case []any:
		specs := make([]string, 0, len(v))
		for _, item := range v {
			if s := stringify(item); s != "" {
				specs = append(specs, s)
			}
		}
		return specs
	default:
		return nil
	}
}

// stringify converts a plugin list element to its string form.
func stringify(item any) string {
	switch s := item.(type) {
	case string:
		return s
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", s)
	}
}
