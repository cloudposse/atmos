package registry

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/identities/gcp_project"
	"github.com/cloudposse/atmos/pkg/auth/identities/gcp_service_account"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegisterGCPIdentities registers all GCP identity constructors with the registry.
func RegisterGCPIdentities(f *Registry) error {
	defer perf.Track(nil, "registry.RegisterGCPIdentities")()

	// gcp/service-account identity.
	if err := f.RegisterIdentity(types.IdentityKindGCPServiceAccount, func(name string, principal map[string]any) (types.Identity, error) {
		defer perf.Track(nil, "registry.CreateGCPServiceAccountIdentity")()

		parsed, err := types.ParseGCPServiceAccountIdentityPrincipal(principal)
		if err != nil {
			return nil, fmt.Errorf("%w: parse gcp/service-account principal: %w", errUtils.ErrInvalidIdentityConfig, err)
		}
		identity, err := gcp_service_account.New(parsed)
		if err != nil {
			return nil, err
		}
		identity.SetName(name)
		return identity, nil
	}); err != nil {
		return err
	}

	// gcp/project identity.
	if err := f.RegisterIdentity(types.IdentityKindGCPProject, func(name string, principal map[string]any) (types.Identity, error) {
		defer perf.Track(nil, "registry.CreateGCPProjectIdentity")()

		parsed, err := types.ParseGCPProjectIdentityPrincipal(principal)
		if err != nil {
			return nil, fmt.Errorf("%w: parse gcp/project principal: %w", errUtils.ErrInvalidIdentityConfig, err)
		}
		identity, err := gcp_project.New(parsed)
		if err != nil {
			return nil, err
		}
		identity.SetName(name)
		return identity, nil
	}); err != nil {
		return err
	}
	return nil
}
