// Package artifactwire assembles the production artifact service that the CLI and the artifact
// workflow step share.
//
// It lives outside pkg/artifact because the assembly needs the plugin-aware registry preparation
// and the managed Cosign signer, and the artifact packages are forbidden from depending on the
// plugin subsystem. The artifact service itself, the request and result mapping, and every
// repository stay free of that dependency.
package artifactwire

import (
	"context"

	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/artifact/compose"
	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/bootstrap"
	pluginsign "github.com/cloudposse/atmos/pkg/plugin/sign"
	"github.com/cloudposse/atmos/pkg/schema"
)

// NewService wires the repositories, the shared verifier, the managed Cosign signer, and the
// plugin-aware registry authentication preparation into one artifact service. It is an
// [invoke.ServiceFactory]. Building the service performs no network access, credential
// resolution, or directory creation.
func NewService(config *schema.AtmosConfiguration, options invoke.ServiceOptions) (artifact.Service, error) {
	defer perf.Track(config, "artifactwire.NewService")()

	composition, err := compose.New(
		config,
		compose.WithBlobSigner(pluginsign.Cosign{Config: config, Yes: options.AcceptTransparencyLog}),
		compose.WithRegistryPreparation(func(ctx context.Context) (func(), error) {
			return bootstrap.PrepareAvailable(ctx, config)
		}),
	)
	if err != nil {
		return nil, err
	}
	return composition.Service, nil
}

var _ invoke.ServiceFactory = NewService
