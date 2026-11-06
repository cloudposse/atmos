package compatibility_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/template/render"
)

// writeFixture writes content to a file in a fresh temp directory and returns its path.
func writeFixture(t *testing.T, name, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	require.NoError(t, os.WriteFile(p, []byte(content), 0o600))
	return p
}

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
			out, err := render.New(render.WithDeprecationReporter(deprecatedFake)).Render(context.Background(), &render.Request{
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
			out, err = render.New(render.WithDeprecationReporter(replacementFake)).Render(context.Background(), &render.Request{
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
	_, err := render.New().Render(context.Background(), &render.Request{
		Name: "t", Text: `{{ net.ParseIP "not-an-ip" }}`,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unable to parse IP")
}

// TestNetParseIP_LegacyIPMethods verifies the netaddr.IP value net.ParseIP
// returns keeps the v3 netaddr.IP method surface working: IsZero, Is4in6 and
// IPAddr under their v3 names/signatures (netip.Addr spells the first two
// IsValid/Is4In6 and has no equivalent of the third), Prior (netip.Addr:
// Prev), and every netip.Addr method promoted unchanged, including String.
func TestNetParseIP_LegacyIPMethods(t *testing.T) {
	tests := []struct {
		name       string
		ip         string
		wantIs4in6 bool
		wantIPAddr string
	}{
		{name: "IPv4", ip: "203.0.113.7", wantIs4in6: false, wantIPAddr: "203.0.113.7"},
		{name: "IPv4-mapped IPv6", ip: "::ffff:203.0.113.7", wantIs4in6: true, wantIPAddr: "203.0.113.7"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := render.New().Render(context.Background(), &render.Request{
				Name: "t",
				Text: `{{ $ip := net.ParseIP .ip }}` +
					`zero=[{{ $ip.IsZero }}] ` +
					`is4in6=[{{ $ip.Is4in6 }}] ` +
					`ipaddr=[{{ $ip.IPAddr }}] ` +
					`prior=[{{ $ip.Prior }}] ` +
					`string=[{{ $ip }}]`,
				Data: map[string]any{"ip": tt.ip},
			})
			require.NoError(t, err)
			assert.Contains(t, out, "zero=[false]")
			assert.Contains(t, out, "is4in6=["+strconv.FormatBool(tt.wantIs4in6)+"]")
			assert.Contains(t, out, "ipaddr=["+tt.wantIPAddr+"]")
			assert.Contains(t, out, "string=["+tt.ip+"]")

			// Prior must not panic and must differ from the original address.
			assert.NotContains(t, out, "prior=["+tt.ip+"]")
		})
	}
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
			_, err := render.New(render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
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
			out, err := render.New(render.WithSprig(false), render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
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
	out, err := render.New(render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
		Name: "t", Text: `{{ trim "  hi  " }}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "hi", out)
	assert.Empty(t, fake.calls)

	fake = &fakeReporter{}
	out, err = render.New(render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
		Name: "t2", Text: `{{ splitN "a,b,c" "," 2 }}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "[a b,c]", out)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, "splitN", fake.calls[0].deprecated)
}

// TestBareAliasShims_SliceBuildsAListWhenSprigDisabled verifies gomplate v3's
// bare `slice`: with Sprig disabled, v3 registered coll.Slice under that name,
// so `slice "a" "b"` built a list instead of reaching text/template's builtin.
func TestBareAliasShims_SliceBuildsAListWhenSprigDisabled(t *testing.T) {
	fake := &fakeReporter{}
	out, err := render.New(render.WithSprig(false), render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
		Name: "t", Text: `{{ slice "a" "b" | toJSON }}`,
	})
	require.NoError(t, err)
	assert.Equal(t, `["a","b"]`, out)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, "slice", fake.calls[0].deprecated)
	assert.Equal(t, "coll.Slice", fake.calls[0].replacement)
}

// TestBareAliasShims_SliceIsSprigsWhenSprigEnabled verifies the shim never
// replaces Sprig's `slice`, which indexes a list and won in v3 too because
// Sprig is layered after gomplate.
func TestBareAliasShims_SliceIsSprigsWhenSprigEnabled(t *testing.T) {
	fake := &fakeReporter{}
	out, err := render.New(render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
		Name: "t", Text: `{{ slice (list 1 2 3 4) 1 3 }}`,
	})
	require.NoError(t, err)
	assert.Equal(t, "[2 3]", out)
	assert.Empty(t, fake.calls, "Sprig's slice is not a deprecated name")
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
		_, err := render.New().Render(context.Background(), &render.Request{
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

// TestNamespaceShims_ApplyAlongsideDatasources verifies a template that
// mixes a namespace shim with a datasource read (here, `ds`) still gets the
// shim: the render engine assembles one function map that serves both.
func TestNamespaceShims_ApplyAlongsideDatasources(t *testing.T) {
	cfg := writeFixture(t, "cfg.yaml", "name: atmos\n")
	fake := &fakeReporter{}

	out, err := render.New(render.WithDeprecationReporter(fake)).Render(context.Background(), &render.Request{
		Name:        "combo",
		Text:        `{{ conv.Bool "yes" }}-{{ (ds "cfg").name }}`,
		Datasources: map[string]render.Datasource{"cfg": {URL: cfg}},
	})
	require.NoError(t, err)
	assert.Equal(t, "true-atmos", out)
	require.Len(t, fake.calls, 1)
	assert.Equal(t, "conv.Bool", fake.calls[0].deprecated)
}

// TestNetLegacyValuesMatchGomplateV3 renders the template compatibility
// corpus's net expressions; each expected value is gomplate v3's output for
// the same expression.
func TestNetLegacyValuesMatchGomplateV3(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"net-ip", `{{ printf "%T:%v" (net.ParseIP "192.0.2.1") (net.ParseIP "192.0.2.1") }}|{{ (net.ParseIP "192.0.2.1").As4 }}`, "netaddr.IP:192.0.2.1|[192 0 2 1]"},
		{"net-prefix", `{{ printf "%T:%v" (net.ParseIPPrefix "192.0.2.0/24") (net.ParseIPPrefix "192.0.2.0/24") }}`, "netaddr.IPPrefix:192.0.2.0/24"},
		{"net-range", `{{ printf "%T:%v" (net.ParseIPRange "192.0.2.1-192.0.2.3") (net.ParseIPRange "192.0.2.1-192.0.2.3") }}`, "netaddr.IPRange:192.0.2.1-192.0.2.3"},
		{"ip-chain-prior", `{{ (net.ParseIP "192.0.2.2").Prior.IPAddr }}`, "192.0.2.1"},
		{"ip-method-compare", `{{ (net.ParseIP "192.0.2.1").Compare (net.ParseIP "192.0.2.2") }}`, "-1"},
		{"ip-method-less", `{{ (net.ParseIP "192.0.2.1").Less (net.ParseIP "192.0.2.2") }}`, "true"},
		{"prefix-contains", `{{ (net.ParseIPPrefix "192.0.2.0/24").Contains (net.ParseIP "192.0.2.42") }}`, "true"},
		{"prefix-method-ip", `{{ (net.ParseIPPrefix "192.0.2.1/24").IP }}`, "192.0.2.1"},
		{"prefix-method-ipnet", `{{ (net.ParseIPPrefix "192.0.2.1/24").IPNet }}`, "192.0.2.1/24"},
		{"prefix-method-iszero", `{{ (net.ParseIPPrefix "192.0.2.1/24").IsZero }}`, "false"},
		{"prefix-method-range", `{{ (net.ParseIPPrefix "192.0.2.1/24").Range }}`, "192.0.2.0-192.0.2.255"},
		{"prefix-method-valid", `{{ (net.ParseIPPrefix "192.0.2.1/24").Valid }}`, "true"},
		{"range-contains", `{{ (net.ParseIPRange "192.0.2.1-192.0.2.3").Contains (net.ParseIP "192.0.2.2") }}`, "true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := render.New(render.WithDeprecationReporter(&fakeReporter{})).Render(context.Background(), &render.Request{
				Name: tt.name, Text: tt.text,
			})
			require.NoError(t, err)
			assert.Equal(t, tt.want, out)
		})
	}
}
