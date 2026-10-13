package exec

import "github.com/cloudposse/atmos/pkg/schema"

const (
	inspectionReferencePrefix = "inspect:"
	componentReferenceKind    = "atmos.Component"
	stateReferenceKind        = "terraform.state"
	staticStateReferenceKind  = "terraform.state.static"
)

// terraformReferenceCacheKey isolates global values between configuration roots and
// inspection modes. Separate target fields prevent ambiguous stack/component slugs.
type terraformReferenceCacheKey struct {
	basePath   string
	configPath string
	stack      string
	component  string
	maskOnly   bool
}

func newTerraformReferenceCacheKey(ac *schema.AtmosConfiguration, stack, component string, maskOnly bool) terraformReferenceCacheKey {
	return terraformReferenceCacheKey{basePath: ac.BasePath, configPath: ac.CliConfigPath, stack: stack, component: component, maskOnly: maskOnly}
}

// Deferred value caches already isolate configuration and effective authentication.
// Their value kind must additionally distinguish inspection from execution.
func terraformReferenceValueKind(kind string, maskOnly bool) string {
	if maskOnly {
		return inspectionReferencePrefix + kind
	}
	return kind
}
