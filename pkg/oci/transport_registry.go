//nolint:gocritic,lintroller // Descriptor-by-value parameters and thin store delegators mirror the ORAS content contracts so stores stay interchangeable.
package oci

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	ggcrremote "github.com/google/go-containerregistry/pkg/v1/remote"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/registry/remote"
	"oras.land/oras-go/v2/registry/remote/auth"
	"oras.land/oras-go/v2/registry/remote/errcode"
	"oras.land/oras-go/v2/registry/remote/retry"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// The largest manifest pushed under a reference: the 4 MiB limit registries commonly enforce.
	maxPushedManifest = 4 << 20

	userAgent         = "atmos"
	dockerHubLegacy   = "index.docker.io"
	dockerHubRegistry = "docker.io"
	dockerHubEndpoint = "registry-1.docker.io"
)

// Repository opens a registry repository with credentials resolved up front. Resolution
// follows the same precedence as the legacy methods (bound identity, keychain, GHCR
// settings, fallback); an explicitly bound identity that fails stops here, before any
// request, and is never replaced by anonymous access.
//
// Every call builds a new credential cache, so tokens obtained for one identity can
// never be presented by another client or for another repository open.
func (c *Client) Repository(ctx context.Context, repository Repository) (RepositoryStore, error) {
	defer perf.Track(c.config, "oci.Client.Repository")()

	resource, err := repository.legacy()
	if err != nil {
		return nil, err
	}
	authenticator, err := c.authentication(ctx, resource)
	if err != nil {
		return nil, err
	}
	// Resolve the bound credential once before any request, so a failing identity stops here
	// instead of surfacing mid-operation or degrading to anonymous access. The callback below still
	// re-invokes the authenticator on every challenge, so refreshing tokens keep refreshing.
	if _, err := credentialFrom(authenticator); err != nil {
		return nil, err
	}
	repo, err := c.remoteRepository(resource, authenticator)
	if err != nil {
		return nil, err
	}
	return &registryStore{repo: repo}, nil
}

func (c *Client) remoteRepository(resource name.Repository, authenticator authn.Authenticator) (*remote.Repository, error) {
	host := orasHost(resource.RegistryStr())
	repo, err := remote.NewRepository(host + "/" + resource.RepositoryStr())
	if err != nil {
		return nil, wrapRegistry(err)
	}
	repo.PlainHTTP = resource.Scheme() == "http"
	repo.Client = &auth.Client{
		Client:     &http.Client{Transport: newRegistryTransport(c.transport)},
		Header:     http.Header{"User-Agent": {userAgent}},
		Cache:      auth.NewCache(),
		Credential: hostScopedCredential(host, authenticator),
	}
	return repo, nil
}

func orasHost(registry string) string {
	if strings.EqualFold(registry, dockerHubLegacy) {
		return dockerHubRegistry
	}
	return registry
}

// hostScopedCredential releases credentials only to the registry that the repository
// was opened for. ORAS also asks for credentials when it contacts a token realm; the
// registry-scoped credential is the one the legacy transport sent there as well.
func hostScopedCredential(registry string, authenticator authn.Authenticator) auth.CredentialFunc {
	return func(_ context.Context, hostport string) (auth.Credential, error) {
		if !hostMatches(registry, hostport) {
			return auth.EmptyCredential, nil
		}
		return credentialFrom(authenticator)
	}
}

func hostMatches(registry, hostport string) bool {
	if strings.EqualFold(registry, hostport) {
		return true
	}
	return strings.EqualFold(registry, dockerHubRegistry) && strings.EqualFold(hostport, dockerHubEndpoint)
}

// credentialFrom adapts a go-containerregistry authenticator. Authorization is invoked on
// every challenge, so authenticators that refresh tokens keep refreshing.
func credentialFrom(authenticator authn.Authenticator) (auth.Credential, error) {
	if authenticator == nil || authenticator == authn.Anonymous {
		return auth.EmptyCredential, nil
	}
	config, err := authenticator.Authorization()
	if err != nil {
		return auth.EmptyCredential, wrapRegistry(err)
	}
	if config == nil {
		return auth.EmptyCredential, nil
	}
	credential := auth.Credential{
		Username:     config.Username,
		Password:     config.Password,
		RefreshToken: config.IdentityToken,
		AccessToken:  config.RegistryToken,
	}
	// An encoded Auth fills whichever of the username and password is missing, the way the registry
	// transport does; a username or password given explicitly keeps its value.
	if config.Auth != "" && (credential.Username == "" || credential.Password == "") {
		if err := fillFromEncodedAuth(&credential, config.Auth); err != nil {
			return auth.EmptyCredential, err
		}
	}
	return credential, nil
}

// fillFromEncodedAuth decodes a base64 "user:password" value and fills the username and password the
// credential does not already carry.
func fillFromEncodedAuth(credential *auth.Credential, encoded string) error {
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return wrapRegistry(err)
	}
	user, password, found := strings.Cut(string(decoded), ":")
	if !found {
		return errUtils.ErrOCIRegistry
	}
	if credential.Username == "" {
		credential.Username = user
	}
	if credential.Password == "" {
		credential.Password = password
	}
	return nil
}

// registryTransport retries only idempotent reads. Mutations are sent exactly once;
// an unknown outcome is reconciled by resolving the destination instead.
type registryTransport struct {
	reads http.RoundTripper
	base  http.RoundTripper
}

func newRegistryTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		// Keep the transport tuning (proxy, dialer, TLS defaults) the legacy client used.
		base = ggcrremote.DefaultTransport
	}
	return registryTransport{reads: &retry.Transport{Base: base}, base: base}
}

//nolint:lintroller // Hot-path transport wrapper that only selects a round tripper.
func (t registryTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	if request.Method == http.MethodGet || request.Method == http.MethodHead {
		return t.reads.RoundTrip(request)
	}
	return t.base.RoundTrip(request)
}

// registryStore adapts one ORAS repository to RepositoryStore.
type registryStore struct {
	repo *remote.Repository
}

func (s *registryStore) Fetch(ctx context.Context, target ocispec.Descriptor) (io.ReadCloser, error) {
	reader, err := s.repo.Fetch(ctx, target)
	return reader, wrapRegistry(err)
}

func (s *registryStore) Exists(ctx context.Context, target ocispec.Descriptor) (bool, error) {
	exists, err := s.repo.Exists(ctx, target)
	return exists, wrapRegistry(err)
}

func (s *registryStore) Push(ctx context.Context, expected ocispec.Descriptor, content io.Reader) error {
	return wrapRegistry(s.repo.Push(ctx, expected, content))
}

// PushReference verifies the manifest bytes against expected before anything is sent, then pushes
// them under reference. A registry that omits Docker-Content-Digest therefore can never tag content
// other than the expected manifest.
func (s *registryStore) PushReference(ctx context.Context, expected ocispec.Descriptor, content io.Reader, reference string) error {
	if expected.Size < 0 || expected.Size > maxPushedManifest {
		return errUtils.ErrOCIIntegrity
	}
	raw, err := readVerified(content, expected, maxPushedManifest)
	if err != nil {
		return err
	}
	return wrapRegistry(s.repo.PushReference(ctx, expected, bytes.NewReader(raw), reference))
}

func (s *registryStore) Resolve(ctx context.Context, reference string) (ocispec.Descriptor, error) {
	defer perf.Track(nil, "oci.registryStore.Resolve")()

	resolved, err := s.repo.Resolve(ctx, reference)
	return resolved, wrapRegistry(err)
}

func (s *registryStore) Tag(ctx context.Context, desc ocispec.Descriptor, reference string) error {
	return wrapRegistry(s.repo.Tag(ctx, desc, reference))
}

func (s *registryStore) Predecessors(ctx context.Context, node ocispec.Descriptor) ([]ocispec.Descriptor, error) {
	// Referrers applies the 400/406 tag-schema fallback, so discovery works without a prior Referrers call.
	return s.Referrers(ctx, node, "")
}

func (s *registryStore) Tags(ctx context.Context) ([]string, error) {
	defer perf.Track(nil, "oci.registryStore.Tags")()

	var tags []string
	err := s.repo.Tags(ctx, "", func(page []string) error {
		tags = append(tags, page...)
		return nil
	})
	if err != nil {
		return nil, wrapRegistry(err)
	}
	return tags, nil
}

// Delete removes a manifest. A descriptor without a media type names the manifest by digest only:
// it is deleted with a single DELETE request that needs no read access and performs no referrers
// index cleanup, because the manifest is never fetched.
func (s *registryStore) Delete(ctx context.Context, target ocispec.Descriptor) error {
	if target.MediaType == "" {
		return wrapRegistry(s.repo.Manifests().Delete(ctx, target))
	}
	err := s.repo.Delete(ctx, target)
	if err != nil && referrersProbeUnavailable(err) {
		// Deleting an attachment first probes the referrers API to decide whether the tag-schema index needs
		// updating. A 400 or 406 on that probe fails the call before any DELETE is sent, so settling the
		// capability as unsupported and calling again never repeats a DELETE. If the capability was already
		// settled the first answer stands.
		if capabilityErr := s.repo.SetReferrersCapability(false); capabilityErr == nil {
			err = s.repo.Delete(ctx, target)
		}
	}
	return wrapRegistry(err)
}

func (s *registryStore) Referrers(ctx context.Context, subject ocispec.Descriptor, artifactType string) ([]ocispec.Descriptor, error) {
	found, err := s.listReferrers(ctx, subject, artifactType)
	if err != nil && referrersAPIUnavailable(err) {
		// The registry answered the referrers API with 400 or 406 instead of 404: the API is not available
		// there. Mark it unsupported, which switches this repository to the tag schema, and list again. If the
		// capability was already settled the first answer stands.
		if capabilityErr := s.repo.SetReferrersCapability(false); capabilityErr == nil {
			found, err = s.listReferrers(ctx, subject, artifactType)
		}
	}
	if err != nil {
		return nil, wrapRegistry(err)
	}
	return found, nil
}

func (s *registryStore) listReferrers(ctx context.Context, subject ocispec.Descriptor, artifactType string) ([]ocispec.Descriptor, error) {
	var found []ocispec.Descriptor
	err := s.repo.Referrers(ctx, subject, artifactType, func(page []ocispec.Descriptor) error {
		found = append(found, page...)
		return nil
	})
	return found, err
}

// referrersAPIUnavailable reports whether err is a registry response that says the referrers API is not
// available: 400 Bad Request or 406 Not Acceptable. Every other error, including other statuses, is a failure.
func referrersAPIUnavailable(err error) bool {
	var response *errcode.ErrorResponse
	if !errors.As(err, &response) {
		return false
	}
	return response.StatusCode == http.StatusBadRequest || response.StatusCode == http.StatusNotAcceptable
}

// referrersProbeUnavailable reports whether err is a referrers API answer of 400 or 406 to a read of the
// referrers endpoint. A response to any other request, a DELETE included, never qualifies.
func referrersProbeUnavailable(err error) bool {
	var response *errcode.ErrorResponse
	if !errors.As(err, &response) || !referrersAPIUnavailable(err) {
		return false
	}
	return response.Method == http.MethodGet && response.URL != nil && strings.Contains(response.URL.Path, "/referrers/")
}

var (
	_ RepositoryStore = (*registryStore)(nil)
	_ Transport       = (*Client)(nil)
)
