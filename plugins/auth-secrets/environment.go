package main

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"text/template"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

const (
	maxEnvironmentValueBytes = 1 << 20
	maxEnvironmentVariables  = 256
)

var errEnvironmentTemplate = errors.New("invalid environment credential template")

func environmentTemplates(request *pluginv1.IdentityRequest) (map[string]*template.Template, error) {
	if _, legacy := request.GetPrincipal().AsMap()["environment"]; legacy {
		return nil, status.Error(codes.InvalidArgument, "principal.environment is no longer supported; use spec.env with explicit .credentials templates")
	}
	raw, ok := request.GetSpec().AsMap()["env"].(map[string]any)
	if !ok || len(raw) == 0 || len(raw) > maxEnvironmentVariables {
		return nil, invalid()
	}
	result := make(map[string]*template.Template, len(raw))
	seen := make(map[string]bool)
	for key, rawTemplate := range raw {
		source, ok := rawTemplate.(string)
		canonical := strings.ToUpper(key)
		if !ok || !validEnvironmentTemplate(key, source) || seen[canonical] {
			return nil, invalid()
		}
		seen[canonical] = true
		value, err := parseEnvironmentTemplate(source)
		if err != nil {
			return nil, invalid()
		}
		result[key] = value
	}
	return result, nil
}

func parseEnvironmentTemplate(source string) (*template.Template, error) {
	value, err := template.New("environment").Option("missingkey=error").Funcs(environmentFunctions()).Parse(source)
	if err != nil {
		return nil, err
	}
	for _, associated := range value.Templates() {
		guardEnvironmentOutput(associated.Root)
	}
	return value, nil
}

func validEnvironmentTemplate(key, source string) bool {
	return source != "" && len(source) <= maxEnvironmentValueBytes && !strings.ContainsRune(source, 0) && environmentKey.MatchString(key)
}

// strictIndex preserves ordinary template indexing but rejects absent map entries.
func strictIndex(value any, indexes ...any) (any, error) {
	current := reflect.ValueOf(value)
	for _, index := range indexes {
		for current.IsValid() && (current.Kind() == reflect.Interface || current.Kind() == reflect.Pointer) {
			current = current.Elem()
		}
		if !current.IsValid() {
			return nil, errEnvironmentTemplate
		}
		var err error
		current, err = indexedEnvironmentValue(current, reflect.ValueOf(index))
		if err != nil {
			return nil, err
		}
	}
	if !current.IsValid() {
		return nil, errEnvironmentTemplate
	}
	return current.Interface(), nil
}

func indexedEnvironmentValue(current, key reflect.Value) (reflect.Value, error) {
	switch current.Kind() {
	case reflect.Map:
		if !key.IsValid() || !key.Type().AssignableTo(current.Type().Key()) {
			return reflect.Value{}, errEnvironmentTemplate
		}
		value := current.MapIndex(key)
		if !value.IsValid() {
			return reflect.Value{}, errEnvironmentTemplate
		}
		return value, nil
	case reflect.Array, reflect.Slice, reflect.String:
		if validSequenceIndex(current, key) {
			return current.Index(int(key.Int())), nil
		}
	}
	return reflect.Value{}, errEnvironmentTemplate
}

func validSequenceIndex(sequence, key reflect.Value) bool {
	return key.IsValid() && key.CanInt() && key.Int() >= 0 && key.Int() < int64(sequence.Len())
}

type boundedEnvironmentValue struct{ buffer bytes.Buffer }

func (value *boundedEnvironmentValue) Write(data []byte) (int, error) {
	if len(data) > maxEnvironmentValueBytes-value.buffer.Len() || bytes.IndexByte(data, 0) >= 0 {
		return 0, errEnvironmentTemplate
	}
	return value.buffer.Write(data)
}

func renderEnvironment(templates map[string]*template.Template, credentials map[string]any) (map[string]any, error) {
	data := make(map[string]any, len(templates))
	for key, value := range templates {
		var output boundedEnvironmentValue
		if err := value.Execute(&output, map[string]any{"credentials": credentials}); err != nil {
			// Template execution errors can include credential data; expose no details.
			return nil, invalid()
		}
		data[key] = output.buffer.String()
	}
	return data, nil
}
