package starlark

import (
	"encoding/json"
	"strings"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/automation"
)

const stepLibraryKey = "atmos.step-library"

func (s *session) stepsModule() starlark.Value {
	members := starlark.StringDict{
		"task":     starlark.NewBuiltin("steps.task", newTask),
		"parallel": starlark.NewBuiltin("steps.parallel", s.parallel),
		"run":      starlark.NewBuiltin("steps.run", s.runStep),
	}
	if s.spec.Steps != nil {
		for _, name := range s.spec.Steps.Names() {
			attr := strings.ReplaceAll(name, "-", "_")
			if _, reserved := members[attr]; reserved {
				continue
			}
			members[attr] = starlark.NewBuiltin("steps."+attr, func(t *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
				if len(args) != 0 {
					return nil, invalidArg("%s accepts keyword arguments; use the fields from the %s step reference", b.Name(), name)
				}
				return s.callStep(t, name, kwargs)
			})
		}
	}
	return module("steps", members)
}

func (s *session) runStep(t *starlark.Thread, _ *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) != 1 {
		return nil, invalidArg("steps.run requires one positional step type followed by keyword arguments")
	}
	name, ok := starlark.AsString(args[0])
	if !ok {
		return nil, invalidArg("steps.run type must be a string")
	}
	return s.callStep(t, name, kwargs)
}

func (s *session) callStep(t *starlark.Thread, name string, kwargs []starlark.Tuple) (starlark.Value, error) {
	library := threadSteps(t)
	if library == nil {
		return nil, invalidArg("the host has not provided a step library")
	}
	fields, err := stepFields(kwargs)
	if err != nil {
		return nil, err
	}
	encoded, err := starlark.Call(t, starjson.Module.Members["encode"], starlark.Tuple{fields}, nil)
	if err != nil {
		return nil, err
	}
	source, _ := starlark.AsString(encoded)
	var parameters map[string]any
	if err := yaml.Unmarshal([]byte(source), &parameters); err != nil {
		return nil, err
	}
	result, err := library.Run(threadContext(t), &automation.StepCall{
		Type: name, Configuration: parameters, WorkingDirectory: s.spec.WorkingDirectory,
		ProcessEnv: s.tools.Environment(s.spec.ProcessEnv),
		Stdout:     s.writer(t, stdoutStream), Stderr: s.writer(t, stderrStream), Parallel: s.spec.Parallel || t.Local(outputKey) != nil,
	})
	if err != nil {
		return nil, err
	}
	if result == nil {
		result = &automation.StepResult{}
	}
	return stepResult(t, result)
}

func stepFields(kwargs []starlark.Tuple) (*starlark.Dict, error) {
	fields := starlark.NewDict(len(kwargs))
	for _, pair := range kwargs {
		key := string(pair[0].(starlark.String))
		switch key {
		case "with_", "for_", "continue_":
			key = strings.TrimSuffix(key, "_")
		}
		if key == "type" {
			return nil, invalidArg("step type is selected by the function; use steps.run(type, **fields) for dynamic dispatch")
		}
		if _, found, _ := fields.Get(starlark.String(key)); found {
			return nil, invalidArg("duplicate step field %q", key)
		}
		if err := fields.SetKey(starlark.String(key), pair[1]); err != nil {
			return nil, err
		}
	}
	return fields, nil
}

func stepResult(t *starlark.Thread, result *automation.StepResult) (starlark.Value, error) {
	// Normalize empty collections so callers always receive lists and dictionaries.
	if result.Values == nil {
		result.Values = []string{}
	}
	if result.Metadata == nil {
		result.Metadata = map[string]any{}
	}
	if result.Outputs == nil {
		result.Outputs = map[string]string{}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return nil, invalidArg("cannot convert step result: %s", err)
	}
	value, err := starlark.Call(t, starjson.Module.Members["decode"], starlark.Tuple{starlark.String(encoded)}, nil)
	if err != nil {
		return nil, err
	}
	members := starlark.StringDict{}
	for _, pair := range value.(*starlark.Dict).Items() {
		members[string(pair[0].(starlark.String))] = pair[1]
	}
	resultValue := starlarkstruct.FromStringDict(starlark.String("step_result"), members)
	resultValue.Freeze()
	return &decodedResult{attrs: resultValue, payload: result.Value, source: "value", kind: "step_result"}, nil
}

func threadSteps(t *starlark.Thread) automation.StepLibrary {
	library, _ := t.Local(stepLibraryKey).(automation.StepLibrary)
	return library
}

func forkSteps(library automation.StepLibrary) automation.StepLibrary {
	if library == nil {
		return nil
	}
	return library.Fork()
}
