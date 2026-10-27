package starlark

import (
	"encoding/json"
	"errors"
	"io"
	"strings"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"gopkg.in/yaml.v3"
)

func yamlModule() starlark.Value {
	return module("yaml", starlark.StringDict{
		"decode":   starlark.NewBuiltin("yaml.decode", decodeYAML),
		"validate": starlark.NewBuiltin("yaml.validate", validateYAML),
	})
}

// decodeYAML reads one data document and reuses Starlark's JSON value conversion.
// No Atmos tags, templates, or external references are evaluated.
func decodeYAML(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var content string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "content", &content); err != nil {
		return nil, err
	}
	if err := threadContext(t).Err(); err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(content))
	var data any
	if err := decoder.Decode(&data); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	var extra yaml.Node
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err != nil {
			return nil, err
		}
		return nil, invalidArg("yaml.decode requires a single document")
	}
	if err := validateYAMLKeys(data); err != nil {
		return nil, err
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return starlark.Call(t, starjson.Module.Members["decode"], starlark.Tuple{starlark.String(encoded)}, nil)
}

// validateYAMLKeys rejects key coercion, including inside lists and nested mappings.
func validateYAMLKeys(data any) error {
	switch value := data.(type) {
	case map[any]any:
		return invalidArg("yaml.decode requires string mapping keys")
	case map[string]any:
		for _, child := range value {
			if err := validateYAMLKeys(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range value {
			if err := validateYAMLKeys(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// validateYAML parses every document into nodes. Custom tags are syntax, not
// executable Atmos functions; mappings are not constrained to Go-compatible keys.
func validateYAML(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var content string
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "content", &content); err != nil {
		return nil, err
	}
	decoder := yaml.NewDecoder(strings.NewReader(content))
	for {
		if err := threadContext(t).Err(); err != nil {
			return nil, err
		}
		var node yaml.Node
		err := decoder.Decode(&node)
		if errors.Is(err, io.EOF) {
			return starlark.None, nil
		}
		if err != nil {
			return nil, err
		}
	}
}
