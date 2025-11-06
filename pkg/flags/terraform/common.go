package terraform

import (
	"github.com/cloudposse/atmos/pkg/flags"
)

// commonCompatibilityFlags returns flags shared across multiple terraform/opentofu commands.
// These include -var, -var-file, -no-color, -lock, -lock-timeout.
func commonCompatibilityFlags() map[string]flags.CompatibilityAlias {
	return map[string]flags.CompatibilityAlias{
		"-var":          {Behavior: flags.AppendToSeparated, Target: ""},
		"-var-file":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-no-color":     {Behavior: flags.AppendToSeparated, Target: ""},
		"-lock":         {Behavior: flags.AppendToSeparated, Target: ""},
		"-lock-timeout": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// defaultCompatibilityFlags returns flags for commands with minimal flag support.
// Most terraform commands support at least -no-color.
func defaultCompatibilityFlags() map[string]flags.CompatibilityAlias {
	return map[string]flags.CompatibilityAlias{
		"-no-color": {Behavior: flags.AppendToSeparated, Target: ""},
	}
}

// mergeMaps merges multiple compatibility alias maps into a single map.
// Later maps override earlier ones if keys conflict.
func mergeMaps(maps ...map[string]flags.CompatibilityAlias) map[string]flags.CompatibilityAlias {
	result := make(map[string]flags.CompatibilityAlias)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
