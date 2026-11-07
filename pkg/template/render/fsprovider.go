package render

import (
	"io/fs"
	"net/url"

	"github.com/hairyhenderson/gomplate/v5"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/template/compatibility"
)

// fsProvider is the go-fsimpl provider interface gomplate reads datasources
// through.
type fsProvider interface {
	Schemes() []string
	New(u *url.URL) (fs.FS, error)
}

// datasourceFSProvider serves http and https datasources with a single GET, as
// gomplate v3 did, and delegates every other scheme to gomplate's provider.
type datasourceFSProvider struct {
	fallback fsProvider
}

// Schemes returns the schemes gomplate's provider supports.
func (p datasourceFSProvider) Schemes() []string {
	defer perf.Track(nil, "render.datasourceFSProvider.Schemes")()

	return p.fallback.Schemes()
}

// New returns the filesystem for u.
func (p datasourceFSProvider) New(u *url.URL) (fs.FS, error) {
	defer perf.Track(nil, "render.datasourceFSProvider.New")()

	if u.Scheme == "http" || u.Scheme == "https" {
		return compatibility.NewHTTPFS(u)
	}
	return p.fallback.New(u)
}

// Gomplate's renderer reads datasources through gomplate.DefaultFSProvider
// unless the context carries one, and only gomplate can set that context
// value. This package is the only gomplate importer, so replacing the default
// here covers every datasource read.
func init() {
	gomplate.DefaultFSProvider = datasourceFSProvider{fallback: gomplate.DefaultFSProvider}
}

// gomplateProvider returns the provider gomplate reads datasources through.
func gomplateProvider() fsProvider { return gomplate.DefaultFSProvider }
