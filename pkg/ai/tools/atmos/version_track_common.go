package atmos

import (
	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
)

// Parameter names shared by the atmos_version_track_* tools.
const (
	paramName       = "name"
	paramTrack      = "track"
	paramPackage    = "package"
	paramEcosystem  = "ecosystem"
	paramDatasource = "datasource"
	paramProvider   = "provider"
	paramDesired    = "desired"
	paramGroup      = "group"
	paramPin        = "pin"
	paramInclude    = "include"
	paramExclude    = "exclude"
	paramPrerelease = "prerelease"
)

// defaultDesiredVersion mirrors `atmos version track add`'s own default for
// the --desired flag.
const defaultDesiredVersion = "latest"

// versionUpdatePolicyParams returns the group/pin/include/exclude/prerelease
// parameter definitions shared by atmos_version_track_add and
// atmos_version_track_set, keeping each tool's own Parameters() short.
func versionUpdatePolicyParams(groupDescription, pinDescription, includeDescription, excludeDescription, prereleaseDescription string) []tools.Parameter {
	return []tools.Parameter{
		{
			Name:        paramGroup,
			Description: groupDescription,
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramPin,
			Description: pinDescription,
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramInclude,
			Description: includeDescription,
			Type:        tools.ParamTypeArray,
			Required:    false,
		},
		{
			Name:        paramExclude,
			Description: excludeDescription,
			Type:        tools.ParamTypeArray,
			Required:    false,
		},
		{
			Name:        paramPrerelease,
			Description: prereleaseDescription,
			Type:        tools.ParamTypeBool,
			Required:    false,
		},
	}
}

// buildVersionEntry builds a schema.VersionEntry from atmos_version_track_add's
// parameters, mirroring `cmd/version/track/add.go`'s flag-to-field mapping.
func buildVersionEntry(params map[string]interface{}, name string) *schema.VersionEntry {
	pkg, _ := params[paramPackage].(string)
	if pkg == "" {
		pkg = name
	}
	ecosystem, _ := params[paramEcosystem].(string)
	if ecosystem == "" {
		ecosystem = manager.InferEcosystem(pkg)
	}
	desired, _ := params[paramDesired].(string)
	if desired == "" {
		desired = defaultDesiredVersion
	}
	datasource, _ := params[paramDatasource].(string)
	provider, _ := params[paramProvider].(string)
	group, _ := params[paramGroup].(string)

	entry := &schema.VersionEntry{
		Ecosystem:  ecosystem,
		Datasource: datasource,
		Provider:   provider,
		Package:    pkg,
		Desired:    desired,
		Group:      group,
		Include:    extractStringSliceParam(params, paramInclude),
		Exclude:    extractStringSliceParam(params, paramExclude),
	}
	if pin, _ := params[paramPin].(string); pin != "" {
		entry.Update = schema.VersionUpdatePolicy{Pin: pin}
	}
	if prerelease, ok := params[paramPrerelease].(bool); ok {
		entry.Prerelease = &prerelease
	}
	return entry
}

// buildVersionEntryFieldUpdates builds the fields map for
// manager.SetEntryFields from atmos_version_track_set's parameters, mirroring
// `cmd/version/track/set.go`'s flag-to-field mapping. Only parameters that
// were actually supplied are included, matching how the CLI only touches a
// field whose flag was explicitly set -- an omitted parameter leaves the
// existing entry field untouched. Note set.go does not support ecosystem or
// datasource; only add.go does.
func buildVersionEntryFieldUpdates(params map[string]interface{}) map[string]any {
	fields := make(map[string]any)
	for _, name := range []string{paramDesired, paramPackage, paramProvider, paramGroup} {
		if v, ok := params[name].(string); ok && v != "" {
			fields[name] = v
		}
	}
	if pin, ok := params[paramPin].(string); ok && pin != "" {
		fields["update.pin"] = pin
	}
	if _, ok := params[paramInclude]; ok {
		fields[paramInclude] = extractStringSliceParam(params, paramInclude)
	}
	if _, ok := params[paramExclude]; ok {
		fields[paramExclude] = extractStringSliceParam(params, paramExclude)
	}
	if prerelease, ok := params[paramPrerelease].(bool); ok {
		fields[paramPrerelease] = prerelease
	}
	return fields
}
