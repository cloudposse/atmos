package schema

import (
	"errors"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/go-viper/mapstructure/v2"
	"gopkg.in/yaml.v3"
)

// ErrInvalidRetryConfig is returned when a retry config cannot be decoded from a stack manifest.
var ErrInvalidRetryConfig = errors.New("invalid retry configuration")

// DecodeRetryConfig decodes a map[string]any (typically read from a stack manifest's
// `components.<type>.<name>.retry:` section) into a *RetryConfig.
//
// Returns (nil, nil) when the input is nil or empty so callers can write:
//
//	cfg, err := schema.DecodeRetryConfig(componentSection["retry"])
//
// Duration fields like "2s" are parsed via mapstructure's StringToTimeDurationHookFunc.
// Errors are joined with ErrInvalidRetryConfig for predictable error checking.
func DecodeRetryConfig(raw any) (*RetryConfig, error) {
	if raw == nil {
		return nil, nil
	}
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%w: expected map, got %T", ErrInvalidRetryConfig, raw)
	}
	if len(m) == 0 {
		return nil, nil
	}

	if err := checkRetryKeys(retryMapKeys(m)); err != nil {
		return nil, err
	}

	var cfg RetryConfig
	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		Result:           &cfg,
		TagName:          "mapstructure",
		WeaklyTypedInput: true,
		ErrorUnused:      true,
		DecodeHook:       mapstructure.StringToTimeDurationHookFunc(),
	})
	if err != nil {
		return nil, errors.Join(ErrInvalidRetryConfig, err)
	}
	if err := decoder.Decode(m); err != nil {
		return nil, errors.Join(ErrInvalidRetryConfig, err)
	}
	return &cfg, nil
}

// RetryFieldNames returns the YAML keys a `retry:` block accepts, in declaration order. It is
// derived from RetryConfig's yaml tags so it cannot drift from the struct.
func RetryFieldNames() []string {
	t := reflect.TypeOf(RetryConfig{})
	names := make([]string, 0, t.NumField())
	for i := range t.NumField() {
		name, _, _ := strings.Cut(t.Field(i).Tag.Get("yaml"), ",")
		if name != "" && name != "-" {
			names = append(names, name)
		}
	}
	return names
}

// UnknownRetryField reports whether err describes an unknown `retry:` key and, if so,
// returns the offending key. It understands the strict-decode error yaml.v3 produces
// ("field delay not found in type schema.RetryConfig") and the one checkRetryKeys produces, so
// callers can show a clean message instead of leaking the Go type name.
func UnknownRetryField(err error) (string, bool) {
	if err == nil {
		return "", false
	}
	msg := err.Error()
	if _, rest, ok := strings.Cut(msg, "unknown retry field \""); ok {
		if field, _, ok := strings.Cut(rest, "\""); ok {
			return field, true
		}
	}
	if _, rest, ok := strings.Cut(msg, "field "); ok {
		if field, tail, ok := strings.Cut(rest, " not found in type "); ok && strings.HasPrefix(tail, "schema.RetryConfig") {
			return field, true
		}
	}
	return "", false
}

// ValidateRetryNode rejects unknown keys in the `retry:` block of a step mapping. The yaml.v3
// decoder drops unknown keys silently unless it is strict, so a misspelled key such as `delay:`
// would otherwise be ignored and the step would retry with defaults. A step with no `retry:`
// block, or one that is not a mapping, is left to the regular decode.
func ValidateRetryNode(step *yaml.Node) error {
	if step == nil || step.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(step.Content); i += 2 {
		if step.Content[i].Value != "retry" {
			continue
		}
		retryNode := step.Content[i+1]
		if retryNode.Kind == yaml.AliasNode {
			retryNode = retryNode.Alias
		}
		if retryNode == nil || retryNode.Kind != yaml.MappingNode {
			return nil
		}
		// Decode the effective mapping so YAML merge keys remain supported and
		// inherited misspellings are checked as well as directly written keys.
		var fields map[string]any
		if err := retryNode.Decode(&fields); err != nil {
			return errors.Join(ErrInvalidRetryConfig, err)
		}
		return checkRetryKeys(retryMapKeys(fields))
	}
	return nil
}

// retryMapKeys returns the keys of a retry map.
func retryMapKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// checkRetryKeys returns ErrInvalidRetryConfig for the first key that is not a RetryConfig field.
func checkRetryKeys(keys []string) error {
	valid := RetryFieldNames()
	for _, key := range keys {
		known := false
		for _, name := range valid {
			if key == name {
				known = true
				break
			}
		}
		if !known {
			return fmt.Errorf("%w: unknown retry field %q (valid fields: %s)", ErrInvalidRetryConfig, key, strings.Join(valid, ", "))
		}
	}
	return nil
}

// validateRetryMapValue rejects unknown keys in a step map's `retry:` value when it is a map.
func validateRetryMapValue(raw any) error {
	m, ok := stringifyTaskMap(raw)
	if !ok {
		return nil
	}
	return checkRetryKeys(retryMapKeys(m))
}
