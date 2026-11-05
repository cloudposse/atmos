package manifest

import (
	"github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
)

// RegionSection normalizes the region in one inheritance layer before merging layers.
// An explicitly authored region takes precedence over its legacy alias at the same scope.
func RegionSection(section map[string]any) map[string]any {
	defer perf.Track(nil, "cloudformation.manifest.RegionSection")()

	if region, ok := section[config.RegionSectionName]; ok {
		return map[string]any{config.RegionSectionName: region}
	}
	settings, _ := section[config.SettingsSectionName].(map[string]any)
	legacy, _ := settings["aws_cloudformation"].(map[string]any)
	if region, ok := legacy[config.RegionSectionName]; ok {
		return map[string]any{config.RegionSectionName: region}
	}
	return nil
}

// ResolveRegion reads the component's resolved region and warns about the legacy spelling.
// An empty region delegates selection to the active identity and AWS SDK default chain.
func ResolveRegion(section map[string]any) string {
	defer perf.Track(nil, "cloudformation.manifest.ResolveRegion")()

	settings, _ := section[config.SettingsSectionName].(map[string]any)
	legacy, _ := settings["aws_cloudformation"].(map[string]any)
	if _, ok := legacy[config.RegionSectionName]; ok {
		ui.Warning("settings.aws_cloudformation.region is deprecated; use region directly on the CloudFormation component or its defaults.")
	}
	region, _ := RegionSection(section)[config.RegionSectionName].(string)
	return region
}
