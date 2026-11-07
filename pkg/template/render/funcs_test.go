package render

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
	"text/template"

	gomplate "github.com/hairyhenderson/gomplate/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func renderWith(t *testing.T, funcs template.FuncMap, text string) (string, error) {
	t.Helper()

	tmpl, err := template.New("test").Funcs(funcs).Parse(text)
	require.NoError(t, err)
	var out bytes.Buffer
	err = tmpl.Execute(&out, nil)
	return out.String(), err
}

func sortedKeys(funcs template.FuncMap) []string {
	keys := make([]string, 0, len(funcs))
	for key := range funcs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

// The per-render map must expose exactly Gomplate's functions with Gomplate's signatures, so
// text/template converts arguments identically. A Gomplate upgrade that renames or retypes a
// render-scoped function fails here instead of panicking in a template.
func TestGomplateFuncsMatchGomplateFunctionsAndSignatures(t *testing.T) {
	for _, experimental := range []bool{false, true} {
		upstreamCtx := context.Background()
		ctx := context.Background()
		if experimental {
			upstreamCtx = gomplate.SetExperimental(upstreamCtx)
			ctx = WithExperimental(ctx)
		}
		upstream := gomplate.CreateFuncs(upstreamCtx)
		funcs := gomplateFuncs(ctx)

		require.Equal(t, sortedKeys(upstream), sortedKeys(funcs))
		for _, name := range renderScopedFuncs {
			require.Contains(t, upstream, name)
			assert.Equal(t, reflect.TypeOf(upstream[name]), reflect.TypeOf(funcs[name]), name)
		}
	}
}

func TestGomplateFuncsAreIsolatedFromEachOtherAndTheSharedMap(t *testing.T) {
	ctx := context.Background()
	first := gomplateFuncs(ctx)
	second := gomplateFuncs(ctx)
	sharedBefore := len(sharedFuncMap(false))

	first["datasource"] = "replaced"
	delete(first, "strings")
	first["added"] = "value"

	assert.NotEqual(t, "replaced", second["datasource"])
	assert.Contains(t, second, "strings")
	assert.NotContains(t, second, "added")
	assert.Len(t, sharedFuncMap(false), sharedBefore)
	assert.Contains(t, sharedFuncMap(false), "strings")
	assert.NotContains(t, sharedFuncMap(false), "added")

	third := gomplateFuncs(ctx)
	assert.Contains(t, third, "strings")
	assert.NotContains(t, third, "added")
}

// Gomplate's file namespace binds the working directory when it is created. Function maps are
// created per render, so a later render must resolve relative paths against the directory that
// is current then, not the one current when the shared map was first built.
func TestGomplateFileNamespaceUsesWorkingDirectoryOfEachRender(t *testing.T) {
	firstDir := t.TempDir()
	secondDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(firstDir, "value.txt"), []byte("first"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(secondDir, "value.txt"), []byte("second"), 0o600))

	t.Chdir(firstDir)
	first, err := renderWith(t, GomplateFuncs(context.Background(), &Data{}), `{{ file.Read "value.txt" }}`)
	require.NoError(t, err)

	t.Chdir(secondDir)
	second, err := renderWith(t, GomplateFuncs(context.Background(), &Data{}), `{{ file.Read "value.txt" }}`)
	require.NoError(t, err)

	assert.Equal(t, "first", first)
	assert.Equal(t, "second", second)
}

// Cloud namespaces read client options from the environment and cache clients, so each function
// map owns its namespace while repeated use within one map reuses it.
func TestGomplateCloudNamespacesArePerFunctionMap(t *testing.T) {
	for _, name := range []string{"aws", "gcp"} {
		first := gomplateFuncs(context.Background())
		second := gomplateFuncs(context.Background())

		firstNamespace := first[name].(func() any)()
		assert.Same(t, firstNamespace, first[name].(func() any)(), name)
		assert.NotSame(t, firstNamespace, second[name].(func() any)(), name)
	}
}

func TestGomplateFuncsHonorExperimentalSetting(t *testing.T) {
	const text = `{{ net.CIDRHost 1 "10.0.0.0/24" }}`

	_, err := renderWith(t, gomplateFuncs(context.Background()), text)
	require.ErrorContains(t, err, "experimental mode not enabled")

	result, err := renderWith(t, gomplateFuncs(WithExperimental(context.Background())), text)
	require.NoError(t, err)
	assert.Equal(t, "10.0.0.1", result)

	// The experimental map must not leak into later non-experimental maps.
	_, err = renderWith(t, gomplateFuncs(context.Background()), text)
	require.ErrorContains(t, err, "experimental mode not enabled")
}

// BenchmarkGomplateFuncs measures building the function map Atmos creates for every template render.
func BenchmarkGomplateFuncs(b *testing.B) {
	ctx := context.Background()
	for b.Loop() {
		GomplateFuncs(ctx, &Data{})
	}
}
