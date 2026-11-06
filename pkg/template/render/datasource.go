package render

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/hairyhenderson/gomplate/v5"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// DatasourceCache memoizes datasource reads by alias and arguments so that a
// datasource referenced many times across a stack is fetched once.
type DatasourceCache struct {
	entries sync.Map
}

// NewDatasourceCache creates an empty DatasourceCache.
func NewDatasourceCache() *DatasourceCache {
	defer perf.Track(nil, "render.NewDatasourceCache")()

	return &DatasourceCache{}
}

// defaultDatasourceCache backs CachedDatasource, preserving the process-wide
// caching users rely on.
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
// in the key one stack's cached value would leak into the other's render. The
// process environment is part of the key too, because a read can depend on
// it: an env datasource reads it directly, and cloud datasources take
// credentials and regions from it, which `templates.settings.env` sets per
// stack.
func cacheKey(alias string, source *Source, args []string) string {
	var target string
	var headers map[string][]string
	if source != nil {
		if source.URL != nil {
			target = source.URL.String()
		}
		headers = source.Header
	}
	return alias + cacheKeySep + target + cacheKeySep + headerKey(headers) + cacheKeySep + strings.Join(args, cacheKeySep) +
		cacheKeySep + environmentKey()
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

// environmentKey fingerprints the process environment, so reads made under
// different environments never share a cache entry.
func environmentKey() string {
	env := os.Environ()
	sort.Strings(env)
	sum := sha256.Sum256([]byte(strings.Join(env, cacheKeySep)))
	return hex.EncodeToString(sum[:])
}

// load returns the cached value for key, if any.
func (c *DatasourceCache) load(key string) (any, bool) {
	return c.entries.Load(key)
}

// store caches value under key.
func (c *DatasourceCache) store(key string, value any) {
	c.entries.Store(key, value)
}

// Read returns the parsed value of the datasource registered under alias in
// sources, reading it through sources only on a cache miss. The cache key
// includes the alias's effective definition, so two stacks that define the
// same alias differently never see each other's values. Nil values are not
// cached.
func (c *DatasourceCache) Read(sources *Data, alias string, args ...string) (any, error) {
	defer perf.Track(nil, "render.DatasourceCache.Read")()

	if sources == nil {
		return nil, errUtils.ErrGomplateDatasourceUnavailable
	}

	// An undefined absolute-URL alias is keyed by the definition the read
	// registers, so the first and later reads share one entry.
	key := cacheKey(alias, sources.effectiveSource(alias), args)
	if value, ok := c.load(key); ok && value != nil {
		return copyDatasourceValue(value), nil
	}

	value, err := sources.Datasource(alias, args...)
	if err != nil {
		return nil, err
	}
	// The environment is process-wide, and a concurrent render can change it
	// (templates.settings.env) during the read. Cache the value only when the
	// key still names the environment it was read under; otherwise the caller
	// gets the value, as an uncached read would, but no other render does.
	if value != nil && cacheKey(alias, sources.effectiveSource(alias), args) == key {
		c.store(key, value)
	}
	// Every caller gets its own copy, so a template that changes the value
	// (for example with Sprig's `set`) cannot change what later renders read.
	return copyDatasourceValue(value), nil
}

// copyDatasourceValue copies the mutable containers gomplate's parsers return
// (maps and slices, including CSV's [][]string), keeping their types. Other
// values are immutable or opaque and are returned as they are.
func copyDatasourceValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[key] = copyDatasourceValue(item)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			out[i] = copyDatasourceValue(item)
		}
		return out
	case []string:
		return append([]string(nil), v...)
	case [][]string:
		out := make([][]string, len(v))
		for i, row := range v {
			out[i] = append([]string(nil), row...)
		}
		return out
	default:
		return value
	}
}

// CachedDatasource reads a datasource through the process-wide cache that
// backs `atmos.GomplateDatasource`, so a datasource referenced from many
// stacks and components is fetched once.
func CachedDatasource(sources *Data, alias string, args ...string) (any, error) {
	defer perf.Track(nil, "render.CachedDatasource")()

	return defaultDatasourceCache.Read(sources, alias, args...)
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

	foldDriveLetterHost(parsed)

	if parsed.Scheme == "" && path.IsAbs(parsed.Path) {
		parsed.Scheme = "file"
	}

	return parsed, nil
}

// foldDriveLetterHost repairs a file URL whose Windows drive letter was parsed
// as the host. Atmos builds file URLs for temp files with the drive letter in
// the path and no host (see fixWindowsFileScheme in internal/exec), which is
// also the form gomplate expects; but url.URL.String renders that as
// "file://D:/path", and parsing it again yields Host "D:" and Path "/path".
// Moving the drive letter back into the path restores the canonical form.
func foldDriveLetterHost(parsed *url.URL) {
	if parsed.Scheme != "file" || len(parsed.Host) != 2 || parsed.Host[1] != ':' {
		return
	}
	letter := parsed.Host[0]
	if (letter < 'a' || letter > 'z') && (letter < 'A' || letter > 'Z') {
		return
	}
	parsed.Path = parsed.Host + parsed.Path
	parsed.Host = ""
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
