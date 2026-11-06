package compatibility

import (
	"context"
	"reflect"
	"testing"
	"text/template"

	"github.com/hairyhenderson/gomplate/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestLogDeprecationReporter_DedupsPerTemplateAndName verifies the default
// reporter's dedup key is (templateName, deprecated): repeating the same
// pair is a no-op, but a different template name or a different deprecated
// name each get their own entry.
func TestLogDeprecationReporter_DedupsPerTemplateAndName(t *testing.T) {
	r := &logDeprecationReporter{}
	r.Deprecated("t", "conv.Bool", "conv.ToBool", "hint")
	r.Deprecated("t", "conv.Bool", "conv.ToBool", "hint")
	r.Deprecated("t", "conv.Slice", "coll.Slice", "hint")
	r.Deprecated("t2", "conv.Bool", "conv.ToBool", "hint")

	count := 0
	r.seen.Range(func(_, _ any) bool {
		count++
		return true
	})
	assert.Equal(t, 3, count)
}

// TestNamespaceWrappersCoverUpstreamMethods walks the method set of the real
// gomplate v5 conv/strings/net/coll namespace objects via reflection and
// asserts every exported method is present on the corresponding wrapper (or,
// for coll, on the collNamespace interface this package uses to obtain it).
// This fails the build if upstream renames, removes, or adds a method this
// package hasn't accounted for, instead of that method silently
// disappearing from (or never appearing in) Atmos templates. It reads
// gomplate's function map directly: importing the render package from an
// internal test would be an import cycle.
func TestNamespaceWrappersCoverUpstreamMethods(t *testing.T) {
	gomplateFuncs := gomplate.CreateFuncs(context.Background())

	cases := []struct {
		key        string
		wrapperTyp reflect.Type
	}{
		{key: "conv", wrapperTyp: reflect.TypeOf(&convWrapper{})},
		{key: "strings", wrapperTyp: reflect.TypeOf(&stringsWrapper{})},
		{key: "net", wrapperTyp: reflect.TypeOf(&netWrapper{})},
		{key: "coll", wrapperTyp: reflect.TypeOf((*collNamespace)(nil)).Elem()},
	}

	for _, tc := range cases {
		t.Run(tc.key, func(t *testing.T) {
			real := realNamespace(t, gomplateFuncs, tc.key)
			realMethods := methodNames(reflect.TypeOf(real))
			wrapperMethods := methodNames(tc.wrapperTyp)

			for m := range realMethods {
				assert.Contains(t, wrapperMethods, m,
					"gomplate v5's %s namespace has method %q with no matching method here", tc.key, m)
			}
		})
	}
}

func realNamespace(t *testing.T, funcs template.FuncMap, key string) any {
	t.Helper()
	factory, ok := funcs[key].(func() any)
	require.True(t, ok, "missing gomplate namespace factory %q", key)
	return factory()
}

func methodNames(typ reflect.Type) map[string]struct{} {
	out := make(map[string]struct{}, typ.NumMethod())
	for i := range typ.NumMethod() {
		out[typ.Method(i).Name] = struct{}{}
	}
	return out
}
