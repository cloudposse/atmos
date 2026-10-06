package hooks

import (
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// stepsKey is the key under which a group step (parallel, test) holds its child steps.
const stepsKey = "steps"

// applyScriptSources maps the in-band `script_source` key Atmos records next to an included
// `script` (see utils.ScriptSourceKey) onto WorkflowStep.ScriptSource for ws and, recursively,
// for its child steps. WorkflowStep.ScriptSource has no YAML key, so the YAML round trip that
// builds the step from the hook payload drops it and it has to be restored explicitly. The
// in-band `literal_fields` key (see schema.LiteralFieldsKey) is restored onto
// WorkflowStep.LiteralFields the same way.
//
// The raw argument is the step payload as it came from the merged stack configuration, before hook
// template rendering. The recorded provenance is honored only while the raw `script` still hashes to the
// recorded utils.ScriptSourceSHA256Key: stack inheritance deep-merges maps, so a child stack that
// overrides just `script` would otherwise inherit its base stack's script_source. Comparing
// against the raw value (not the rendered one) keeps templated scripts matching.
//
// The recorded path is relative to the project base path when the file is inside it; the step
// gets the absolute path so load() and tracebacks resolve against the included file.
func applyScriptSources(atmosConfig *schema.AtmosConfiguration, ws *schema.WorkflowStep, raw any) {
	m, ok := stringKeyedMap(raw)
	if !ok {
		return
	}
	if recorded := utils.ScriptSourceMatches(m); recorded != "" {
		ws.ScriptSource = utils.ResolveRecordedScriptSource(atmosConfig, recorded)
	} else if stale, isString := m[utils.ScriptSourceKey].(string); isString && stale != "" {
		log.Debug("Ignoring stale script_source; the step script no longer matches the included file",
			"script_source", stale)
	}
	ws.LiteralFields = schema.LiteralFieldsFromValue(m[schema.LiteralFieldsKey])
	children, ok := m[stepsKey].([]any)
	if !ok {
		return
	}
	for i := range ws.Steps {
		if i >= len(children) {
			return
		}
		applyScriptSources(atmosConfig, &ws.Steps[i], children[i])
	}
}

// stringKeyedMap returns v as a map[string]any. A hook's `with:` payload reaches the engine as
// map[any]any after the typed Hook is round-tripped through yaml.v2 (see resolveHookForExecution),
// so both shapes are accepted; non-string keys are skipped.
func stringKeyedMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case map[any]any:
		converted := make(map[string]any, len(m))
		for key, value := range m {
			if name, isString := key.(string); isString {
				converted[name] = value
			}
		}
		return converted, true
	default:
		return nil, false
	}
}

// withoutScriptSource returns m without the internal script_source, script_source_sha256, and
// literal_fields keys. It returns m itself when all are absent, so the common case does not copy.
func withoutScriptSource(m map[string]any) map[string]any {
	_, hasSource := m[utils.ScriptSourceKey]
	_, hasHash := m[utils.ScriptSourceSHA256Key]
	_, hasLiteral := m[schema.LiteralFieldsKey]
	if !hasSource && !hasHash && !hasLiteral {
		return m
	}
	filtered := make(map[string]any, len(m))
	for k, v := range m {
		if k != utils.ScriptSourceKey && k != utils.ScriptSourceSHA256Key && k != schema.LiteralFieldsKey {
			filtered[k] = v
		}
	}
	return filtered
}
