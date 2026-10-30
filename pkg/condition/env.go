package condition

import (
	"os"
	"strings"
)

// OSEnv returns the process environment as a map, for exposing environment variables to a `when`
// CEL expression as the `env` map (for example `when: !cel 'env.DEPLOY_ENV == "prod"'`).
//
//nolint:lintroller // This package cannot import perf because schema aliases condition.
func OSEnv() map[string]string {
	environ := os.Environ()
	env := make(map[string]string, len(environ))
	for _, kv := range environ {
		if i := strings.IndexByte(kv, '='); i >= 0 {
			env[kv[:i]] = kv[i+1:]
		}
	}
	return env
}
