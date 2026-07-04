package manager

// Blank imports register the built-in datasource resolvers with the resolver
// registry so any consumer of the manager package gets them.
import (
	_ "github.com/cloudposse/atmos/pkg/version/resolver/github"
	_ "github.com/cloudposse/atmos/pkg/version/resolver/toolchain"
)
