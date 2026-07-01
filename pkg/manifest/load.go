package manifest

import (
	"encoding/json"
	"errors"
	"reflect"

	"github.com/santhosh-tekuri/jsonschema/v5"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Detect returns the kind and apiVersion declared by a YAML manifest without
// validating or decoding its spec. Use it to dispatch documents to the right
// kind before calling Load.
func Detect(data []byte) (kind, apiVersion string, err error) {
	defer perf.Track(nil, "manifest.Detect")()

	var probe envelopeProbe
	if err := yaml.Unmarshal(data, &probe); err != nil {
		return "", "", errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanation("The document is not valid YAML").
			WithHint("Check for syntax errors (indentation, quotes, colons)").
			Err()
	}
	return probe.Kind, probe.APIVersion, nil
}

// Load parses, validates, and decodes a YAML manifest of the given kind.
// The document is validated against the JSON Schema generated at
// registration time before being decoded into the typed envelope, so a
// successful return guarantees a schema-valid manifest.
//
// The type parameter S must match the spec type registered for kind; a
// mismatch is rejected before decoding to prevent silently returning a
// zeroed or partially decoded Spec.
func Load[S any](kind string, data []byte) (*Manifest[S], error) {
	defer perf.Track(nil, "manifest.Load")()

	// Verify that the caller's spec type S matches the registered prototype
	// type so that a wrong-spec call fails loudly rather than decoding silently
	// into a zero value (unknown YAML fields are ignored by yaml.Unmarshal).
	def, ok := GetDefinition(kind)
	if ok && def.SpecType() != nil {
		var zero S
		callerType := reflect.TypeOf(zero)
		// Unwrap pointer if the caller used *T.
		if callerType != nil && callerType.Kind() == reflect.Ptr {
			callerType = callerType.Elem()
		}
		if callerType != nil && callerType != def.SpecType() {
			return nil, errUtils.Build(errUtils.ErrManifestKindMismatch).
				WithExplanationf(
					"Load[%s] was called for kind `%s` but the registered spec type is `%s`",
					callerType.Name(), kind, def.SpecType().Name(),
				).
				WithHint("Use the spec type that matches the registered kind").
				WithContext("kind", kind).
				WithContext("caller_type", callerType.Name()).
				WithContext("registered_type", def.SpecType().Name()).
				Err()
		}
	}

	if err := Validate(kind, data); err != nil {
		return nil, err
	}

	var m Manifest[S]
	if err := yaml.Unmarshal(data, &m); err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanationf("Failed to decode `%s` manifest", kind).
			WithContext("kind", kind).
			Err()
	}
	return &m, nil
}

// Validate checks a YAML manifest against the registered JSON Schema for the
// given kind, including the envelope (apiVersion, kind, metadata).
func Validate(kind string, data []byte) error {
	defer perf.Track(nil, "manifest.Validate")()

	def, ok := GetDefinition(kind)
	if !ok {
		return errUtils.Build(errUtils.ErrManifestKindUnknown).
			WithExplanationf("Manifest kind `%s` is not registered", kind).
			WithHint(kindsHint()).
			WithContext("kind", kind).
			Err()
	}

	declaredKind, declaredAPIVersion, err := Detect(data)
	if err != nil {
		return err
	}
	if declaredKind != kind {
		return errUtils.Build(errUtils.ErrManifestKindMismatch).
			WithExplanationf("Expected kind `%s` but the document declares `%s`", kind, declaredKind).
			WithHint("Set `kind` to the expected manifest kind").
			WithContext("expected_kind", kind).
			WithContext("declared_kind", declaredKind).
			Err()
	}
	if declaredAPIVersion != def.APIVersion {
		return errUtils.Build(errUtils.ErrManifestAPIVersion).
			WithExplanationf("Expected apiVersion `%s` but the document declares `%s`", def.APIVersion, declaredAPIVersion).
			WithHintf("Set `apiVersion: %s`", def.APIVersion).
			WithContext("expected_api_version", def.APIVersion).
			WithContext("declared_api_version", declaredAPIVersion).
			Err()
	}

	instance, err := yamlToJSONValue(data)
	if err != nil {
		return err
	}

	if err := def.compiled.Validate(instance); err != nil {
		return buildValidationError(kind, err)
	}
	return nil
}

// yamlToJSONValue converts YAML bytes into a JSON-compatible Go value
// suitable for JSON Schema validation.
func yamlToJSONValue(data []byte) (any, error) {
	var value any
	if err := yaml.Unmarshal(data, &value); err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanation("The document is not valid YAML").
			Err()
	}

	// Round-trip through JSON to normalize types (e.g. integers to float64)
	// the way the JSON Schema validator expects.
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			WithExplanation("The document contains values that cannot be represented as JSON").
			Err()
	}

	var normalized any
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return nil, errUtils.Build(errUtils.ErrManifestParse).
			WithCause(err).
			Err()
	}
	return normalized, nil
}

// buildValidationError converts a jsonschema validation error into an Atmos
// error with actionable detail.
func buildValidationError(kind string, err error) error {
	var validationErr *jsonschema.ValidationError
	if errors.As(err, &validationErr) {
		detail, marshalErr := json.MarshalIndent(validationErr.BasicOutput(), "", "  ")
		if marshalErr != nil {
			detail = []byte(validationErr.Error())
		}
		return errUtils.Build(errUtils.ErrManifestValidation).
			WithExplanationf("The `%s` manifest does not conform to its schema", kind).
			WithCausef("%s", string(detail)).
			WithHint("Compare the manifest against the schema fields and types").
			WithContext("kind", kind).
			Err()
	}
	return errUtils.Build(errUtils.ErrManifestValidation).
		WithCause(err).
		WithContext("kind", kind).
		Err()
}
