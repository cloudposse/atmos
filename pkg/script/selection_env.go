package script

import (
	"strings"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// The environment bindings of the global --profile and --identity flags. Child Atmos processes
	// read their selection from them.
	profileEnvVar  = "ATMOS_PROFILE"
	identityEnvVar = "ATMOS_IDENTITY"
	// The value that tells a child process authentication is turned off.
	identityDisabledValue = "false"
)

// SelectionEnv returns the environment entries that make a child Atmos process use the same
// profiles and identity as the running one. A global flag written on the command line, such as
// `--profile=dev`, exists only in the parent's memory; without these entries a nested `atmos`
// call would silently run with the default profile. Empty selections add no entry, and the bare
// `--identity` interactive-selection sentinel is never forwarded because a child cannot answer it.
func SelectionEnv(profiles []string, identity string) map[string]string {
	defer perf.Track(nil, "script.SelectionEnv")()

	values := map[string]string{}
	var names []string
	for _, profile := range profiles {
		if profile = strings.TrimSpace(profile); profile != "" && profile != cfg.ProfileFlagSelectValue {
			names = append(names, profile)
		}
	}
	if len(names) > 0 {
		values[profileEnvVar] = strings.Join(names, ",")
	}
	switch identity = strings.TrimSpace(identity); identity {
	case "", cfg.IdentityFlagSelectValue:
	case cfg.IdentityFlagDisabledValue:
		values[identityEnvVar] = identityDisabledValue
	default:
		values[identityEnvVar] = identity
	}
	return values
}
