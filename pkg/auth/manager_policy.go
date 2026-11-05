package auth

import (
	"context"

	"github.com/cloudposse/atmos/pkg/policy/guard"
)

// enforceAuthPolicy resolves the whole chain before any credentials are used.
// A development target may still traverse a production identity or provider.
func (m *manager) enforceAuthPolicy(ctx context.Context, identity string, chain []string) error {
	enabled, err := guard.Enabled("auth")
	if err != nil || !enabled {
		return err
	}
	if chain == nil {
		chain, err = m.buildAuthenticationChain(identity)
		if err != nil {
			return err
		}
	}
	input := guard.AuthInput{Identity: identity, Identities: []string{}}
	for index, name := range chain {
		if _, provider := m.providers[name]; index == 0 && provider {
			input.Provider = name
		} else {
			input.Identities = append(input.Identities, name)
		}
	}
	return guard.Authenticate(ctx, input)
}
