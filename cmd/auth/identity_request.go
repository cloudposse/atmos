package auth

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
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

// disabled reports whether the effective identity disables Atmos
// authentication (`--identity=false` or `ATMOS_IDENTITY=false`).
func (r identityRequest) disabled() bool {
	return isIdentityDisabled(r.name())
}

// isIdentityDisabled reports whether identity is a value that disables Atmos
// authentication: false, 0, no, off (case-insensitive), or the disabled sentinel.
func isIdentityDisabled(identity string) bool {
	return identity != "" && cfg.NormalizeIdentityValue(identity) == cfg.IdentityFlagDisabledValue
}

// checkIdentityNotDisabled rejects `--identity=false` / `ATMOS_IDENTITY=false`
// for auth subcommands that exist to act on an identity (login, logout, whoami,
// env, shell, console). Without it the value is looked up as an identity named
// "false" and fails with a misleading "identity not found" error.
func checkIdentityNotDisabled(command, identity string) error {
	if !isIdentityDisabled(identity) {
		return nil
	}
	return errUtils.Build(errUtils.ErrAuthDisabledIdentityRequired).
		WithExplanationf("`atmos auth %s` acts on an identity, but the identity %q disables Atmos authentication (from `--identity` or `ATMOS_IDENTITY`).", command, identity).
		WithHint("Pass `--identity <name>` to choose an identity, or unset `ATMOS_IDENTITY`.").
		WithHint("Run `atmos auth list` to see the available identities.").
		Err()
}
