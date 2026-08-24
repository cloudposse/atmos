package factory

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/identities/gcp_project"
	"github.com/cloudposse/atmos/pkg/auth/identities/gcp_service_account"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegisterGCPIdentities registers all GCP identity constructors with the factory.
func RegisterGCPIdentities(f *Factory) {
	defer perf.Track(nil, "factory.RegisterGCPIdentities")()

	// gcp/service-account identity.
	f.RegisterIdentity(types.IdentityKindGCPServiceAccount, func(name string, principal map[string]any) (types.Identity, error) {
		defer perf.Track(nil, "factory.CreateGCPServiceAccountIdentity")()

		parsed, err := types.ParseGCPServiceAccountIdentityPrincipal(principal)
		if err != nil {
			return nil, fmt.Errorf("%w: parse gcp/service-account principal: %v", errUtils.ErrInvalidIdentityConfig, err)
		}
		identity, err := gcp_service_account.New(parsed)
		if err != nil {
			return nil, err
		}
		identity.SetName(name)
		return identity, nil
	})

	// gcp/project identity.
	f.RegisterIdentity(types.IdentityKindGCPProject, func(name string, principal map[string]any) (types.Identity, error) {
		defer perf.Track(nil, "factory.CreateGCPProjectIdentity")()

		parsed, err := types.ParseGCPProjectIdentityPrincipal(principal)
		if err != nil {
			return nil, fmt.Errorf("%w: parse gcp/project principal: %v", errUtils.ErrInvalidIdentityConfig, err)
		}
		identity, err := gcp_project.New(parsed)
		if err != nil {
			return nil, err
		}
		identity.SetName(name)
		return identity, nil
	})
}
