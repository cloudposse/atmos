package atmos

import (
	"context"
	"fmt"

	"github.com/cloudposse/atmos/pkg/ai/tools"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/manager"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// VersionTrackAddTool adds a dependency entry to a version track in atmos.yaml.
type VersionTrackAddTool struct {
	atmosConfig *schema.AtmosConfiguration
}

// NewVersionTrackAddTool creates a new version track add tool.
func NewVersionTrackAddTool(atmosConfig *schema.AtmosConfiguration) *VersionTrackAddTool {
	return &VersionTrackAddTool{
		atmosConfig: atmosConfig,
	}
}

// Name returns the tool name.
func (t *VersionTrackAddTool) Name() string {
	return "atmos_version_track_add"
}

// Description returns the tool description.
func (t *VersionTrackAddTool) Description() string {
	return "Add a dependency entry to an Atmos version track (declarative, ecosystem-agnostic pinned-version " +
		"management in atmos.yaml -- GitHub tags/releases, OCI tags, Docker images, toolchain tools, and more). " +
		"This is a separate subsystem from `atmos toolchain` (installed CLI tool binaries) and `atmos vendor` " +
		"(vendored component sources); each manages versions within its own domain. Requires user confirmation."
}

// Parameters returns the tool parameters.
func (t *VersionTrackAddTool) Parameters() []tools.Parameter {
	params := []tools.Parameter{
		{
			Name:        paramName,
			Description: "Dependency name/key within the track (e.g. 'terraform-aws-modules/vpc').",
			Type:        tools.ParamTypeString,
			Required:    true,
		},
		{
			Name:        paramTrack,
			Description: "Version track to add to. Omit to use the project's default track.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramPackage,
			Description: "Upstream package identifier resolved by the datasource. Defaults to name.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name: paramEcosystem,
			Description: "Datasource ecosystem (e.g. 'github', 'github/actions', 'oci', 'toolchain'). " +
				"Defaults to an ecosystem inferred from package.",
			Type:     tools.ParamTypeString,
			Required: false,
		},
		{
			Name:        paramDatasource,
			Description: "Explicit datasource name, overriding the ecosystem default.",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramProvider,
			Description: "Provider used to resolve the datasource (e.g. a specific registry host).",
			Type:        tools.ParamTypeString,
			Required:    false,
		},
		{
			Name:        paramDesired,
			Description: "Desired version expression: a concrete version, a SemVer constraint, or 'latest'.",
			Type:        tools.ParamTypeString,
			Required:    false,
			Default:     defaultDesiredVersion,
		},
	}
	return append(params, versionUpdatePolicyParams(
		"Optional group name for applying shared update policy across several entries.",
		"Update pin mode (e.g. 'digest' to re-pin a digest even when the version is unchanged).",
		"Version patterns to include when resolving updates.",
		"Version patterns to exclude when resolving updates.",
		"Allow prerelease versions when resolving updates.",
	)...)
}

// Execute adds the dependency entry to the track.
func (t *VersionTrackAddTool) Execute(_ context.Context, params map[string]interface{}) (*tools.Result, error) {
	name, err := extractRequiredStringParam(params, paramName)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}
	track, _ := params[paramTrack].(string)
	entry := buildVersionEntry(params, name)

	file, err := manager.AddEntry(t.atmosConfig, track, name, entry)
	if err != nil {
		return &tools.Result{Success: false, Error: err}, err
	}

	effectiveTrack := manager.EffectiveTrack(t.atmosConfig, track)
	output := fmt.Sprintf("Added dependency %s to track %s in %s", name, effectiveTrack, atmosyaml.DisplayPath(file))
	return &tools.Result{
		Success: true,
		Output:  output,
		Data: map[string]interface{}{
			paramName:  name,
			paramTrack: effectiveTrack,
			"file":     atmosyaml.DisplayPath(file),
		},
	}, nil
}

// RequiresPermission returns true if this tool needs permission.
func (t *VersionTrackAddTool) RequiresPermission() bool {
	return true // Writing atmos.yaml requires confirmation.
}

// IsRestricted returns true if this tool is always restricted.
func (t *VersionTrackAddTool) IsRestricted() bool {
	return false // User can allow via configuration.
}
