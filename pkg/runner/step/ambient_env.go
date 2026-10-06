package step

import (
	"maps"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ApplyAmbientEnv sets the step's complete process environment: the ambient entries (the
// inherited process environment and any values already rendered by the caller) overlaid with the
// step's declared env. Only the declared entries are template-rendered by the step handlers. The
// ambient entries are marked literal so they reach child processes exactly as they are, and a
// stray "{{" in an inherited variable can neither fail the step nor be evaluated as a template.
//
// The declared map is the step's own `env:` as written, before rendering. The step's existing
// literal markers for its declared entries are kept.
func ApplyAmbientEnv(step *schema.WorkflowStep, ambient, declared map[string]string) {
	defer perf.Track(nil, "step.ApplyAmbientEnv")()

	merged := make(map[string]string, len(ambient)+len(declared))
	maps.Copy(merged, ambient)
	maps.Copy(merged, declared)
	step.Env = merged

	literal := make([]string, 0, len(step.LiteralFields)+len(ambient))
	literal = append(literal, step.LiteralFields...)
	for key := range ambient {
		if hasEnvKey(declared, key) {
			continue
		}
		literal = append(literal, schema.LiteralFieldEnvPrefix+key)
	}
	step.LiteralFields = literal
}

// hasEnvKey reports whether env declares key. Keys match case-insensitively because
// configuration loading can change the case of map keys.
func hasEnvKey(env map[string]string, key string) bool {
	if _, ok := env[key]; ok {
		return true
	}
	for candidate := range env {
		if strings.EqualFold(candidate, key) {
			return true
		}
	}
	return false
}
