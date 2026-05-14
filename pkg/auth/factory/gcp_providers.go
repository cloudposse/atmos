package factory

import (
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/providers/gcp_adc"
	"github.com/cloudposse/atmos/pkg/auth/providers/gcp_wif"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegisterGCPProviders registers all GCP provider constructors with the factory.
func RegisterGCPProviders(f *Factory) {
	defer perf.Track(nil, "factory.RegisterGCPProviders")()

	// gcp/adc provider.
	f.RegisterProvider(types.ProviderKindGCPADC, func(name string, spec map[string]any) (types.Provider, error) {
		defer perf.Track(nil, "factory.CreateGCPADCProvider")()

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
	})

	// gcp/workload-identity-federation provider.
	f.RegisterProvider(types.ProviderKindGCPWorkloadIdentityFederation, func(name string, spec map[string]any) (types.Provider, error) {
		defer perf.Track(nil, "factory.CreateGCPWIFProvider")()

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
	})
}
