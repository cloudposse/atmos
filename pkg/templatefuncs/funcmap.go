// Package templatefuncs holds Go text/template functions Atmos authors
// itself (as opposed to Sprig's or Gomplate's), meant to be registered
// alongside them in every FuncMap Atmos builds -- scaffold templates, stack
// config templates, locals, secrets/store reference templates, and
// toolchain asset templates alike.
package templatefuncs

import "github.com/cloudposse/atmos/pkg/perf"

// FuncMap returns every template function this package exports, keyed by
// the name it's registered under. Callers merge it into a text/template
// FuncMap the same way they merge Sprig's or Gomplate's.
func FuncMap() map[string]any {
	defer perf.Track(nil, "templatefuncs.FuncMap")()

	return map[string]any{
		"collectKeys": CollectKeys,
	}
}
