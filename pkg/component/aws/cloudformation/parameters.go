package cloudformation

import (
	"gopkg.in/yaml.v3"

	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
)

// registerNoEchoValues finds parameters marked NoEcho in the template and registers
// their resolved values with the masker, so plan/diff/apply/describe output never
// shows them unmasked. Resolved `!secret` values are already registered generically
// by the YAML-function resolution pipeline before this component ever runs — this
// covers the CloudFormation-specific NoEcho contract on top of that.
//
// Best-effort: an unparsable template does not fail the operation (the deploy itself
// will surface real template errors); it just means NoEcho detection is skipped.
func registerNoEchoValues(templateBody string, spec *stackSpec) {
	defer perf.Track(nil, "cloudformation.registerNoEchoValues")()

	noEchoNames := noEchoParameterNames(templateBody)
	if len(noEchoNames) == 0 {
		return
	}

	for _, param := range spec.Parameters {
		if param.ParameterKey == nil || param.ParameterValue == nil {
			continue
		}
		if noEchoNames[*param.ParameterKey] {
			iolib.RegisterSecretValue(*param.ParameterValue)
		}
	}
}

// noEchoParameterNames parses a CloudFormation template (YAML or JSON — CloudFormation's
// JSON is valid YAML) and returns the set of parameter names declared with NoEcho: true.
func noEchoParameterNames(templateBody string) map[string]bool {
	var doc struct {
		Parameters map[string]struct {
			NoEcho any `yaml:"NoEcho"`
		} `yaml:"Parameters"`
	}

	if err := yaml.Unmarshal([]byte(templateBody), &doc); err != nil {
		return nil
	}

	names := make(map[string]bool)
	for name, param := range doc.Parameters {
		if isTruthy(param.NoEcho) {
			names[name] = true
		}
	}
	return names
}

// isTruthy handles NoEcho being decoded as either a YAML bool or a JSON/YAML string
// ("true"/"false"), both valid in CloudFormation templates.
func isTruthy(value any) bool {
	switch v := value.(type) {
	case bool:
		return v
	case string:
		return v == "true" || v == "True" || v == "TRUE"
	default:
		return false
	}
}
