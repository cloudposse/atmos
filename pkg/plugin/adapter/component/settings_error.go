package component

import (
	"errors"
	"regexp"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"

	errUtils "github.com/cloudposse/atmos/errors"
)

// keySeparator joins the parts of a configuration key.
const keySeparator = "."

// quotedName matches a property name that a schema message quotes, as in "additionalProperties 'x' not allowed".
var quotedName = regexp.MustCompile(`'([^']+)'`)

// settingsError describes a schema violation by the keys involved. Schema messages can echo the
// offending value, so only key names, the failing keyword, and the section are reported.
func (p *Provider) settingsError(err error) error {
	section := "components.terraform." + p.name
	builder := errUtils.Build(errUtils.ErrPluginInvalidConfig).WithContext("config_section", section)
	var violation *jsonschema.ValidationError
	if !errors.As(err, &violation) {
		return builder.WithExplanationf("The settings under `%s` do not match the schema of configuration provider `%s`.", section, p.name).Err()
	}
	seen := map[string]bool{}
	for _, leaf := range leaves(violation) {
		line := describeViolation(section, leaf)
		if seen[line] {
			continue
		}
		seen[line] = true
		builder.WithExplanation(line)
	}
	return builder.WithHintf("Fix the setting named above. `atmos config set <key> <value>` edits atmos.yaml without loading plugin settings, so it works even while this error is shown.").Err()
}

func leaves(err *jsonschema.ValidationError) []*jsonschema.ValidationError {
	if len(err.Causes) == 0 {
		return []*jsonschema.ValidationError{err}
	}
	var result []*jsonschema.ValidationError
	for _, cause := range err.Causes {
		result = append(result, leaves(cause)...)
	}
	return result
}

func describeViolation(section string, leaf *jsonschema.ValidationError) string {
	key := section + strings.ReplaceAll(leaf.InstanceLocation, "/", keySeparator)
	keyword := leaf.KeywordLocation[strings.LastIndex(leaf.KeywordLocation, "/")+1:]
	switch keyword {
	case "additionalProperties":
		if name := quotedName.FindStringSubmatch(leaf.Message); name != nil {
			return "Unknown setting `" + joinKey(key, name[1]) + "`. Check the spelling."
		}
		return "Unknown setting under `" + key + "`. Check the spelling."
	case "required":
		if name := quotedName.FindStringSubmatch(leaf.Message); name != nil {
			return "Missing required setting `" + joinKey(key, name[1]) + "`."
		}
		return "A required setting is missing under `" + key + "`."
	case "type":
		return "`" + key + "` has the wrong type."
	default:
		return "`" + key + "` has an unsupported value."
	}
}

// joinKey appends a property name to a key path, tolerating a trailing separator.
func joinKey(key, name string) string {
	return strings.TrimSuffix(key, keySeparator) + keySeparator + name
}
