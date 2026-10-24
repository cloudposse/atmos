package starlark

import (
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/santhosh-tekuri/jsonschema/v5"
	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
)

// Schema resources use engine-backed file reads or bounded HTTPS requests, never
// the library's process-global loader. The compiler resolves relative $refs.
func (s *session) validateConfigFile(thread *starlark.Thread, value starlark.Value, path string) (starlark.Value, error) {
	ctx := threadContext(thread)
	records, err := configRecords(ctx, value, 0)
	if err != nil {
		return nil, err
	}
	compiler := jsonschema.NewCompiler()
	resources := 0
	compiler.LoadURL = func(location string) (io.ReadCloser, error) {
		resources++
		if resources > configSchemaMaxResources {
			return nil, invalidArg("schema exceeds %d resource limit", configSchemaMaxResources)
		}
		return s.loadConfigSchema(thread, location)
	}
	location := path
	if !strings.HasPrefix(path, "https://") {
		location = (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
	}
	compiled, err := compiler.Compile(location)
	if err != nil {
		return nil, invalidArg("schema %s: %s", path, err)
	}
	encoded, err := encodeConfigJSON(thread, records)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber()
	var raw any
	if err := decoder.Decode(&raw); err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := compiled.Validate(raw); err != nil {
		return nil, configSchemaProblem(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return records, nil
}

func encodeConfigJSON(thread *starlark.Thread, value starlark.Value) (string, error) {
	encoded, err := starlark.Call(thread, starjson.Module.Members["encode"], starlark.Tuple{value}, nil)
	if err != nil {
		return "", err
	}
	return string(encoded.(starlark.String)), nil
}

func configSchemaProblem(err error) error {
	var validation *jsonschema.ValidationError
	if !errors.As(err, &validation) {
		return err
	}
	// The outer error describes the schema as a whole. The first leaf carries the
	// actionable instance pointer.
	leaf := validation
	for len(leaf.Causes) > 0 {
		leaf = leaf.Causes[0]
	}
	return &configProblem{path: leaf.InstanceLocation, message: leaf.Message}
}

func (s *session) loadConfigSchema(thread *starlark.Thread, location string) (io.ReadCloser, error) {
	resource, err := url.Parse(location)
	if err != nil {
		return nil, err
	}
	var doc *configDocument
	switch resource.Scheme {
	case "file":
		if resource.Host != "" || resource.RawQuery != "" {
			return nil, invalidArg("invalid local schema URL")
		}
		doc, err = s.readConfig(thread, filepath.FromSlash(resource.Path), "")
	case "https":
		doc, err = s.readRemoteSchema(thread, resource)
	default:
		return nil, invalidArg("schema references must use local files or HTTPS")
	}
	if err != nil {
		return nil, configDiagnostic(location, doc, err)
	}
	encoded, err := encodeConfigJSON(thread, doc.value)
	if err != nil {
		return nil, err
	}
	return io.NopCloser(strings.NewReader(encoded)), nil
}
