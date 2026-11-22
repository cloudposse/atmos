package auth

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// identityRequest is the identity a user asked an auth subcommand to use.
type identityRequest struct {
	// Flag is the value of an explicit --identity flag on the command line
	// (IdentityFlagSelectValue when given without a value), or "" when the
	// flag was not used.
	flag string
	// Env is the identity from ATMOS_IDENTITY (via Viper), or "" when unset.
	// `atmos auth shell` exports ATMOS_IDENTITY, so it is ambient context
	// rather than an explicit choice made for this command.
	env string
}

// resolveIdentityRequest reads the explicit --identity flag and the
// ATMOS_IDENTITY fallback separately, so callers can let explicit selectors
// such as --tags take precedence over the ambient environment variable.
func resolveIdentityRequest(cmd *cobra.Command, v *viper.Viper) identityRequest {
	req := identityRequest{flag: GetIdentityFromFlags(cmd)}
	if req.flag == "" {
		req.env = v.GetString(IdentityFlagName)
	}
	return req
}

// name returns the effective identity: the explicit flag, else ATMOS_IDENTITY.
func (r identityRequest) name() string {
	if r.flag != "" {
		return r.flag
	}
	return r.env
}
