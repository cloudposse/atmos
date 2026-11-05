package auth

import (
	"fmt"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func (m *manager) validateChainCredentialContracts() error {
	var outputs []string
	for _, name := range m.chain {
		var step any = m.identities[name]
		if provider, found := m.providers[name]; found {
			step = provider
		}
		metadata, supported := step.(types.CredentialContractProvider)
		if !supported {
			outputs = nil
			continue
		}
		contract := metadata.CredentialContract()
		for _, output := range outputs {
			if !slices.Contains(contract.Accepts, output) {
				return fmt.Errorf("%w: %s does not accept %s credentials", errUtils.ErrInvalidIdentityConfig, name, output)
			}
		}
		outputs = contract.Produces
	}
	return nil
}
