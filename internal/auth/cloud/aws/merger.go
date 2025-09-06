package aws

import (
	"github.com/charmbracelet/log"
	m "github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// MergeIdentityEnvOverrides merges identity environment variables into the component environment section
func MergeIdentityEnvOverrides(info *schema.ConfigAndStacksInfo, envList []schema.EnvironmentVariable) {
	if info == nil {
		return
	}
	if info.ComponentEnvSection == nil {
		info.ComponentEnvSection = make(schema.AtmosSectionMapType)
	}

	// Convert EnvironmentVariable list to map[string]any for merging
	envMapAny := make(map[string]any)
	for _, env := range envList {
		envMapAny[env.Key] = env.Value
	}
	log.Debug("Merging environment variables", "envMapAny", envMapAny)

	// Merge with existing component environment section
	info.ComponentEnvSection, _ = m.Merge(&schema.AtmosConfiguration{}, []map[string]any{info.ComponentEnvSection, envMapAny})

	// Update ComponentEnvList with merged environment variables
	info.ComponentEnvList = u.ConvertEnvVars(info.ComponentEnvSection)
}
