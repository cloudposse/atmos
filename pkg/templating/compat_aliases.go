package templating

import (
	"strings"
	"text/template"

	"github.com/cloudposse/atmos/pkg/perf"
)

// bareAliasCandidates are the pipeline-unfriendly (subject-first) bare names
// gomplate v3 registered directly in the function map, wrapping the standard
// library's `strings` package with its native argument order. Sprig, when
// enabled, already registers the same names (with its own semantics), and
// Sprig is layered after gomplate in baseFuncs, so in v3 the effective
// function for these names was already Sprig's whenever Sprig was enabled;
// nothing changes there. Only when Sprig is disabled does gomplate v5's
// removal of these names change behavior, so that's the only case this shim
// needs to cover.
//
// "slice" is deliberately not in this list: text/template has provided a
// builtin `slice` function since Go 1.13, so leaving the name unregistered
// means callers keep using that builtin exactly as before. Registering our
// own copy here would shadow the builtin with gomplate's different
// (variadic-args-to-list) semantics, which is the one thing this file must
// never do.
var bareAliasCandidates = []string{"contains", "hasPrefix", "hasSuffix", "split", "trim"}

// applyBareAliasShims registers the v3 bare aliases gomplate v5 removed,
// after Sprig has already been layered into funcs, so a name Sprig already
// provides is left untouched. Call names and hints all point at the
// pipeline-friendly namespaced replacement and, since Sprig registers
// equivalent names, at Sprig's version too.
func applyBareAliasShims(funcs template.FuncMap, name string, reporter DeprecationReporter) {
	defer perf.Track(nil, "templating.applyBareAliasShims")()

	if _, ok := funcs["splitN"]; !ok {
		funcs["splitN"] = bareSplitN(name, reporter)
	}

	for _, alias := range bareAliasCandidates {
		if _, ok := funcs[alias]; ok {
			continue // Sprig (or a caller) already provides this name.
		}
		if shim := bareAliasShim(alias, name, reporter); shim != nil {
			funcs[alias] = shim
		}
	}
}

// bareAliasHint builds the standard hint sentence for a bare v3 alias,
// naming both the namespaced gomplate replacement and Sprig's function of
// the same name as alternatives.
func bareAliasHint(alias, namespaced string) string {
	return "the bare `" + alias + "` alias was removed in gomplate v5. Atmos still accepts it, but this shim will be removed in a future major release. " +
		"Replace `" + alias + "` with `" + namespaced + "` (note the argument order is reversed for pipelining), or enable Sprig, which provides its own `" + alias + "`."
}

// bareSplitN is the v3 bare `splitN` alias: gomplate v5 removed it, and
// unlike the other bare aliases, Sprig doesn't provide it either (Sprig's
// equivalent is the differently-cased, differently-shaped `splitn`), so it's
// always registered when absent, independent of whether Sprig is enabled.
func bareSplitN(name string, reporter DeprecationReporter) func(s, sep string, n int) []string {
	return func(s, sep string, n int) []string {
		reporter.Deprecated(name, "splitN", "strings.SplitN",
			"the bare `splitN` alias was removed in gomplate v5. Atmos still accepts it, but this shim will be removed in a future major release. "+
				"Replace `splitN s sep n` with `strings.SplitN sep n s` (note the argument order is reversed for pipelining).")
		return strings.SplitN(s, sep, n)
	}
}

// bareAliasShim returns the v3-argument-order function for one of
// bareAliasCandidates, or nil if alias isn't one of them.
func bareAliasShim(alias, name string, reporter DeprecationReporter) any {
	switch alias {
	case "contains":
		return func(s, substr string) bool {
			reporter.Deprecated(name, "contains", "strings.Contains", bareAliasHint("contains", "strings.Contains"))
			return strings.Contains(s, substr)
		}
	case "hasPrefix":
		return func(s, prefix string) bool {
			reporter.Deprecated(name, "hasPrefix", "strings.HasPrefix", bareAliasHint("hasPrefix", "strings.HasPrefix"))
			return strings.HasPrefix(s, prefix)
		}
	case "hasSuffix":
		return func(s, suffix string) bool {
			reporter.Deprecated(name, "hasSuffix", "strings.HasSuffix", bareAliasHint("hasSuffix", "strings.HasSuffix"))
			return strings.HasSuffix(s, suffix)
		}
	case "split":
		return func(s, sep string) []string {
			reporter.Deprecated(name, "split", "strings.Split", bareAliasHint("split", "strings.Split"))
			return strings.Split(s, sep)
		}
	case "trim":
		return func(s, cutset string) string {
			reporter.Deprecated(name, "trim", "strings.Trim", bareAliasHint("trim", "strings.Trim"))
			return strings.Trim(s, cutset)
		}
	default:
		return nil
	}
}
