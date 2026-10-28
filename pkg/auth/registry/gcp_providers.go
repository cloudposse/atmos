package registry

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/providers/gcp_adc"
	"github.com/cloudposse/atmos/pkg/auth/providers/gcp_wif"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegisterGCPProviders registers all GCP provider constructors with the registry.
func RegisterGCPProviders(f *Registry) error {
	defer perf.Track(nil, "registry.RegisterGCPProviders")()

	// gcp/adc provider.
	if err := f.RegisterProvider(types.ProviderKindGCPADC, func(name string, spec map[string]any) (types.Provider, error) {
		defer perf.Track(nil, "registry.CreateGCPADCProvider")()

		parsed, err := types.ParseGCPADCProviderSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("parse gcp/adc spec: %w", errors.Join(errUtils.ErrInvalidProviderConfig, err))
		}
		provider, err := gcp_adc.New(parsed)
		if err != nil {
			return nil, err
		}
		provider.SetName(name)
		return provider, nil
	}); err != nil {
		return err
	}

	// gcp/workload-identity-federation provider.
	if err := f.RegisterProvider(types.ProviderKindGCPWorkloadIdentityFederation, func(name string, spec map[string]any) (types.Provider, error) {
		defer perf.Track(nil, "registry.CreateGCPWIFProvider")()

		parsed, err := types.ParseGCPWorkloadIdentityFederationProviderSpec(spec)
		if err != nil {
			return nil, fmt.Errorf("parse gcp/wif spec: %w", errors.Join(errUtils.ErrInvalidProviderConfig, err))
		}
		provider, err := gcp_wif.New(parsed)
		if err != nil {
			return nil, err
		}
		provider.SetName(name)
		return provider, nil
	}); err != nil {
		return err
	}
	return nil
}
