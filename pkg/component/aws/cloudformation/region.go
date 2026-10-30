package cloudformation

import "github.com/cloudposse/atmos/pkg/component/aws/cloudformation/manifest"

// resolveRegion selects the inherited component region, retaining the deprecated alias.
// An empty return defers to the active identity and then the AWS SDK default chain.
func resolveRegion(componentSection map[string]any) string {
	return manifest.ResolveRegion(componentSection)
}
