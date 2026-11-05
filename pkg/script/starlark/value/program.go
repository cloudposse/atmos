// Package value provides host-independent Starlark compilation and read-only values.
// It does not import the script host, CI, or policy packages.
package value

import (
	"fmt"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ValueMap supplies read-only values. Policy callers must supply materialized data.
type ValueMap interface {
	Keys() []string
	Get(string) (any, bool, error)
}

// Globals creates fresh read-only context bindings and a deferred conversion error sink.
func Globals(values ValueMap) (starlark.StringDict, *ItemErrors) {
	defer perf.Track(nil, "value.Globals")()

	sink := &ItemErrors{}
	return starlark.StringDict{
		"ctx":  &configurationMap{values: values, attributes: true, sink: sink},
		"json": starjson.Module,
	}, sink
}

// Err reports errors from mapping enumeration, whose Starlark API cannot return errors.
func (s *ItemErrors) Err() error {
	defer perf.Track(nil, "value.ItemErrors.Err")()

	return s.err
}

// Result converts a Starlark result into detached native values.
func Result(v starlark.Value) (any, error) {
	defer perf.Track(nil, "starlark.value.Result")()

	return configurationResult(v, make(map[starlark.Value]bool))
}

// Compile wraps a parsed function body without changing authored source locations.
// No load statements are allowed inside the resulting function.
func Compile(source, filename string, line int32, globals starlark.StringDict) (*starlark.Program, error) {
	defer perf.Track(nil, "starlark.value.Compile")()

	if line < 1 {
		line = 1
	}
	options := syntax.FileOptions{TopLevelControl: true, While: true, Set: true, Recursion: true}
	file, err := options.Parse(filename, syntax.FilePortion{Content: []byte(source), FirstLine: line, FirstCol: 1}, 0)
	if err != nil {
		return nil, err
	}
	if len(file.Stmts) == 0 {
		return nil, fmt.Errorf("%w: !starlark requires a function body with a return value", errUtils.ErrStarlark)
	}
	pos, _ := file.Stmts[0].Span()
	file.Stmts = []syntax.Stmt{&syntax.DefStmt{
		Def: pos, Name: &syntax.Ident{NamePos: pos, Name: "__atmos_value"}, Lparen: pos, Rparen: pos, Body: file.Stmts,
	}}
	return starlark.FileProgram(file, globals.Has)
}
