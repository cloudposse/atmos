// Package templating compat.go keeps gomplate v3 function names that gomplate
// v4/v5 removed or renamed working: templates that still call them execute
// exactly as before, and a DeprecationReporter is notified (once per template
// per name) so the removal is visible without breaking the render. See
// compat_aliases.go for the bare (non-namespaced) v3 aliases and
// compat_lint.go for the static checks that warn about datasource behavior
// gomplate v5 changed without removing a name.
package templating

import (
	"fmt"
	stdnet "net"
	"net/netip"
	"net/url"
	"reflect"
	"sync"
	"text/template"

	"go4.org/netipx"

	errUtils "github.com/cloudposse/atmos/errors"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DeprecationReporter is notified when a template uses a gomplate v3 name
// this package keeps working through a shim. Implementations must be safe
// for concurrent use: stack processing renders many templates concurrently.
type DeprecationReporter interface {
	// Deprecated reports that templateName called deprecated, which this
	// package now serves by delegating to replacement. hint is a complete,
	// self-contained sentence naming the exact replacement; it is written to
	// be shown to a user as-is.
	Deprecated(templateName, deprecated, replacement, hint string)
}

// logDeprecationReporter logs a warning the first time a given
// (templateName, deprecated) pair is seen and is a no-op after that. Without
// this dedup, a deprecated call in a catalog template shared by every stack
// would log once per stack per component, which is hundreds of identical
// lines in a normal repository.
type logDeprecationReporter struct {
	seen sync.Map
}

func newLogDeprecationReporter() *logDeprecationReporter {
	defer perf.Track(nil, "templating.newLogDeprecationReporter")()

	return &logDeprecationReporter{}
}

// Deprecated implements DeprecationReporter.
func (r *logDeprecationReporter) Deprecated(templateName, deprecated, replacement, hint string) {
	defer perf.Track(nil, "templating.logDeprecationReporter.Deprecated")()

	key := templateName + "\x00" + deprecated
	if _, loaded := r.seen.LoadOrStore(key, struct{}{}); loaded {
		return
	}
	log.Warn("template uses a function removed from gomplate v5; a compatibility shim is serving it for now",
		"template", templateName, "deprecated", deprecated, "replacement", replacement, "hint", hint)
}

// defaultDeprecationReporter is shared by every Engine that does not set its
// own reporter, so dedup holds for the lifetime of the process, matching how
// defaultDatasourceCache is shared.
var defaultDeprecationReporter = newLogDeprecationReporter()

// WithDeprecationReporter overrides the DeprecationReporter used to report
// gomplate v3 names kept alive by this package's compatibility shims. The
// default logs a warning via pkg/logger, deduplicated per template per name.
func WithDeprecationReporter(reporter DeprecationReporter) Option {
	defer perf.Track(nil, "templating.WithDeprecationReporter")()

	return func(e *engine) {
		if reporter != nil {
			e.reporter = reporter
		}
	}
}

// deprecationHint builds the standard "was removed / still works / replace
// with" sentence used by every namespaced shim in this file.
func deprecationHint(deprecated, replacement string) string {
	return fmt.Sprintf(
		"%s was removed in gomplate v5. Atmos still accepts it, but this shim will be removed in a future major release. Replace `%s` with `%s`.",
		deprecated, deprecated, replacement,
	)
}

// deprecationHintTypeChanged is deprecationHint plus a note that the return
// type changed, for the net.ParseIPPrefix/ParseIPRange shims.
func deprecationHintTypeChanged(deprecated, replacement, newType string) string {
	return fmt.Sprintf(
		"%s was removed in gomplate v5. Atmos still accepts it, but this shim will be removed in a future major release. Replace `%s` with `%s`; note the return type changed to `%s`.",
		deprecated, deprecated, replacement, newType,
	)
}

// wrapConversionError turns a gomplate v5 conversion error into one carrying
// a hint that explains the v3 behavior it replaces. Gomplate v3's
// conv.ToInt64/ToFloat64/Atoi/ToInt silently returned the zero value for
// unconvertible input; v5 returns an error instead, which is a correctness
// improvement Atmos does not undo, but callers written against the old
// behavior need a clear pointer to what changed.
func wrapConversionError(err error, fn string) error {
	if err == nil {
		return nil
	}
	hint := fmt.Sprintf(
		"%s no longer falls back to a zero value on unconvertible input in gomplate v5 (%s); guard the call with `if` (or provide a default) before converting.",
		fn, err.Error(),
	)
	return fmt.Errorf("%w: %s", errUtils.ErrTemplateConversion, hint)
}

// namespaceValue looks up a zero-arg gomplate namespace factory function
// (e.g. funcs["conv"]) and returns the namespace object it produces, asserted
// to T. Gomplate registers every namespace this way: internal/funcs/*.go
// register `f["conv"] = func() any { return ns }`, where ns's concrete type
// (unexported, defined in gomplate's internal/funcs package) satisfies T
// structurally as long as T lists a subset of its methods with matching
// signatures.
func namespaceValue[T any](funcs template.FuncMap, key string) (T, bool) {
	var zero T
	raw, ok := funcs[key]
	if !ok {
		return zero, false
	}
	factory, ok := raw.(func() any)
	if !ok {
		return zero, false
	}
	value, ok := factory().(T)
	return value, ok
}

// convNamespace lists every exported method of gomplate v5's conv namespace
// (internal/funcs.ConvFuncs). Embedding it in convWrapper promotes all of
// them, so the wrapper stays current with upstream without listing methods
// it doesn't change; TestNamespaceWrappersCoverUpstreamMethods (compat_test.go)
// fails if upstream adds or removes a method here.
type convNamespace interface {
	ToBool(in any) bool
	ToBools(in ...any) []bool
	Join(in any, sep string) (string, error)
	ParseInt(s any, base, bitSize int) (int64, error)
	ParseFloat(s any, bitSize int) (float64, error)
	ParseUint(s any, base, bitSize int) (uint64, error)
	Atoi(s any) (int, error)
	URL(s any) (*url.URL, error)
	ToInt64(in any) (int64, error)
	ToInt(in any) (int, error)
	ToInt64s(in ...any) ([]int64, error)
	ToInts(in ...any) ([]int, error)
	ToFloat64(in any) (float64, error)
	ToFloat64s(in ...any) ([]float64, error)
	ToString(in any) string
	ToStrings(in ...any) []string
	Default(def, in any) any
}

// collNamespace lists every exported method of gomplate v5's coll namespace
// (internal/funcs.CollFuncs). The conv and strings shims delegate their
// removed methods (Slice, Dict, Has, Sort) to it, obtained from the live
// gomplate function map rather than called directly, so the coverage test can
// verify it the same way as the namespaces this file wraps for warnings.
type collNamespace interface {
	Slice(args ...any) []any
	GoSlice(item reflect.Value, indexes ...reflect.Value) (reflect.Value, error)
	Has(in any, key string) bool
	Index(args ...any) (any, error)
	Dict(in ...any) (map[string]any, error)
	Keys(in ...map[string]any) ([]string, error)
	Values(in ...map[string]any) ([]any, error)
	Append(v any, list any) ([]any, error)
	Prepend(v any, list any) ([]any, error)
	Uniq(in any) ([]any, error)
	Reverse(in any) ([]any, error)
	Merge(dst map[string]any, src ...map[string]any) (map[string]any, error)
	Sort(args ...any) ([]any, error)
	JSONPath(p string, in any) (any, error)
	JQ(jqExpr string, in any) (any, error)
	Flatten(args ...any) ([]any, error)
	Pick(args ...any) (map[string]any, error)
	Omit(args ...any) (any, error)
	Set(key string, value any, m map[string]any) (map[string]any, error)
	Unset(key string, m map[string]any) (map[string]any, error)
}

// stringsNamespace lists every exported method of gomplate v5's strings
// namespace (internal/funcs.StringFuncs). See convNamespace for why it's an
// embedded interface rather than a struct.
type stringsNamespace interface {
	Abbrev(args ...any) (string, error)
	ReplaceAll(old, newStr string, s any) string
	Contains(substr string, s any) bool
	HasPrefix(prefix string, s any) bool
	HasSuffix(suffix string, s any) bool
	Repeat(count int, s any) (string, error)
	SkipLines(skip int, in string) (string, error)
	Split(sep string, s any) []string
	SplitN(sep string, n int, s any) []string
	Trim(cutset string, s any) string
	TrimLeft(cutset string, s any) string
	TrimPrefix(cutset string, s any) string
	TrimRight(cutset string, s any) string
	TrimSuffix(cutset string, s any) string
	Title(s any) string
	ToUpper(s any) string
	ToLower(s any) string
	TrimSpace(s any) string
	Trunc(length int, s any) string
	Indent(args ...any) (string, error)
	Slug(in any) string
	Quote(in any) string
	ShellQuote(in any) string
	Squote(in any) string
	SnakeCase(in any) (string, error)
	CamelCase(in any) (string, error)
	KebabCase(in any) (string, error)
	WordWrap(args ...any) (string, error)
	RuneCount(args ...any) (int, error)
}

// netNamespace lists every exported method of gomplate v5's net namespace
// (internal/funcs.NetFuncs). See convNamespace for why it's an embedded
// interface rather than a struct.
type netNamespace interface {
	LookupIP(name any) (string, error)
	LookupIPs(name any) ([]string, error)
	LookupCNAME(name any) (string, error)
	LookupSRV(name any) (*stdnet.SRV, error)
	LookupSRVs(name any) ([]*stdnet.SRV, error)
	LookupTXT(name any) ([]string, error)
	ParseAddr(ip any) (netip.Addr, error)
	ParsePrefix(ipprefix any) (netip.Prefix, error)
	ParseRange(iprange any) (netipx.IPRange, error)
	CIDRHost(hostnum any, prefix any) (netip.Addr, error)
	CIDRNetmask(prefix any) (netip.Addr, error)
	CIDRSubnets(newbits any, prefix any) ([]netip.Prefix, error)
	CIDRSubnetSizes(args ...any) ([]netip.Prefix, error)
}

// convWrapper serves the current conv namespace unchanged (via the embedded
// interface) plus the v3 names gomplate v5 removed.
type convWrapper struct {
	convNamespace
	coll     collNamespace
	name     string
	reporter DeprecationReporter
}

// Bool is gomplate v3's conv.Bool, removed in v5 in favor of ToBool.
func (w *convWrapper) Bool(s any) bool {
	defer perf.Track(nil, "templating.convWrapper.Bool")()

	w.reporter.Deprecated(w.name, "conv.Bool", "conv.ToBool", deprecationHint("conv.Bool", "conv.ToBool"))
	return w.ToBool(s)
}

// Slice is gomplate v3's conv.Slice, removed in v5 in favor of coll.Slice.
func (w *convWrapper) Slice(args ...any) []any {
	defer perf.Track(nil, "templating.convWrapper.Slice")()

	w.reporter.Deprecated(w.name, "conv.Slice", "coll.Slice", deprecationHint("conv.Slice", "coll.Slice"))
	return w.coll.Slice(args...)
}

// Dict is gomplate v3's conv.Dict, removed in v5 in favor of coll.Dict.
func (w *convWrapper) Dict(in ...any) (map[string]any, error) {
	defer perf.Track(nil, "templating.convWrapper.Dict")()

	w.reporter.Deprecated(w.name, "conv.Dict", "coll.Dict", deprecationHint("conv.Dict", "coll.Dict"))
	return w.coll.Dict(in...)
}

// Has is gomplate v3's conv.Has, removed in v5 in favor of coll.Has.
func (w *convWrapper) Has(in any, key string) bool {
	defer perf.Track(nil, "templating.convWrapper.Has")()

	w.reporter.Deprecated(w.name, "conv.Has", "coll.Has", deprecationHint("conv.Has", "coll.Has"))
	return w.coll.Has(in, key)
}

// ToInt64 overrides the embedded method to add a hint when gomplate v5's
// error-returning behavior (instead of v3's silent zero) surprises a caller.
func (w *convWrapper) ToInt64(in any) (int64, error) {
	defer perf.Track(nil, "templating.convWrapper.ToInt64")()

	v, err := w.convNamespace.ToInt64(in)
	return v, wrapConversionError(err, "conv.ToInt64")
}

// ToInt overrides the embedded method; see ToInt64.
func (w *convWrapper) ToInt(in any) (int, error) {
	defer perf.Track(nil, "templating.convWrapper.ToInt")()

	v, err := w.convNamespace.ToInt(in)
	return v, wrapConversionError(err, "conv.ToInt")
}

// ToFloat64 overrides the embedded method; see ToInt64.
func (w *convWrapper) ToFloat64(in any) (float64, error) {
	defer perf.Track(nil, "templating.convWrapper.ToFloat64")()

	v, err := w.convNamespace.ToFloat64(in)
	return v, wrapConversionError(err, "conv.ToFloat64")
}

// Atoi overrides the embedded method; see ToInt64.
func (w *convWrapper) Atoi(in any) (int, error) {
	defer perf.Track(nil, "templating.convWrapper.Atoi")()

	v, err := w.convNamespace.Atoi(in)
	return v, wrapConversionError(err, "conv.Atoi")
}

// stringsWrapper serves the current strings namespace unchanged (via the
// embedded interface) plus the v3 names gomplate v5 removed.
type stringsWrapper struct {
	stringsNamespace
	coll     collNamespace
	name     string
	reporter DeprecationReporter
}

// Sort is gomplate v3's strings.Sort, removed in v5 in favor of coll.Sort.
func (w *stringsWrapper) Sort(args ...any) ([]any, error) {
	defer perf.Track(nil, "templating.stringsWrapper.Sort")()

	w.reporter.Deprecated(w.name, "strings.Sort", "coll.Sort", deprecationHint("strings.Sort", "coll.Sort"))
	return w.coll.Sort(args...)
}

// netWrapper serves the current net namespace unchanged (via the embedded
// interface) plus the v3 names gomplate v5 removed.
type netWrapper struct {
	netNamespace
	name     string
	reporter DeprecationReporter
}

// ParseIP is gomplate v3's net.ParseIP, removed in v5 in favor of
// net.ParseAddr. V3 returned an inet.af/netaddr IP; netip.Addr is that type's
// standard-library successor with the same method names (Is4, Is6, String,
// IsPrivate, ...), so delegating to ParseAddr keeps existing templates that
// inspect the result working, and invalid input errors exactly as v3 did.
func (w *netWrapper) ParseIP(ip any) (netip.Addr, error) {
	defer perf.Track(nil, "templating.netWrapper.ParseIP")()

	w.reporter.Deprecated(w.name, "net.ParseIP", "net.ParseAddr", deprecationHint("net.ParseIP", "net.ParseAddr"))
	return w.ParseAddr(ip)
}

// ParseIPPrefix is gomplate v3's net.ParseIPPrefix, removed in v5 in favor of
// net.ParsePrefix, which returns netip.Prefix instead of v3's netaddr.IPPrefix.
func (w *netWrapper) ParseIPPrefix(ipprefix any) (netip.Prefix, error) {
	defer perf.Track(nil, "templating.netWrapper.ParseIPPrefix")()

	w.reporter.Deprecated(w.name, "net.ParseIPPrefix", "net.ParsePrefix",
		deprecationHintTypeChanged("net.ParseIPPrefix", "net.ParsePrefix", "netip.Prefix"))
	return w.ParsePrefix(ipprefix)
}

// ParseIPRange is gomplate v3's net.ParseIPRange, removed in v5 in favor of
// net.ParseRange, which returns netipx.IPRange instead of v3's netaddr.IPRange.
func (w *netWrapper) ParseIPRange(iprange any) (netipx.IPRange, error) {
	defer perf.Track(nil, "templating.netWrapper.ParseIPRange")()

	w.reporter.Deprecated(w.name, "net.ParseIPRange", "net.ParseRange",
		deprecationHintTypeChanged("net.ParseIPRange", "net.ParseRange", "netipx.IPRange"))
	return w.ParseRange(iprange)
}

// applyNamespaceShims replaces the conv, strings and net entries of funcs
// (already populated from gomplateFuncs) with wrappers that keep the v3
// method names gomplate v5 removed working. It is a no-op for any namespace
// gomplateFuncs doesn't provide (gomplate disabled) or whose factory shape
// doesn't match what this file expects (an upstream gomplate change), in
// which case templates simply see whatever gomplate itself registered.
func applyNamespaceShims(funcs, gomplateFuncs template.FuncMap, name string, reporter DeprecationReporter) {
	defer perf.Track(nil, "templating.applyNamespaceShims")()

	coll, ok := namespaceValue[collNamespace](gomplateFuncs, "coll")
	if !ok {
		return
	}

	if conv, ok := namespaceValue[convNamespace](gomplateFuncs, "conv"); ok {
		wrapper := &convWrapper{convNamespace: conv, coll: coll, name: name, reporter: reporter}
		funcs["conv"] = func() any { return wrapper }
	}

	if strs, ok := namespaceValue[stringsNamespace](gomplateFuncs, "strings"); ok {
		wrapper := &stringsWrapper{stringsNamespace: strs, coll: coll, name: name, reporter: reporter}
		funcs["strings"] = func() any { return wrapper }
	}

	if net, ok := namespaceValue[netNamespace](gomplateFuncs, "net"); ok {
		wrapper := &netWrapper{netNamespace: net, name: name, reporter: reporter}
		funcs["net"] = func() any { return wrapper }
	}
}
