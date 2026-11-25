package atmosbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"sync"
	"time"

	"github.com/gruntwork-io/terragrunt/internal/getter"
	"github.com/gruntwork-io/terragrunt/internal/tfimpl"
	"github.com/gruntwork-io/terragrunt/pkg/log"
)

// registryTimeout bounds each registry request when the caller supplies no client.
const registryTimeout = 30 * time.Second

var (
	// ErrInvalidRegistrySource identifies a source that cannot use the tfr getter.
	ErrInvalidRegistrySource = errors.New("invalid Terraform registry source")
	// ErrMalformedRegistryVersion identifies an invalid exact version query.
	ErrMalformedRegistryVersion = errors.New("malformed Terraform registry version")
	// ErrRegistryVersionConstraint identifies a constraint in an exact-version query.
	ErrRegistryVersionConstraint = errors.New("terraform registry version constraint requires the terraform block")
	// ErrRegistryLookup identifies a failure of the registry protocol: service discovery, version
	// lookup, the X-Terraform-Get header, or the download URL it names. The upstream error is kept.
	ErrRegistryLookup = errors.New("terraform registry lookup failed")
)

// RegistryRequest describes one `tfr://` module source to resolve.
type RegistryRequest struct {
	// Client issues the registry requests. A nil client uses the default transport with a timeout.
	Client *http.Client
	// Environment supplies TG_TF_DEFAULT_REGISTRY_HOST, TG_TF_REGISTRY_TOKEN, and the variables that locate
	// the user's CLI configuration, such as TF_TOKEN_<host> and TF_CLI_CONFIG_FILE.
	Environment map[string]string
	Source      string
	// Constraint is the terraform block's `version` constraint. It applies when the source has no `?version=`.
	Constraint string
	// OpenTofu selects the default registry host and the CLI configuration files that supply credentials.
	OpenTofu bool
}

// ResolveRegistrySource turns a `tfr://` source into the getter URL the registry's download endpoint names, using
// Terragrunt's own implementation of the Terraform module registry protocol: service discovery, version
// selection, and the X-Terraform-Get header. A `//subdir` on the module path is appended to the result.
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func ResolveRegistrySource(ctx context.Context, logger log.Logger, request RegistryRequest) (string, error) {
	resolution, err := ResolveRegistrySourceWithAuth(ctx, logger, request)
	return resolution.Source, err
}

// RegistryResolution carries same-origin archive authorization separately from the source URL.
// Authorization must remain secret and be sent only to requests for AuthOrigin.
type RegistryResolution struct {
	Source        string
	Authorization string
	AuthOrigin    string
}

// ResolveRegistrySourceWithAuth also returns the registry authorization needed by a
// same-origin archive. It never embeds credentials in the getter URL.
//
//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func ResolveRegistrySourceWithAuth(ctx context.Context, logger log.Logger, request RegistryRequest) (RegistryResolution, error) {
	lookup, err := newRegistryLookup(request)
	if err != nil {
		return RegistryResolution{}, err
	}
	return lookup.resolve(ctx, logger)
}

// registryLookup holds one request's registry transport and authentication scope.
type registryLookup struct {
	client   *http.Client
	auth     getter.RegistryAuth
	request  RegistryRequest
	source   *url.URL
	domain   string
	captured *registryAuthCapture
}

func newRegistryLookup(request RegistryRequest) (*registryLookup, error) {
	source, err := url.Parse(request.Source)
	if err != nil {
		return nil, fmt.Errorf("%w %q: %w", ErrInvalidRegistrySource, request.Source, err)
	}
	if source.Scheme != getter.SchemeTFR {
		return nil, fmt.Errorf("%w: %q is not a tfr:// source", ErrInvalidRegistrySource, request.Source)
	}
	implementation := tfimpl.Terraform
	if request.OpenTofu {
		implementation = tfimpl.OpenTofu
	}
	client := request.Client
	if client == nil {
		client = &http.Client{Timeout: registryTimeout}
	}
	// Capture the exact header the registry lookup used. This honors Terraform's
	// CLI credential precedence without making a second, potentially different lookup.
	captured := &registryAuthCapture{base: client.Transport}
	copyClient := *client
	copyClient.Transport = captured
	client = &copyClient
	env := Environment(request.Environment, io.Discard, io.Discard).WithHTTP(client)
	auth := getter.NewRegistryAuth(env)
	auth.Impl = implementation
	domain := source.Host
	if domain == "" {
		domain = tfimpl.DefaultRegistryDomain(request.Environment, implementation)
	}
	return &registryLookup{client: client, auth: auth, request: request, source: source, domain: domain, captured: captured}, nil
}

func (r *registryLookup) resolve(ctx context.Context, logger log.Logger) (RegistryResolution, error) {
	base, err := getter.GetModuleRegistryURLBasePath(ctx, logger, r.client, r.auth, r.domain)
	if err != nil {
		return RegistryResolution{}, fmt.Errorf("%w: service discovery for %s: %w", ErrRegistryLookup, r.domain, err)
	}
	modulePath, moduleSubdir := getter.SourceDirSubdir(r.source.Path)
	version, err := r.version(ctx, logger, base, modulePath)
	if err != nil {
		return RegistryResolution{}, err
	}
	moduleURL, err := getter.BuildRequestURL(r.domain, base, modulePath, version)
	if err != nil {
		return RegistryResolution{}, fmt.Errorf("%w: module URL for %s: %w", ErrRegistryLookup, modulePath, err)
	}
	header, err := getter.GetTerraformGetHeader(ctx, logger, r.client, r.auth, moduleURL)
	if err != nil {
		return RegistryResolution{}, fmt.Errorf("%w: download header for %s: %w", ErrRegistryLookup, modulePath, err)
	}
	download, err := getter.GetDownloadURLFromHeader(moduleURL, header)
	if err != nil {
		return RegistryResolution{}, fmt.Errorf("%w: download URL for %s: %w", ErrRegistryLookup, modulePath, err)
	}
	return r.resolution(download, moduleSubdir, moduleURL), nil
}

// resolution forwards authorization only to archives on the registry's exact origin.
func (r *registryLookup) resolution(download, moduleSubdir string, moduleURL *url.URL) RegistryResolution {
	archive, subdir := getter.SourceDirSubdir(download)
	// Getter subdirectories are URL paths, with slash separators on every OS.
	//nolint:forbidigo // filepath.Join would corrupt the getter URL on Windows.
	resolution := RegistryResolution{Source: withSubdir(archive, path.Join(subdir, moduleSubdir))}
	archiveURL, archiveErr := url.Parse(archive)
	if archiveErr == nil && (archiveURL.Scheme == "https" || archiveURL.Scheme == "http") &&
		archiveURL.Scheme == moduleURL.Scheme && strings.EqualFold(archiveURL.Host, moduleURL.Host) {
		resolution.Authorization = r.captured.authorization(moduleURL)
		if resolution.Authorization != "" {
			resolution.AuthOrigin = moduleURL.Scheme + "://" + moduleURL.Host
		}
	}
	return resolution
}

type registryAuthCapture struct {
	base     http.RoundTripper
	mu       sync.Mutex
	byOrigin map[string]string
}

//nolint:lintroller // The bridge is a separate module and cannot import the host performance tracker.
func (c *registryAuthCapture) RoundTrip(request *http.Request) (*http.Response, error) {
	if value := request.Header.Get("Authorization"); value != "" {
		c.mu.Lock()
		if c.byOrigin == nil {
			c.byOrigin = make(map[string]string)
		}
		c.byOrigin[request.URL.Scheme+"://"+request.URL.Host] = value
		c.mu.Unlock()
	}
	base := c.base
	if base == nil {
		base = http.DefaultTransport
	}
	return base.RoundTrip(request)
}

func (c *registryAuthCapture) authorization(registry *url.URL) string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.byOrigin[registry.Scheme+"://"+registry.Host]
}

// version applies Terragrunt's precedence: an exact `?version=`, else the terraform block's constraint,
// else the latest stable release.
func (r *registryLookup) version(ctx context.Context, logger log.Logger, base, modulePath string) (string, error) {
	versions, present := r.source.Query()["version"]
	if present {
		if len(versions) != 1 || versions[0] == "" {
			return "", fmt.Errorf("%w: the version query must appear once and not be empty", ErrMalformedRegistryVersion)
		}
		if getter.SourceHasVersionConstraint(r.request.Source) {
			return "", fmt.Errorf("%w: the source %q sets a version constraint in its ?version= query, which accepts an exact version only; express a constraint with the terraform block's version attribute instead", ErrRegistryVersionConstraint, r.request.Source)
		}
		return versions[0], nil
	}
	var version string
	var err error
	if r.request.Constraint != "" {
		version, err = getter.GetMatchingModuleVersion(ctx, logger, r.client, r.auth, r.domain, base, modulePath, r.request.Constraint)
	} else {
		version, err = getter.GetLatestModuleVersion(ctx, logger, r.client, r.auth, r.domain, base, modulePath)
	}
	if err != nil {
		return "", fmt.Errorf("%w: version of %s: %w", ErrRegistryLookup, modulePath, err)
	}
	return version, nil
}

// withSubdir appends `//subdir` to the path of a getter URL, ahead of its query, which is where go-getter reads it.
func withSubdir(source, subdir string) string {
	if subdir == "" || subdir == "." {
		return source
	}
	location, query, hasQuery := strings.Cut(source, "?")
	location = strings.TrimSuffix(location, "/") + "//" + strings.TrimPrefix(subdir, "/")
	if hasQuery {
		return location + "?" + query
	}
	return location
}
