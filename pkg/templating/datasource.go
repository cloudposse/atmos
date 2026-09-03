package templating

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE

import (
	"fmt"
	"net/http"
	"net/url"
	"path"
	"path/filepath"
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

func cacheKey(alias string, args []string) string {
	return alias + "\x00" + strings.Join(args, "\x00")
}

func (c *DatasourceCache) load(key string) (any, bool) {
	return c.entries.Load(key)
}

func (c *DatasourceCache) store(key string, value any) {
	c.entries.Store(key, value)
}

// Datasource returns the parsed value of the datasource registered under alias
// for the render in progress. It can only be called from a template function
// while Render is executing that template through gomplate's renderer; Render
// routes any template that references atmos.GomplateDatasource there.
func (e *engine) Datasource(alias string, args ...string) (any, error) {
	defer perf.Track(nil, "templating.Engine.Datasource")()

	key := cacheKey(alias, args)
	if value, ok := e.cache.load(key); ok && value != nil {
		return value, nil
	}

	state := e.live.get()
	if state == nil || state.tmpl == nil {
		return nil, errUtils.ErrGomplateDatasourceUnavailable
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
