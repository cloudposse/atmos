package cloudformation

import "github.com/cloudposse/atmos/pkg/perf"

// resolveRegion returns the explicit region override, if any, from
// `settings.aws_cloudformation.region`. An empty return defers the rest of the
// precedence chain (active identity's region, then the SDK default chain) to
// buildAWSConfig/identity.LoadConfigWithAuth.
func resolveRegion(componentSection map[string]any) string {
	defer perf.Track(nil, "cloudformation.resolveRegion")()

	settings, ok := componentSection["settings"].(map[string]any)
	if !ok {
		return ""
	}
	cfnSettings, ok := settings["aws_cloudformation"].(map[string]any)
	if !ok {
		return ""
	}
	region, _ := cfnSettings["region"].(string)
	return region
}
