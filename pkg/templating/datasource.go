package templating

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hairyhenderson/gomplate/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DatasourceReader reads a named gomplate datasource, optionally with
// datasource-specific arguments (for example a sub-path).
type DatasourceReader interface {
	Datasource(alias string, args ...string) (any, error)
}

// DatasourceCache memoizes datasource reads by alias and arguments so that a
// datasource referenced many times across a stack is fetched once.
type DatasourceCache struct {
	entries sync.Map
}

// NewDatasourceCache creates an empty DatasourceCache.
func NewDatasourceCache() *DatasourceCache {
	defer perf.Track(nil, "templating.NewDatasourceCache")()

	return &DatasourceCache{}
}

// defaultDatasourceCache is shared by every Engine that does not set its own,
// preserving the process-wide caching users rely on.
var defaultDatasourceCache = NewDatasourceCache()

// cacheKeySep separates the fields folded into a datasource cache key. It is
// a control character, so it can't collide with an alias, URL, header, or
// argument value.
const cacheKeySep = "\x00"

// cacheKey builds the datasource cache key from the alias, its effective
// definition (URL and headers) and the call arguments. The definition is part
// of the key because datasources are defined per stack manifest
// (settings.templates.settings.gomplate.datasources): two stacks can define
// the same alias with different URLs or headers, and without the definition
// in the key one stack's cached value would leak into the other's render.
func cacheKey(alias string, ds Datasource, args []string) string {
	return alias + cacheKeySep + ds.URL + cacheKeySep + headerKey(ds.Headers) + cacheKeySep + strings.Join(args, cacheKeySep)
}

// headerKey deterministically renders headers (sorted keys, sorted values per
// key) so that two Datasource definitions differing only in header order
// produce the same key, while differing header values produce different keys.
func headerKey(headers map[string][]string) string {
	if len(headers) == 0 {
		return ""
	}
	keys := make([]string, 0, len(headers))
	for k := range headers {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var b strings.Builder
	for _, k := range keys {
		values := append([]string(nil), headers[k]...)
		sort.Strings(values)
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(strings.Join(values, ","))
		b.WriteByte(';')
	}
	return b.String()
}

// load returns the cached value for key, if any.
func (c *DatasourceCache) load(key string) (any, bool) {
	return c.entries.Load(key)
}

// store caches value under key.
func (c *DatasourceCache) store(key string, value any) {
	c.entries.Store(key, value)
}

// Datasource returns the parsed value of the datasource registered under alias
// for the render in progress. It can only be called from a template function
// while Render is executing that template through gomplate's renderer; Render
// routes any template that references atmos.GomplateDatasource there. A live
// render is required before the cache is even consulted, both because a
// value can only be produced by rendering and because the cache key itself is
// built from the live render's datasource definitions.
func (e *engine) Datasource(alias string, args ...string) (any, error) {
	defer perf.Track(nil, "templating.Engine.Datasource")()

	state := e.live.get()
	if state == nil || state.tmpl == nil {
		return nil, errUtils.ErrGomplateDatasourceUnavailable
	}

	// An alias not present in the render's datasource map is one defined
	// inline via defineDatasource; key on alias+args with an empty definition.
	ds := state.datasources[alias]
	key := cacheKey(alias, ds, args)
	if value, ok := e.cache.load(key); ok && value != nil {
		return value, nil
	}

	call := make([]string, 0, len(args)+1)
	call = append(call, fmt.Sprintf("%q", alias))
	for _, arg := range args {
		call = append(call, fmt.Sprintf("%q", arg))
	}
	text := state.left + " " + captureFuncName + " (ds " + strings.Join(call, " ") + ") " + state.right

	state.captured = nil
	if _, err := state.tmpl.Inline("__atmos_datasource", text, nil); err != nil {
		return nil, err
	}

	value := state.captured
	if value != nil {
		e.cache.store(key, value)
	}
	return value, nil
}

// toDataSources converts datasource definitions to gomplate's representation.
func toDataSources(sources map[string]Datasource) (map[string]gomplate.DataSource, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	out := make(map[string]gomplate.DataSource, len(sources))
	for alias, source := range sources {
		if alias == "" {
			return nil, fmt.Errorf("%w: datasource alias must not be empty", errUtils.ErrInvalidDatasourceURL)
		}
		parsed, err := parseDatasourceURL(source.URL)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrInvalidDatasourceURL, alias, err)
		}
		ds := gomplate.DataSource{URL: parsed}
		if len(source.Headers) > 0 {
			ds.Header = http.Header(source.Headers)
		}
		out[alias] = ds
	}
	return out, nil
}

// parseDatasourceURL parses a datasource URL the way gomplate's CLI does: "-"
// means stdin, Windows drive and UNC paths become file URLs, and a bare
// absolute path with no scheme is a file URL.
func parseDatasourceURL(value string) (*url.URL, error) {
	if value == "-" {
		value = "stdin://"
	}
	value, hasVolume := windowsVolumeToFileURL(filepath.ToSlash(value))

	parsed, err := url.Parse(value)
	if err != nil {
		return nil, err
	}

	if hasVolume && len(parsed.Path) >= 3 && parsed.Path[0] == '/' && parsed.Path[2] == ':' {
		// Drop the leading slash url.Parse keeps in front of the drive letter.
		parsed.Path = parsed.Path[1:]
	}

	if parsed.Scheme == "" && path.IsAbs(parsed.Path) {
		parsed.Scheme = "file"
	}

	return parsed, nil
}

// windowsVolumeToFileURL prefixes a value that starts with a Windows volume
// (drive letter or UNC share) with the file scheme, and reports whether it did.
func windowsVolumeToFileURL(value string) (string, bool) {
	volName := filepath.VolumeName(value)
	if volName == "" {
		return value, false
	}
	if len(volName) > 2 {
		// UNC path.
		return "file:" + value, true
	}
	return "file:///" + value, true
}
