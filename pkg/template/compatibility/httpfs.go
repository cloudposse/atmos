package compatibility

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// errHTTPStatus wraps an unsuccessful HTTP datasource response.
var errHTTPStatus = errors.New("http datasource request failed")

// readOnly is the mode reported for HTTP datasource responses.
const readOnly fs.FileMode = 0o444

// NewHTTPFS returns a filesystem for an http or https datasource rooted at
// base that reads each file with a single GET, as gomplate v3 did. Gomplate
// v5's HTTP filesystem sends a HEAD request to stat a file before the GET and
// fails against servers that reject HEAD; this filesystem takes the content
// type from the GET response instead. Contexts, headers and HTTP clients are
// applied the same way as gomplate's filesystem (WithContext, WithHeader,
// WithHTTPClient).
func NewHTTPFS(base *url.URL) (fs.FS, error) {
	defer perf.Track(nil, "compatibility.NewHTTPFS")()

	return &httpFS{ctx: context.Background(), base: base, client: http.DefaultClient, headers: http.Header{}}, nil
}

type httpFS struct {
	ctx     context.Context
	base    *url.URL
	client  *http.Client
	headers http.Header
}

// WithContext returns a copy of the filesystem that issues requests with ctx.
func (f *httpFS) WithContext(ctx context.Context) fs.FS {
	defer perf.Track(nil, "compatibility.httpFS.WithContext")()

	if ctx == nil {
		return f
	}
	fsys := *f
	fsys.ctx = ctx
	return &fsys
}

// WithHeader returns a copy of the filesystem that adds headers to requests.
func (f *httpFS) WithHeader(headers http.Header) fs.FS {
	defer perf.Track(nil, "compatibility.httpFS.WithHeader")()

	if headers == nil {
		return f
	}
	fsys := *f
	fsys.headers = f.headers.Clone()
	for key, values := range headers {
		for _, value := range values {
			fsys.headers.Add(key, value)
		}
	}
	return &fsys
}

// WithHTTPClient returns a copy of the filesystem that uses client.
func (f *httpFS) WithHTTPClient(client *http.Client) fs.FS {
	defer perf.Track(nil, "compatibility.httpFS.WithHTTPClient")()

	if client == nil {
		return f
	}
	fsys := *f
	fsys.client = client
	return &fsys
}

// Open returns a file for name; the request is made on the first Stat or Read.
func (f *httpFS) Open(name string) (fs.File, error) {
	defer perf.Track(nil, "compatibility.httpFS.Open")()

	if !fs.ValidPath(name) {
		return nil, &fs.PathError{Op: "open", Path: name, Err: fs.ErrInvalid}
	}
	target, err := subURL(f.base, name)
	if err != nil {
		return nil, err
	}
	return &httpFile{ctx: f.ctx, url: target, client: f.client, headers: f.headers, name: name}, nil
}

// ReadFile reads name with a single GET.
func (f *httpFS) ReadFile(name string) ([]byte, error) {
	defer perf.Track(nil, "compatibility.httpFS.ReadFile")()

	file, err := f.Open(name)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return io.ReadAll(file)
}

// Sub returns a filesystem rooted at name.
func (f *httpFS) Sub(name string) (fs.FS, error) {
	defer perf.Track(nil, "compatibility.httpFS.Sub")()

	target, err := subURL(f.base, name)
	if err != nil {
		return nil, err
	}
	fsys := *f
	fsys.base = target
	return &fsys, nil
}

// subURL resolves name against base and keeps base's query parameters, with
// name's parameters taking precedence, as gomplate's filesystem does.
func subURL(base *url.URL, name string) (*url.URL, error) {
	rel, err := url.Parse(name)
	if err != nil {
		return nil, err
	}
	target := base.ResolveReference(rel)
	if base.RawQuery != "" {
		query := base.Query()
		for key := range rel.Query() {
			query.Set(key, rel.Query().Get(key))
		}
		target.RawQuery = query.Encode()
	}
	return target, nil
}

// httpFile fetches its URL once and serves both Stat and Read from that GET.
type httpFile struct {
	ctx     context.Context
	url     *url.URL
	client  *http.Client
	headers http.Header
	name    string

	fetched bool
	body    io.ReadCloser
	info    fs.FileInfo
	err     error
}

func (f *httpFile) fetch() error {
	if f.fetched {
		return f.err
	}
	f.fetched = true
	request, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.url.String(), http.NoBody)
	if err != nil {
		f.err = err
		return err
	}
	request.Header = f.headers.Clone()
	response, err := f.client.Do(request)
	if err != nil {
		f.err = err
		return err
	}
	if response.StatusCode >= http.StatusBadRequest {
		response.Body.Close()
		f.err = fmt.Errorf("%w: GET returned status %d", errHTTPStatus, response.StatusCode)
		return f.err
	}
	modTime := time.Time{}
	if modified := response.Header.Get("Last-Modified"); modified != "" {
		modTime, _ = http.ParseTime(modified)
	}
	f.body = response.Body
	f.info = httpFileInfo{
		name:        f.name,
		size:        response.ContentLength,
		modTime:     modTime,
		contentType: response.Header.Get("Content-Type"),
	}
	return nil
}

// Stat makes the GET request, if not yet made, and describes the response.
func (f *httpFile) Stat() (fs.FileInfo, error) {
	defer perf.Track(nil, "compatibility.httpFile.Stat")()

	if err := f.fetch(); err != nil {
		return nil, err
	}
	return f.info, nil
}

// Read reads the GET response body.
func (f *httpFile) Read(p []byte) (int, error) {
	defer perf.Track(nil, "compatibility.httpFile.Read")()

	if err := f.fetch(); err != nil {
		return 0, err
	}
	return f.body.Read(p)
}

// Close releases the response body.
func (f *httpFile) Close() error {
	defer perf.Track(nil, "compatibility.httpFile.Close")()

	if f.body == nil {
		return nil
	}
	return f.body.Close()
}

// httpFileInfo describes a GET response. ContentType lets gomplate pick the
// parser from the response's Content-Type header.
type httpFileInfo struct {
	name        string
	size        int64
	modTime     time.Time
	contentType string
}

func (i httpFileInfo) Name() string       { return i.name }
func (i httpFileInfo) Size() int64        { return i.size }
func (i httpFileInfo) Mode() fs.FileMode  { return readOnly }
func (i httpFileInfo) ModTime() time.Time { return i.modTime }
func (i httpFileInfo) IsDir() bool        { return false }
func (i httpFileInfo) Sys() any           { return nil }

// ContentType returns the response's Content-Type header.
func (i httpFileInfo) ContentType() string { return i.contentType }
