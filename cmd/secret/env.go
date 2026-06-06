package secret

import (
	"fmt"
	"os"

	envpkg "github.com/cloudposse/atmos/pkg/env"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets"
	"github.com/cloudposse/atmos/pkg/ui"
)

// secretEnvPair is a resolved secret rendered as an environment variable.
// The name is the declaration name verbatim (consistent with `secret pull`).
type secretEnvPair struct {
	name  string
	value string
}

// resolveSecretEnv resolves every declared secret for the service and returns a
// complete environment list (os.Environ() + global env from atmos.yaml + the
// resolved secrets) suitable for handing to a child process, along with the
// number of secrets injected.
//
// Missing or uninitialized secrets are skipped with a warning and do not abort
// the operation (same behavior as `secret pull`). Secret values are registered
// with the masker by svc.Get, so Atmos's own stderr stays masked — but values
// handed to the child process are NOT masked in that child's output.
func resolveSecretEnv(svc *secrets.Service, atmosConfig *schema.AtmosConfiguration) ([]string, int, error) {
	defer perf.Track(nil, "secret.resolveSecretEnv")()

	// Declarations() is sorted, so the injected order is deterministic.
	pairs := make([]secretEnvPair, 0)
	for _, decl := range svc.Declarations() {
		value, getErr := svc.Get(decl.Name, secrets.ResolveOptions{})
		if getErr != nil {
			ui.Warningf("Skipping `%s`: %v", decl.Name, getErr)
			continue
		}
		pairs = append(pairs, secretEnvPair{name: decl.Name, value: fmt.Sprintf("%v", value)})
	}

	base := envpkg.MergeGlobalEnv(os.Environ(), atmosConfig.Env)
	return applySecretEnv(base, pairs), len(pairs), nil
}

// applySecretEnv layers the resolved secrets onto the base environment,
// replacing any existing entry with the same name (so a declared secret takes
// precedence over an inherited variable of the same name) or appending it.
func applySecretEnv(base []string, pairs []secretEnvPair) []string {
	env := base
	for _, p := range pairs {
		env = envpkg.UpdateEnvVar(env, p.name, p.value)
	}
	return env
}
