package exec

import "testing"

// FuzzProcessTmpl exercises Go template parsing and execution (Sprig/Gomplate/
// Atmos funcs included) with arbitrary, potentially malformed template strings
// to catch panics beyond the errors the parser/executor already returns.
func FuzzProcessTmpl(f *testing.F) {
	f.Add("value: {{ invalid syntax }}")
	f.Add("value: {{ now")
	f.Add("value: {{ nonexistentfunc }}")
	f.Add("id: {{ uuidv4 }}")
	f.Add("token: {{ randAlphaNum 10 }}")
	f.Add("plain string, no template")
	f.Add("")

	f.Fuzz(func(t *testing.T, tmplValue string) {
		_, _ = ProcessTmpl(nil, "fuzz", tmplValue, nil, true)
	})
}

// FuzzIsGolangTemplate exercises the Go-template detection helper with
// arbitrary strings to catch panics in the template parser or node walk.
func FuzzIsGolangTemplate(f *testing.F) {
	f.Add("value: {{ invalid syntax }}")
	f.Add("value: {{ now")
	f.Add("plain string, no template")
	f.Add("{{ .Foo }}")
	f.Add("")

	f.Fuzz(func(t *testing.T, str string) {
		_, _ = IsGolangTemplate(nil, str)
	})
}
