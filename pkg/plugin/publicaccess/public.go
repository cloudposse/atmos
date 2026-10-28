// Package publicaccess verifies public registry visibility without consulting local OCI credentials.
package publicaccess

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

const (
	maxManifest     = 8 << 20
	maxBlob         = 256 << 20
	maxObjects      = 128
	sha256HexLength = 64
)

// HTTPClient is the HTTP boundary for GitHub package metadata.
//
//go:generate go run go.uber.org/mock/mockgen -source=public.go -destination=mock_public_test.go -package=publicaccess
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// Client verifies metadata and anonymously downloads complete OCI graphs.
type Client struct {
	http      HTTPClient
	transport http.RoundTripper
	token     string
}

// Option configures public verification.
type Option func(*Client)

// WithGitHubToken supplies a token only for the GitHub metadata API.
//
//nolint:lintroller // Functional option.
func WithGitHubToken(token string) Option { return func(c *Client) { c.token = token } }

// WithHTTPClient supplies the metadata API client.
//
//nolint:lintroller // Functional option.
func WithHTTPClient(client HTTPClient) Option { return func(c *Client) { c.http = client } }

// WithTransport supplies anonymous registry transport, including custom trust roots.
//
//nolint:lintroller // Functional option.
func WithTransport(value http.RoundTripper) Option { return func(c *Client) { c.transport = value } }

// New constructs a verifier with bounded requests and no ambient OCI credentials.
//
//nolint:lintroller // Constructor only.
func New(options ...Option) *Client {
	c := &Client{http: &http.Client{Timeout: time.Minute}, transport: http.DefaultTransport}
	for _, option := range options {
		option(c)
	}
	return c
}

// RequirePublic checks GHCR's package visibility before a publisher writes any bytes.
// The owner is derived from the repository; organization packages are supported.
func (c *Client) RequirePublic(ctx context.Context, repository string) error {
	defer perf.Track(nil, "publicaccess.RequirePublic")()
	repo, err := pluginoci.ParseDestination(repository)
	if err != nil {
		return err
	}
	parts := strings.SplitN(repo.RepositoryStr(), "/", 2)
	if repo.RegistryStr() != "ghcr.io" || len(parts) != 2 {
		return fmt.Errorf("%w: visibility preflight requires a GHCR owner/package repository", errUtils.ErrPluginInvalidConfig)
	}
	response, err := c.packageMetadata(ctx, parts)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("%w: provision %s as a public GHCR package before publishing (HTTP %d)", errUtils.ErrOCIRegistry, repo.Name(), response.StatusCode)
	}
	raw, err := readBounded(response.Body, maxManifest)
	if err != nil {
		return err
	}
	var metadata struct {
		Visibility string `json:"visibility"`
	}
	if err := json.Unmarshal(raw, &metadata); err != nil {
		return errors.Join(errUtils.ErrOCIRegistry, err)
	}
	if metadata.Visibility != "public" {
		return fmt.Errorf("%w: %s must be public in GitHub package settings", errUtils.ErrOCIRegistry, repo.Name())
	}
	return nil
}

func (c *Client) packageMetadata(ctx context.Context, parts []string) (*http.Response, error) {
	endpoint := "https://api.github.com/orgs/" + url.PathEscape(parts[0]) + "/packages/container/" + url.PathEscape(parts[1])
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}
	response, err := c.http.Do(request)
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	return response, nil
}

// Verify downloads and hashes every manifest, config, and layer using anonymous registry authentication.
// Expected pins the root digest when nonempty. No platform is selected or omitted.
func (c *Client) Verify(ctx context.Context, repository, reference, expected string) (string, error) {
	defer perf.Track(nil, "publicaccess.Verify")()
	repo, err := pluginoci.ParseDestination(repository)
	if err != nil {
		return "", err
	}
	if reference == "" {
		return "", errUtils.ErrPluginInvalidConfig
	}
	ref := repo.Name() + ":" + reference
	if strings.HasPrefix(reference, "sha256:") {
		ref = repo.Name() + "@" + reference
		if expected == "" {
			expected = reference
		}
	}
	if _, err := name.ParseReference(ref, name.StrictValidation); err != nil {
		return "", errors.Join(errUtils.ErrPluginInvalidConfig, err)
	}
	if expected != "" {
		if _, err := v1.NewHash(expected); err != nil {
			return "", errors.Join(errUtils.ErrOCIIntegrity, err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	tr, err := transport.NewWithContext(ctx, repo.Registry, authn.Anonymous, c.transport, []string{repo.Scope(transport.PullScope)})
	if err != nil {
		return "", err
	}
	walk := graph{ctx: ctx, client: &http.Client{Transport: tr, Timeout: time.Minute}, repository: repo, seen: make(map[string]int64)}
	return walk.pull(reference, false, expected, -1)
}

type graph struct {
	ctx        context.Context
	client     HTTPClient
	repository name.Repository
	seen       map[string]int64
}

func (g *graph) pull(reference string, blob bool, expected string, size int64) (string, error) {
	kind, limit := "manifests", int64(maxManifest)
	if blob {
		kind, limit = "blobs", maxBlob
	}
	key := kind + "/" + reference
	if previous, ok := g.seen[key]; ok {
		if !matchesDescriptor("", "", size, previous) {
			return "", errUtils.ErrOCIIntegrity
		}
		return expected, nil
	}
	if len(g.seen) >= maxObjects || size > limit || size < -1 {
		return "", errUtils.ErrPluginPayloadTooLarge
	}
	g.seen[key] = size
	raw, err := g.fetch(key, limit)
	if err != nil {
		return "", err
	}
	hash := sha256.Sum256(raw)
	digest := "sha256:" + hex.EncodeToString(hash[:])
	if !matchesDescriptor(expected, digest, size, int64(len(raw))) {
		return "", errUtils.ErrOCIIntegrity
	}
	g.seen[key] = int64(len(raw))
	if blob {
		return digest, nil
	}
	return digest, g.children(raw)
}

func (g *graph) children(raw []byte) error {
	var manifest struct {
		Manifests []v1.Descriptor `json:"manifests"`
		Config    *v1.Descriptor  `json:"config"`
		Layers    []v1.Descriptor `json:"layers"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return errors.Join(errUtils.ErrOCIIntegrity, err)
	}
	children, childBlob := manifest.Manifests, false
	if manifest.Config != nil {
		children, childBlob = append([]v1.Descriptor{*manifest.Config}, manifest.Layers...), true
	}
	if len(children) == 0 || (manifest.Config != nil && len(manifest.Manifests) != 0) {
		return errUtils.ErrOCIIntegrity
	}
	for i := range children {
		child := &children[i]
		if !validDescriptor(child) {
			return errUtils.ErrOCIIntegrity
		}
		if _, err := g.pull(child.Digest.String(), childBlob, child.Digest.String(), child.Size); err != nil {
			return err
		}
	}
	return nil
}

func (g *graph) fetch(key string, limit int64) ([]byte, error) {
	endpoint := g.repository.Scheme() + "://" + g.repository.RegistryStr() + "/v2/" + g.repository.RepositoryStr() + "/" + key
	request, err := http.NewRequestWithContext(g.ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", "application/vnd.oci.image.index.v1+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json, application/vnd.docker.distribution.manifest.list.v2+json")
	response, err := g.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if err := transport.CheckError(response, http.StatusOK); err != nil {
		return nil, err
	}
	return readBounded(response.Body, limit)
}

func readBounded(reader io.Reader, limit int64) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, errors.Join(errUtils.ErrOCIRegistry, err)
	}
	if int64(len(raw)) > limit {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	return raw, nil
}

func matchesDescriptor(expected, actual string, size, actualSize int64) bool {
	return (expected == "" || expected == actual) && (size < 0 || size == actualSize)
}

func validDescriptor(d *v1.Descriptor) bool {
	return d.Digest.Algorithm == "sha256" && len(d.Digest.Hex) == sha256HexLength && d.Size >= 0
}
