package main

import (
	"fmt"
	"runtime"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func TestEnvironmentFormattingRejectsBeforeLargeAllocation(t *testing.T) {
	source := `{{ printf "` + strings.Repeat("%1000000s", 32) + `" ` + strings.Repeat(`"secret" `, 32) + `}}`
	templates, err := environmentTemplates(&pluginv1.IdentityRequest{Spec: object(t, map[string]any{"env": map[string]any{"TOKEN": source}})})
	require.NoError(t, err)
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	_, err = renderEnvironment(templates, nil)
	runtime.ReadMemStats(&after)
	require.Error(t, err)
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(8*maxEnvironmentValueBytes), "reject expansion before fmt allocates its full result")
}

func TestBoundedPrintfPreservesScalarFormatting(t *testing.T) {
	for _, test := range []struct {
		format string
		args   []any
	}{
		{"Bearer %s:%s", []any{"access", "secret"}},
		{"%[2]s:%[1]s", []any{"secret", "access"}},
		{"%[3]*.[2]*[1]f", []any{3.14159, 2, 8}},
		{"%*s", []any{-1, "x"}},
		{"%*s", []any{uint8(4), "x"}},
		{"%*s", []any{-8, "secret"}},
		{"%.s/%.*s", []any{"secret", 2, "secret"}},
		{"%#08x/%+d/%t", []any{uint64(23), int32(3), true}},
		{"%.2s/%8s/%q/%#v/% x/%# x", []any{"你好世界", "日本", "\n\x00", "secret", "ab", "ab"}},
		{"%.2x/%.20s", []any{"secret", "secret"}},
		{"%T/%T", []any{"secret", 1.0}},
		{"%e/%E/%g/%f/%q/%b", []any{1.2, 1.2, 1.2, complex(1.2, 3.4), 'a', uint8(8)}},
		{"literal %% suffix", nil},
		{"", nil},
	} {
		t.Run(test.format, func(t *testing.T) {
			result, err := boundedPrintf(test.format, test.args...)
			require.NoError(t, err)
			require.Equal(t, fmt.Sprintf(test.format, test.args...), result)
		})
	}
	large := strings.Repeat("x", maxEnvironmentValueBytes)
	result, err := boundedPrintf("%s", large)
	require.NoError(t, err)
	require.Equal(t, large, result)
}

func TestBoundedPrintfRejectsUnsafeOrInvalidFormats(t *testing.T) {
	for _, test := range []struct {
		format string
		args   []any
	}{
		{"%1000000000s", []any{"secret"}},
		{"%*s", []any{maxEnvironmentValueBytes + 1, "secret"}},
		{"%*s", []any{uint64(maxEnvironmentValueBytes + 1), "secret"}},
		{"%*s", []any{-maxEnvironmentValueBytes - 1, "secret"}},
		{"%.*s", []any{maxEnvironmentValueBytes + 1, "secret"}},
		{"%.*s", []any{-1, "secret"}},
		{"%*s", []any{"not-width", "secret"}},
		{"%*s", []any{nil, "secret"}},
		{"%*s", nil},
		{"%.[2]*[1]s", []any{"secret"}},
		{"%[0]s", []any{"secret"}},
		{"%[2]s", []any{"secret"}},
		{"%[1s", []any{"secret"}},
		{"%[999999999999]s", []any{"secret"}},
		{"%", nil},
		{"%s", nil},
		{"%s", []any{map[string]string{"secret": "value"}}},
		{"%w", []any{1}},
		{"%d", []any{"secret"}},
		{"%q", []any{strings.Repeat("\x01", maxEnvironmentValueBytes/2)}},
		{"%s%s", []any{strings.Repeat("x", maxEnvironmentValueBytes), "x"}},
		{"%s", []any{"nul\x00byte"}},
		{"literal\x00", nil},
		{strings.Repeat("x", maxEnvironmentValueBytes+1), nil},
	} {
		t.Run(fmt.Sprintf("case_%d_%s", len(test.format), test.format[:min(30, len(test.format))]), func(t *testing.T) {
			_, err := boundedPrintf(test.format, test.args...)
			require.ErrorIs(t, err, errEnvironmentTemplate)
		})
	}
}

func TestEnvironmentOutputFunctionsMatchGoTemplates(t *testing.T) {
	for _, test := range []struct {
		source string
		want   string
	}{
		{`{{ printf "%[2]s:%[1]s" .credentials.secret "Bearer" }}`, "Bearer:private"},
		{`{{ .credentials.secret | printf .credentials.format }}`, "private "},
		{`{{ print "Bearer " .credentials.secret 2 3 }}`, "Bearer private2 3"},
		{`{{ println "Bearer" .credentials.secret 2 3 }}`, "Bearer private 2 3\n"},
		{`{{ html .credentials.secret "<&" }}`, "private&lt;&amp;"},
		{`{{ js .credentials.secret "<'" }}`, `private\u003C\'`},
		{`{{ urlquery .credentials.secret " 日本/?~" }}`, "private+%E6%97%A5%E6%9C%AC%2F%3F~"},
	} {
		t.Run(test.source, func(t *testing.T) {
			templates, err := environmentTemplates(&pluginv1.IdentityRequest{Spec: object(t, map[string]any{"env": map[string]any{"TOKEN": test.source}})})
			require.NoError(t, err)
			result, err := renderEnvironment(templates, map[string]any{"secret": "private", "format": "%-8s"})
			require.NoError(t, err)
			require.Equal(t, test.want, result["TOKEN"])
		})
	}
}

func TestEnvironmentOutputFunctionsRejectExpansion(t *testing.T) {
	_, err := boundedPrint(true, strings.Repeat("x", maxEnvironmentValueBytes))
	require.ErrorIs(t, err, errEnvironmentTemplate)
	for _, function := range []string{"print", "println", "html", "js", "urlquery"} {
		t.Run(function, func(t *testing.T) {
			_, err := environmentFunctions()[function].(func(...any) (string, error))(strings.Repeat("'", maxEnvironmentValueBytes), "x")
			require.ErrorIs(t, err, errEnvironmentTemplate)
			_, err = environmentFunctions()[function].(func(...any) (string, error))(map[string]string{"unsupported": "private"})
			require.ErrorIs(t, err, errEnvironmentTemplate)
		})
	}
	for _, function := range []string{"html", "js", "urlquery"} {
		t.Run(function+"_escaping_expansion", func(t *testing.T) {
			_, err := environmentFunctions()[function].(func(...any) (string, error))(strings.Repeat("'", maxEnvironmentValueBytes))
			require.ErrorIs(t, err, errEnvironmentTemplate)
		})
	}
}

func TestEnvironmentOutputActionsCheckValuesBeforeFormatting(t *testing.T) {
	for _, source := range []string{
		`{{ . }}`, `{{ .credentials }}`, `{{ $values := .credentials }}{{ $values }}`,
		`{{ with .credentials }}{{ . }}{{ end }}`,
		`{{ if .credentials }}{{ .credentials }}{{ else }}literal{{ end }}`,
		`{{ define "nested" }}{{ . }}{{ end }}{{ template "nested" .credentials }}`,
		`{{ .credentials.oversized }}`,
	} {
		t.Run(source, func(t *testing.T) {
			templates, err := environmentTemplates(&pluginv1.IdentityRequest{Spec: object(t, map[string]any{"env": map[string]any{"TOKEN": source}})})
			require.NoError(t, err)
			result, err := renderEnvironment(templates, map[string]any{"oversized": strings.Repeat("x", maxEnvironmentValueBytes+1)})
			require.Error(t, err)
			require.Nil(t, result)
		})
	}
	for _, source := range []string{
		`{{ $values := .credentials }}{{ $values.Format }}`,
		`{{ with .credentials }}{{ .Format }}{{ else }}unused{{ end }}`,
		`{{ range .credentials }}{{ . }}{{ else }}unused{{ end }}`,
		`{{ if false }}unused{{ else }}{{ .credentials.Format }}{{ end }}`,
		`{{ define "nested" }}{{ .Format }}{{ end }}{{ template "nested" .credentials }}`,
	} {
		t.Run(source, func(t *testing.T) {
			templates, err := environmentTemplates(&pluginv1.IdentityRequest{Spec: object(t, map[string]any{"env": map[string]any{"TOKEN": source}})})
			require.NoError(t, err)
			result, err := renderEnvironment(templates, map[string]any{"Format": "private"})
			require.NoError(t, err)
			require.Equal(t, "private", result["TOKEN"])
		})
	}
}
