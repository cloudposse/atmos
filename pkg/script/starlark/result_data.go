package starlark

import (
	"sync"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"go.starlark.net/syntax"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/script"
)

// decodedResult keeps captured streams intact and decodes JSON only on demand.
// Decoded values are frozen before publication, including when first accessed
// concurrently by tasks sharing a result created on the parent thread.
type decodedResult struct {
	attrs   *starlarkstruct.Struct
	payload string
	source  string
	kind    string
	// hint tells the user how to get data when the payload is not JSON. It depends on what
	// produced the result, so the producer sets it.
	hint    string
	once    sync.Once
	decoded starlark.Value
	err     error
}

var _ starlark.HasAttrs = (*decodedResult)(nil)

// Hints for a result whose payload is not JSON, by producer.
const (
	processDataHint = "Read result.stdout, or request JSON output from the command."
	atmosDataHint   = "Read result.stdout, or request JSON output with the command's --format=json flag."
)

func newProcessResult(output script.ProcessOutput) *decodedResult {
	attrs := starlarkstruct.FromStringDict(starlark.String("process_result"), starlark.StringDict{
		"stdout": starlark.String(output.Stdout), "stderr": starlark.String(output.Stderr), "exit_code": starlark.MakeInt(output.ExitCode),
	})
	return &decodedResult{attrs: attrs, payload: output.Stdout, source: "stdout", kind: "process_result", hint: processDataHint}
}

// String preserves the raw result representation without decoding stdout.
func (r *decodedResult) String() string {
	defer perf.Track(nil, "starlark.decodedResult.String")()

	return r.attrs.String()
}

// Type identifies the original result kind.
func (r *decodedResult) Type() string {
	defer perf.Track(nil, "starlark.decodedResult.Type")()

	return r.kind
}

// Truth reports that a result exists, independently of its exit code.
func (r *decodedResult) Truth() starlark.Bool {
	defer perf.Track(nil, "starlark.decodedResult.Truth")()

	return starlark.True
}

// Freeze is a no-op: raw fields and lazily published data are immutable.
func (r *decodedResult) Freeze() {
	defer perf.Track(nil, "starlark.decodedResult.Freeze")()
}

// Hash preserves hashing by the raw result fields.
func (r *decodedResult) Hash() (uint32, error) {
	defer perf.Track(nil, "starlark.decodedResult.Hash")()

	return r.attrs.Hash()
}

// CompareSameType compares raw fields, independently of whether JSON was read.
func (r *decodedResult) CompareSameType(op syntax.Token, other starlark.Value, depth int) (bool, error) {
	defer perf.Track(nil, "starlark.decodedResult.CompareSameType")()

	return starlark.CompareDepth(op, r.attrs, other.(*decodedResult).attrs, depth)
}

// AttrNames includes the lazy JSON view alongside the captured process fields.
func (r *decodedResult) AttrNames() []string {
	defer perf.Track(nil, "starlark.decodedResult.AttrNames")()

	return append([]string{"data"}, r.attrs.AttrNames()...)
}

// Attr decodes the payload on the first access to data, retaining parse failures for
// subsequent accesses. Other attributes never require valid JSON.
func (r *decodedResult) Attr(name string) (starlark.Value, error) {
	defer perf.Track(nil, "starlark.decodedResult.Attr")()

	if name != "data" {
		return r.attrs.Attr(name)
	}
	r.once.Do(func() {
		r.decoded, r.err = starlark.Call(&starlark.Thread{Name: "result.data"}, starjson.Module.Members["decode"], starlark.Tuple{starlark.String(r.payload)}, nil)
		if r.err != nil {
			r.err = r.notJSON(r.err)
			return
		}
		r.decoded.Freeze()
	})
	return r.decoded, r.err
}

// notJSON reports a payload that is not valid JSON, with a hint that matches its producer.
func (r *decodedResult) notJSON(cause error) error {
	hint := r.hint
	if hint == "" {
		hint = "Read result." + r.source + " instead, or produce JSON."
	}
	return failWithAll(errUtils.ErrStarlark, []error{
		script.NewDiagnostic("result.data requires valid JSON on " + r.source).With(func(b *errUtils.ErrorBuilder) { b.WithHint(hint) }).Err(), cause,
	}, "result.data requires valid JSON on %s", r.source)
}

// MarshalJSON preserves serialization of original result fields, even for text
// commands. Serialize result.data explicitly to emit the decoded payload.
func (r *decodedResult) MarshalJSON() ([]byte, error) {
	defer perf.Track(nil, "starlark.decodedResult.MarshalJSON")()

	encoded, err := starlark.Call(&starlark.Thread{Name: "result.encode"}, starjson.Module.Members["encode"], starlark.Tuple{r.attrs}, nil)
	if err != nil {
		return nil, err
	}
	return []byte(encoded.(starlark.String)), nil
}
