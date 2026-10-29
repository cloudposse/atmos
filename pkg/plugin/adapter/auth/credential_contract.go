package auth

import (
	"slices"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

// CredentialContract exposes the provider's statically validated output types.
//
//nolint:lintroller // Static metadata accessor without I/O.
func (p *provider) CredentialContract() types.CredentialContract {
	return types.CredentialContract{Produces: slices.Clone(p.definition.Outputs)}
}

// CredentialContract exposes the identity's accepted and produced envelope types.
//
//nolint:lintroller // Static metadata accessor without I/O.
func (i *identity) CredentialContract() types.CredentialContract {
	return types.CredentialContract{Accepts: slices.Clone(i.accepts), Produces: slices.Clone(i.definition.Outputs)}
}
