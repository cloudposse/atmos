package delimiters

import (
	"bytes"
	"testing"
	"text/template"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/edition"
	"github.com/cloudposse/atmos/pkg/schema"
)

func configWith(pin string, delims []string) *schema.AtmosConfiguration {
	cfg := &schema.AtmosConfiguration{Edition: pin}
	cfg.Templates.Settings.Delimiters = delims
	return cfg
}

func TestForSteps(t *testing.T) {
	custom := []string{"[[", "]]"}
	tests := []struct {
		name    string
		cfg     *schema.AtmosConfiguration
		want    Delimiters
		wantErr error
	}{
		{name: "nil config uses default", cfg: nil, want: Default()},
		{name: "unpinned with custom delimiters uses custom", cfg: configWith("", custom), want: Delimiters{Left: "[[", Right: "]]"}},
		{name: "pinned before the transition keeps default", cfg: configWith("2026-10-18", custom), want: Default()},
		{name: "pinned on the transition date uses custom", cfg: configWith("2026-10-19", custom), want: Delimiters{Left: "[[", Right: "]]"}},
		{name: "pinned after the transition uses custom", cfg: configWith("2026-12-01", custom), want: Delimiters{Left: "[[", Right: "]]"}},
		{name: "year pin before the transition keeps default", cfg: configWith("2025", custom), want: Default()},
		{name: "no delimiters configured uses default", cfg: configWith("", nil), want: Default()},
		{name: "explicit default pair uses default", cfg: configWith("", []string{"{{", "}}"}), want: Default()},
		{name: "invalid pin is an error", cfg: configWith("not-a-date", custom), wantErr: edition.ErrInvalidEdition},
		{name: "invalid configured pair is an error when honored", cfg: configWith("", []string{"[["}), wantErr: errUtils.ErrInvalidTemplateSettings},
		{name: "invalid configured pair is ignored under an old pin", cfg: configWith("2026-10-18", []string{"[["}), want: Default()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ForSteps(tt.cfg)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		name    string
		in      []string
		want    Delimiters
		wantErr bool
	}{
		{name: "nil selects default", in: nil, want: Default()},
		{name: "empty selects default", in: []string{}, want: Default()},
		{name: "pair", in: []string{"<<", ">>"}, want: Delimiters{Left: "<<", Right: ">>"}},
		{name: "single item", in: []string{"<<"}, wantErr: true},
		{name: "three items", in: []string{"<<", ">>", "x"}, wantErr: true},
		{name: "empty left", in: []string{"", ">>"}, wantErr: true},
		{name: "empty right", in: []string{"<<", ""}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Parse(tt.in)
			if tt.wantErr {
				require.ErrorIs(t, err, errUtils.ErrInvalidTemplateSettings)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestDelimitersHelpers(t *testing.T) {
	custom := Delimiters{Left: "[[", Right: "]]"}

	assert.True(t, Delimiters{}.IsDefault(), "the zero value is the default pair")
	assert.True(t, Default().IsDefault())
	assert.False(t, custom.IsDefault())
	assert.Equal(t, Default(), Delimiters{Left: "[["}.Normalize(), "a half-set pair falls back to the default")

	assert.True(t, custom.Contains("a [[ .x ]]"))
	assert.False(t, custom.Contains("a {{ .x }}"))
	assert.True(t, Default().Contains("a {{ .x }}"))
	assert.False(t, Default().Contains("a [[ .x ]]"))
	assert.True(t, Delimiters{}.Contains("{{"))

	assert.True(t, custom.ContainsAny("only ]] closes"))
	assert.False(t, custom.ContainsAny("only }} closes"))
	assert.True(t, Default().ContainsAny("only }} closes"))

	assert.Equal(t, "[[ .step.name ]]", custom.Wrap(".step.name"))
	assert.Equal(t, "{{ .step.name }}", Default().Wrap(".step.name"))
	assert.Equal(t, "{{ .step.name }}", Delimiters{}.Wrap(".step.name"))
}

func TestProtectRoundTrip(t *testing.T) {
	const placeholder = "\x00OPEN\x00"
	tests := []struct {
		name  string
		delim Delimiters
		in    string
		want  string
	}{
		{name: "custom replaces custom only", delim: Delimiters{Left: "[[", Right: "]]"}, in: "a [[ b {{ c", want: "a " + placeholder + " b {{ c"},
		{name: "default replaces default only", delim: Default(), in: "a [[ b {{ c", want: "a [[ b " + placeholder + " c"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			protected := tt.delim.Protect(tt.in, placeholder)
			assert.Equal(t, tt.want, protected)
			assert.Equal(t, tt.in, tt.delim.Unprotect(protected, placeholder))
		})
	}
}

func TestApply(t *testing.T) {
	render := func(d Delimiters, text string) string {
		tmpl, err := d.Apply(template.New("t")).Parse(text)
		require.NoError(t, err)
		var buf bytes.Buffer
		require.NoError(t, tmpl.Execute(&buf, map[string]string{"x": "ok"}))
		return buf.String()
	}
	assert.Equal(t, "ok {{ .x }}", render(Delimiters{Left: "[[", Right: "]]"}, "[[ .x ]] {{ .x }}"))
	assert.Equal(t, "ok [[ .x ]]", render(Default(), "{{ .x }} [[ .x ]]"))
	assert.Equal(t, "ok", render(Delimiters{}, "{{ .x }}"))
}
