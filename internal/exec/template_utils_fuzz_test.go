package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/schema"
)

// customDelimiterAtmosConfig returns an atmosConfig with non-default template
// delimiters configured, for exercising the delimiter-aware code path
// alongside the default-delimiter (nil) path both fuzz targets already cover.
func customDelimiterAtmosConfig() *schema.AtmosConfiguration {
	return &schema.AtmosConfiguration{
		Templates: schema.Templates{
			Settings: schema.TemplatesSettings{
				Delimiters: []string{"[[", "]]"},
			},
		},
	}
}

// FuzzProcessTmpl exercises Go template parsing and execution (Sprig/Gomplate/
// Atmos funcs included) with arbitrary, potentially malformed template strings
// to catch panics beyond the errors the parser/executor already returns. Each
// input is run against both the default (nil atmosConfig) and a non-default
// atmosConfig.Templates.Settings.Delimiters configuration, since a populated
// atmosConfig changes which func maps and code paths get exercised.
func FuzzProcessTmpl(f *testing.F) {
	f.Add("value: {{ invalid syntax }}")
	f.Add("value: {{ now")
	f.Add("value: {{ nonexistentfunc }}")
	f.Add("id: {{ uuidv4 }}")
	f.Add("token: {{ randAlphaNum 10 }}")
	f.Add("plain string, no template")
	f.Add("")
	f.Add("value: [[ invalid syntax ]]")
	f.Add("value: [[ now")
	f.Add("id: [[ uuidv4 ]]")

	f.Fuzz(func(t *testing.T, tmplValue string) {
		_, _ = ProcessTmpl(nil, "fuzz", tmplValue, nil, true)
		_, _ = ProcessTmpl(customDelimiterAtmosConfig(), "fuzz", tmplValue, nil, true)
	})
}

// FuzzIsGolangTemplate exercises the Go-template detection helper with
// arbitrary strings to catch panics in the template parser or node walk.
// Each input is run against both the default (nil atmosConfig) and a
// non-default atmosConfig.Templates.Settings.Delimiters configuration.
func FuzzIsGolangTemplate(f *testing.F) {
	f.Add("value: {{ invalid syntax }}")
	f.Add("value: {{ now")
	f.Add("plain string, no template")
	f.Add("{{ .Foo }}")
	f.Add("")
	f.Add("[[ .Foo ]]")
	f.Add("value: [[ invalid syntax ]]")

	f.Fuzz(func(t *testing.T, str string) {
		_, _ = IsGolangTemplate(nil, str)
		_, _ = IsGolangTemplate(customDelimiterAtmosConfig(), str)
	})
}
