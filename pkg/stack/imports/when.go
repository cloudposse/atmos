package imports

import (
	"github.com/go-viper/mapstructure/v2"

	"github.com/cloudposse/atmos/pkg/ci"
	"github.com/cloudposse/atmos/pkg/condition"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// WhenContext returns the runtime facts available to a stack import's `when` condition: whether
// Atmos is running in a detected CI environment, and the process environment (exposed to CEL
// expressions as the `env` map). Stack, component, and command facts are not available because
// imports are resolved while the stack manifests are read, before any of them is known.
func WhenContext() condition.Context {
	defer perf.Track(nil, "imports.WhenContext")()

	return condition.Context{CI: ci.IsCI(), Env: condition.OSEnv()}
}

// IsActive reports whether the import should be processed. An import without a `when` condition is
// always active. A condition that cannot be evaluated does not match, the same as for the `import:`
// list in `atmos.yaml`. The usual cause is a CEL expression that reads an unset environment
// variable, such as `env.DEPLOY_ENV == "prod"`, and that should skip the import rather than fail.
// The reason is logged at debug level.
func IsActive(imp *schema.StackImport, ctx *condition.Context) bool {
	defer perf.Track(nil, "imports.IsActive")()

	if imp.When.IsZero() {
		return true
	}

	active, err := imp.When.EvaluateE(*ctx)
	if err != nil {
		log.Debug("Stack import condition could not be evaluated, so it does not match", "path", imp.Path, "error", err)
		return false
	}
	return active
}

// DecodeObject decodes one object-form entry of a stack manifest's `import` section, including its
// optional `when` condition (a predicate, a list, a map, or a `!cel` expression).
func DecodeObject(raw any) (schema.StackImport, error) {
	defer perf.Track(nil, "imports.DecodeObject")()

	var imp schema.StackImport
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		DecodeHook: schema.ConditionDecodeHook(),
		Result:     &imp,
	})
	if err != nil {
		return schema.StackImport{}, err
	}
	if err := decoder.Decode(raw); err != nil {
		return schema.StackImport{}, err
	}
	return imp, nil
}
