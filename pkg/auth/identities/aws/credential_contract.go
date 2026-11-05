package aws

import "github.com/cloudposse/atmos/pkg/auth/types"

// CredentialContract validates native plugin-to-AWS chains before authentication.
func (*assumeRoleIdentity) CredentialContract() types.CredentialContract {
	return types.CredentialContract{Accepts: []string{"aws/v1"}, Produces: []string{"aws/v1"}}
}
