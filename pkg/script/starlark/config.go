package starlark

import (
	"errors"
	"net/url"
	"path/filepath"
	"strings"

	"go.starlark.net/starlark"

	errUtils "github.com/cloudposse/atmos/errors"
)

func (s *session) configModule() starlark.Value {
	return module("config", starlark.StringDict{
		"load":     starlark.NewBuiltin("config.load", s.loadConfig),
		"validate": starlark.NewBuiltin("config.validate", s.validateConfig),
	})
}

func (s *session) loadConfig(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var path, format string
	var schema starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "path", &path, "schema", &schema, "format?", &format); err != nil {
		return nil, err
	}
	if path == "" {
		return nil, invalidArg("config.load: path must not be empty")
	}
	path = s.modulePath(thread, path)
	doc, err := s.readConfig(thread, path, format)
	if err != nil {
		return nil, configDiagnostic(path, doc, err)
	}
	result, err := s.checkConfig(thread, doc.value, schema)
	if err != nil {
		return nil, configDiagnostic(path, doc, err)
	}
	return result, nil
}

func (s *session) validateConfig(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	var value, schema starlark.Value
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "value", &value, "schema", &schema); err != nil {
		return nil, err
	}
	result, err := s.checkConfig(thread, value, schema)
	if err != nil {
		return nil, configDiagnostic("config.validate", nil, err)
	}
	return result, nil
}

func (s *session) checkConfig(thread *starlark.Thread, value, schema starlark.Value) (starlark.Value, error) {
	if path, ok := starlark.AsString(schema); ok {
		location, err := s.configSchemaLocation(thread, path)
		if err != nil {
			return nil, err
		}
		return s.validateConfigFile(thread, value, location)
	}
	rule, err := compileConfigRule(schema, 0)
	if err != nil {
		return nil, err
	}
	return rule.validate(threadContext(thread), value, "", 0)
}

func (s *session) readConfig(thread *starlark.Thread, path, format string) (*configDocument, error) {
	if format == "" {
		format = strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	}
	if format == "yml" {
		format = "yaml"
	}
	if format != "json" && format != "yaml" {
		return nil, invalidArg("expected .json, .yaml, or .yml; specify format=\"json\" or format=\"yaml\" for other files")
	}
	ctx := threadContext(thread)
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	contents, err := s.engine.readFile(path)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return parseConfig(ctx, contents, format)
}

func configDiagnostic(source string, doc *configDocument, err error) error {
	var problem *configProblem
	if doc != nil && errors.As(err, &problem) {
		path := problem.path
		for {
			if pos, ok := doc.locations[path]; ok {
				return failWith(errUtils.ErrStarlarkInvalidArgument, err, "%s:%d:%d: %s", source, pos.line, pos.column, problem)
			}
			if path == "" {
				break
			}
			path = path[:strings.LastIndex(path, "/")]
		}
	}
	return failWith(errUtils.ErrStarlarkInvalidArgument, err, "%s: %s", source, err)
}

func (s *session) configSchemaLocation(thread *starlark.Thread, path string) (string, error) {
	if path == "" {
		return "", invalidArg("schema path must not be empty")
	}
	resource, err := url.Parse(path)
	if err != nil {
		return "", err
	}
	if resource.IsAbs() && !filepath.IsAbs(path) {
		if err := configHTTPSURL(resource); err != nil {
			return "", err
		}
		return resource.String(), nil
	}
	return s.modulePath(thread, path), nil
}
