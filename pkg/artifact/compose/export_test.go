package compose

import (
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/store"
)

// NewIdentityResolverForTest exposes the resolver with an injected auth bridge.
func NewIdentityResolverForTest(config *schema.AtmosConfiguration, build func(*schema.AtmosConfiguration) (store.AuthContextResolver, error)) artifact.IdentityResolver {
	return newIdentityResolver(config, build).resolve
}
