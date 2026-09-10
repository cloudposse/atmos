package templating

import (
	"bytes"
	"context"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
)

// deprecationCall records one DeprecationReporter.Deprecated invocation.
type deprecationCall struct {
	templateName, deprecated, replacement, hint string
}

// fakeReporter is a DeprecationReporter that records every call instead of
// logging, so tests can assert exactly what fired without capturing logs.
type fakeReporter struct {
	mu    sync.Mutex
	calls []deprecationCall
}

func (f *fakeReporter) Deprecated(templateName, deprecated, replacement, hint string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, deprecationCall{templateName, deprecated, replacement, hint})
}

// TestNamespaceShims_MatchReplacementOutput renders every removed v3
// namespace method the compatibility layer serves and its v5 replacement
// side by side: both must produce the same output, and only the v3 name
// must record exactly one deprecation warning naming the replacement.
func TestNamespaceShims_MatchReplacementOutput(t *testing.T) {
	tests := []struct {
		name            string
		deprecatedText  string
		replacementText string
		wantOut         string
		wantDeprecated  string
		wantReplacement string
	}{
		{
			name:            "conv.Bool",
			deprecatedText:  `{{ conv.Bool "yes" }}`,
			replacementText: `{{ conv.ToBool "yes" }}`,
			wantOut:         "true",
			wantDeprecated:  "conv.Bool",
			wantReplacement: "conv.ToBool",
		},
		{
			name:            "conv.Slice",
			deprecatedText:  `{{ conv.Slice 1 2 3 }}`,
			replacementText: `{{ coll.Slice 1 2 3 }}`,
			wantOut:         "[1 2 3]",
			wantDeprecated:  "conv.Slice",
			wantReplacement: "coll.Slice",
		},
		{
			name:            "conv.Dict",
			deprecatedText:  `{{ $d := conv.Dict "a" 1 "b" 2 }}{{ $d.a }}-{{ $d.b }}`,
			replacementText: `{{ $d := coll.Dict "a" 1 "b" 2 }}{{ $d.a }}-{{ $d.b }}`,
			wantOut:         "1-2",
			wantDeprecated:  "conv.Dict",
			wantReplacement: "coll.Dict",
		},
		{
			name:            "conv.Has",
			deprecatedText:  `{{ conv.Has (coll.Dict "a" 1) "a" }}`,
			replacementText: `{{ coll.Has (coll.Dict "a" 1) "a" }}`,
			wantOut:         "true",
			wantDeprecated:  "conv.Has",
			wantReplacement: "coll.Has",
		},
		{
			name:            "strings.Sort",
			deprecatedText:  `{{ strings.Sort (coll.Slice "b" "a" "c") }}`,
			replacementText: `{{ coll.Sort (coll.Slice "b" "a" "c") }}`,
			wantOut:         "[a b c]",
			wantDeprecated:  "strings.Sort",
			wantReplacement: "coll.Sort",
		},
		{
			name:            "net.ParseIP",
			deprecatedText:  `{{ net.ParseIP "127.0.0.1" }}`,
			replacementText: `{{ net.ParseAddr "127.0.0.1" }}`,
			wantOut:         "127.0.0.1",
			wantDeprecated:  "net.ParseIP",
			wantReplacement: "net.ParseAddr",
		},
		{
			name:            "net.ParseIPPrefix",
			deprecatedText:  `{{ net.ParseIPPrefix "10.0.0.0/8" }}`,
			replacementText: `{{ net.ParsePrefix "10.0.0.0/8" }}`,
			wantOut:         "10.0.0.0/8",
			wantDeprecated:  "net.ParseIPPrefix",
			wantReplacement: "net.ParsePrefix",
		},
		{
			name:            "net.ParseIPRange",
			deprecatedText:  `{{ net.ParseIPRange "10.0.0.1-10.0.0.10" }}`,
			replacementText: `{{ net.ParseRange "10.0.0.1-10.0.0.10" }}`,
			wantOut:         "10.0.0.1-10.0.0.10",
			wantDeprecated:  "net.ParseIPRange",
			wantReplacement: "net.ParseRange",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deprecatedFake := &fakeReporter{}
			out, err := New(WithDeprecationReporter(deprecatedFake)).Render(context.Background(), &Request{
				Name: tt.name, Text: tt.deprecatedText,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)

			require.Len(t, deprecatedFake.calls, 1)
			call := deprecatedFake.calls[0]
			assert.Equal(t, tt.name, call.templateName)
			assert.Equal(t, tt.wantDeprecated, call.deprecated)
			assert.Equal(t, tt.wantReplacement, call.replacement)
			assert.Contains(t, call.hint, "gomplate v5")
			assert.Contains(t, call.hint, "`"+tt.wantReplacement+"`")

			replacementFake := &fakeReporter{}
			out, err = New(WithDeprecationReporter(replacementFake)).Render(context.Background(), &Request{
				Name: tt.name, Text: tt.replacementText,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
			assert.Empty(t, replacementFake.calls)
		})
	}
}

// TestNetParseIP_InvalidInputErrors verifies the shim errors on unparsable
// input rather than silently returning a zero value, matching gomplate v3's
// net.ParseIP (which also errored on invalid input, via a different parser).
func TestNetParseIP_InvalidInputErrors(t *testing.T) {
	_, err := New().Render(context.Background(), &Request{
		Name: "t", Text: `{{ net.ParseIP "not-an-ip" }}`,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse IP")
}

// TestConvWrapper_ConversionErrorsCarryHint verifies conv.ToInt64/ToInt/
// ToFloat64/Atoi wrap gomplate v5's conversion errors with a hint explaining
// that v5 no longer falls back to a zero value the way v3 did, and that this
// is a hinted error, not a deprecation warning (the method names themselves
// didn't change).
func TestConvWrapper_ConversionErrorsCarryHint(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "ToInt64", text: `{{ conv.ToInt64 "not-a-number" }}`},
		{name: "ToInt", text: `{{ conv.ToInt "not-a-number" }}`},
		{name: "ToFloat64", text: `{{ conv.ToFloat64 "not-a-number" }}`},
		{name: "Atoi", text: `{{ conv.Atoi "not-a-number" }}`},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeReporter{}
			_, err := New(WithDeprecationReporter(fake)).Render(context.Background(), &Request{
				Name: "t", Text: tt.text,
			})
			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrTemplateConversion)
			assert.Contains(t, err.Error(), "guard the call with `if`")
			assert.Empty(t, fake.calls, "a hinted conversion error is not a deprecated-name warning")
		})
	}
}

// TestBareAliasShims_SprigDisabled verifies the v3 bare aliases (subject-
// first stdlib argument order) work and warn when Sprig is disabled, which is
// the only configuration where gomplate v5's removal of them is visible.
func TestBareAliasShims_SprigDisabled(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantOut    string
		deprecated string
	}{
		{name: "contains", text: `{{ contains "hello" "ell" }}`, wantOut: "true", deprecated: "contains"},
		{name: "hasPrefix", text: `{{ hasPrefix "hello" "he" }}`, wantOut: "true", deprecated: "hasPrefix"},
		{name: "hasSuffix", text: `{{ hasSuffix "hello" "lo" }}`, wantOut: "true", deprecated: "hasSuffix"},
		{name: "split", text: `{{ split "a,b,c" "," }}`, wantOut: "[a b c]", deprecated: "split"},
		{name: "trim", text: `{{ trim "  hi  " " " }}`, wantOut: "hi", deprecated: "trim"},
		{name: "splitN", text: `{{ splitN "a,b,c" "," 2 }}`, wantOut: "[a b,c]", deprecated: "splitN"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := &fakeReporter{}
			out, err := New(WithSprig(false), WithDeprecationReporter(fake)).Render(context.Background(), &Request{
				Name: tt.name, Text: tt.text,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.wantOut, out)
			require.Len(t, fake.calls, 1)
			assert.Equal(t, tt.deprecated, fake.calls[0].deprecated)
			assert.Contains(t, fake.calls[0].hint, "gomplate v5")
		})
	}
}

// TestBareAliasShims_SprigEnabled verifies that when Sprig is enabled, the
// bare names it already provides are left untouched: Sprig's function runs
// (with Sprig's own argument order and semantics), and no deprecation
// warning fires, because this package didn't register anything under that
// name. SplitN is the one name Sprig never provides (it registers the
// differently-cased "splitn"), so it works and warns either way.
func TestBareAliasShims_SprigEnabled(t *testing.T) {
	fake := &fakeReporter{}
	// Sprig's bare "trim" is strings.TrimSpace (single argument), unlike
	// gomplate v3's two-argument cutset version.
	out, err := New(WithDeprecationReporter(fake)).Render(context.Background(), &Request{
		Name: "t", Text: `{{ trim "  hi  " }}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "hi", out)
	assert.Empty(t, fake.calls)

	fake = &fakeReporter{}
	out, err = New(WithDeprecationReporter(fake)).Render(context.Background(), &Request{
		Name: "t2", Text: `{{ splitN "a,b,c" "," 2 }}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "[a b,c]", out)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, "splitN", fake.calls[0].deprecated)
}

// TestBareAliasShims_SliceNeverShadowsBuiltin verifies that "slice" is never
// registered by this package, in either Sprig configuration: with Sprig
// disabled, calling it must still hit text/template's own builtin (which has
// different semantics than gomplate v3's variadic-args-to-list version), not
// a shim.
func TestBareAliasShims_SliceNeverShadowsBuiltin(t *testing.T) {
	fake := &fakeReporter{}
	// The builtin `slice` indexes an existing list; it errors on a list of
	// bare integer literals used as if they were the list itself, which is
	// exactly the point: gomplate v3's `slice 1 2 3` (build a list) doesn't
	// work anymore, and this package must not resurrect it under this name.
	_, err := New(WithSprig(false), WithDeprecationReporter(fake)).Render(context.Background(), &Request{
		Name: "t", Text: `{{ slice (coll.Slice 1 2 3 4) 1 3 }}`,
	})
	require.NoError(t, err)
	assert.Empty(t, fake.calls, "slice must never be shimmed; it would shadow the text/template builtin")
}

// TestLogDeprecationReporter_DedupsPerTemplateAndName verifies the default
// reporter's dedup key is (templateName, deprecated): repeating the same
// pair is a no-op, but a different template name or a different deprecated
// name each get their own entry.
func TestLogDeprecationReporter_DedupsPerTemplateAndName(t *testing.T) {
	r := newLogDeprecationReporter()
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

// TestDefaultDeprecationReporter_LogsOncePerTemplate renders the same
// deprecated call twice through the default (log-backed) reporter and
// verifies the warning is only logged once: stack processing renders every
// component section of every stack, so without this dedup a single
// deprecated call in a shared catalog template would flood the log.
func TestDefaultDeprecationReporter_LogsOncePerTemplate(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	name := "TestDefaultDeprecationReporter_LogsOncePerTemplate"
	for range 2 {
		_, err := New().Render(context.Background(), &Request{
			Name: name, Text: `{{ conv.Bool "yes" }}`,
		})
		require.NoError(t, err)
	}

	// Count log lines (via the "deprecated=" key, which appears once per
	// line), not occurrences of "conv.Bool": the hint sentence itself
	// mentions "conv.Bool" twice more, which would otherwise overcount even
	// a single, correctly-deduped log line.
	assert.Equal(t, 1, strings.Count(buf.String(), "deprecated=conv.Bool"))
}

// TestNamespaceShims_ApplyOnGomplateRendererPath verifies a template that
// mixes a namespace shim with a call gomplate can only serve through its
// renderer (here, `ds`) still gets the shim: engine.baseFuncs assembles one
// function map shared by both paths, and gomplate.RenderOptions.Funcs
// overrides whatever gomplate itself would have registered for "conv".
func TestNamespaceShims_ApplyOnGomplateRendererPath(t *testing.T) {
	cfg := writeFixture(t, "cfg.yaml", "name: atmos\n")
	fake := &fakeReporter{}

	out, err := New(WithDeprecationReporter(fake)).Render(context.Background(), &Request{
		Name:        "combo",
		Text:        `{{ conv.Bool "yes" }}-{{ (ds "cfg").name }}`,
		Datasources: map[string]Datasource{"cfg": {URL: cfg}},
	})
	require.NoError(t, err)
	assert.Equal(t, "true-atmos", out)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, "conv.Bool", fake.calls[0].deprecated)
}

// TestNamespaceWrappersCoverUpstreamMethods walks the method set of the real
// gomplate v5 conv/strings/net/coll namespace objects via reflection and
// asserts every exported method is present on the corresponding wrapper (or,
// for coll, on the collNamespace interface this package uses to obtain it).
// This fails the build if upstream renames, removes, or adds a method this
// package hasn't accounted for, instead of that method silently
// disappearing from (or never appearing in) Atmos templates.
func TestNamespaceWrappersCoverUpstreamMethods(t *testing.T) {
	gomplateFuncs := GomplateFuncs(context.Background())

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
