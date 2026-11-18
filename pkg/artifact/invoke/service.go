package invoke

import (
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ServiceOptions are the per-invocation settings that reach the composition root.
type ServiceOptions struct {
	// AcceptTransparencyLog accepts Sigstore transparency log publication without prompting.
	AcceptTransparencyLog bool
}

// ServiceFactory builds the artifact service for the loaded configuration. Production composes the
// real repositories lazily, so building the service performs no network access, credential
// resolution, or directory creation. Tests inject a factory to run an entry point against a fake.
type ServiceFactory func(config *schema.AtmosConfiguration, options ServiceOptions) (artifact.Service, error)
