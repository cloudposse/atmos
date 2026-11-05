package oci

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/registry"
	"github.com/google/go-containerregistry/pkg/v1/random"
	v1remote "github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version/resolver"
)

// startRegistry runs an in-memory OCI registry and pushes one random image
// under the given tags, returning the registry host and the image digest.
func startRegistry(t *testing.T, repoPath string, tags []string) (string, string) {
	t.Helper()
	server := httptest.NewServer(registry.New())
	t.Cleanup(server.Close)
	serverURL, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parsing registry URL: %v", err)
	}
	host := serverURL.Host

	img, err := random.Image(256, 1)
	if err != nil {
		t.Fatalf("creating random image: %v", err)
	}
	digest, err := img.Digest()
	if err != nil {
		t.Fatalf("computing image digest: %v", err)
	}
	repo, err := name.NewRepository(host+"/"+repoPath, name.Insecure)
	if err != nil {
		t.Fatalf("building repository ref: %v", err)
	}
	for _, tag := range tags {
		if err := v1remote.Write(repo.Tag(tag), img); err != nil {
			t.Fatalf("pushing tag %s: %v", tag, err)
		}
	}
	return host, digest.String()
}

func TestOCIResolverVersionsAndPin(t *testing.T) {
	host, digest := startRegistry(t, "acme/app", []string{"1.28.0", "1.28.1", "1.29.0-rc.1"})

	req := &resolver.Request{
		Package:    "acme/app",
		Datasource: DatasourceOCI,
		Provider:   schema.VersionProvider{URL: host, Insecure: true},
		Config:     &schema.AtmosConfiguration{},
	}
	var r Resolver

	candidates, err := r.Versions(context.Background(), req)
	if err != nil {
		t.Fatalf("Versions returned error: %v", err)
	}
	var versions []string
	for _, candidate := range candidates {
		versions = append(versions, candidate.Version)
	}
	sort.Strings(versions)
	expected := []string{"1.28.0", "1.28.1", "1.29.0-rc.1"}
	if len(versions) != len(expected) {
		t.Fatalf("expected %d tags, got %v", len(expected), versions)
	}
	for i := range expected {
		if versions[i] != expected[i] {
			t.Fatalf("expected tags %v, got %v", expected, versions)
		}
	}

	pinned, err := r.Pin(context.Background(), req, "1.28.1")
	if err != nil {
		t.Fatalf("Pin returned error: %v", err)
	}
	if pinned != digest {
		t.Fatalf("expected digest %s, got %s", digest, pinned)
	}
}

func TestOCIResolverPackageWithEmbeddedRegistryHost(t *testing.T) {
	host, digest := startRegistry(t, "acme/embedded", []string{"v1.0.0"})

	// The package itself carries the registry host; no provider URL.
	req := &resolver.Request{
		Package:    host + "/acme/embedded",
		Datasource: DatasourceDocker,
		Provider:   schema.VersionProvider{Insecure: true},
		Config:     &schema.AtmosConfiguration{},
	}
	var r Resolver
	pinned, err := r.Pin(context.Background(), req, "v1.0.0")
	if err != nil {
		t.Fatalf("Pin returned error: %v", err)
	}
	if pinned != digest {
		t.Fatalf("expected digest %s, got %s", digest, pinned)
	}
}

func TestRepositoryDefaultsAndHostDetection(t *testing.T) {
	// docker-tags without provider URL defaults to Docker Hub.
	repo, err := repository(&resolver.Request{Package: "library/nginx", Datasource: DatasourceDocker})
	if err != nil {
		t.Fatalf("repository returned error: %v", err)
	}
	if repo.RegistryStr() != name.DefaultRegistry {
		t.Fatalf("expected Docker Hub default, got %s", repo.RegistryStr())
	}

	// Provider URL is prepended only when the package has no registry host.
	repo, err = repository(&resolver.Request{
		Package:  "acme/app",
		Provider: schema.VersionProvider{URL: "ghcr.io"},
	})
	if err != nil {
		t.Fatalf("repository returned error: %v", err)
	}
	if repo.RegistryStr() != "ghcr.io" {
		t.Fatalf("expected ghcr.io, got %s", repo.RegistryStr())
	}
}

func TestVersionResolverGHCRTokenPriority(t *testing.T) {
	for _, tc := range []struct{ name, host, token, wantPassword string }{
		{"Atmos token precedes Docker for GHCR", "ghcr.io", "atmos-token", "atmos-token"},
		{"Docker remains fallback without token", "ghcr.io", "", "docker-token"},
		{"Atmos token never reaches other hosts", "registry.example.com", "atmos-token", "docker-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := registry.New()
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				username, password, ok := r.BasicAuth()
				if !ok || username != "user" || password != tc.wantPassword {
					w.Header().Set("WWW-Authenticate", `Basic realm="registry"`)
					w.WriteHeader(http.StatusUnauthorized)
					return
				}
				handler.ServeHTTP(w, r)
			}))
			t.Cleanup(server.Close)
			image, err := random.Image(256, 1)
			require.NoError(t, err)
			imageDigest, err := image.Digest()
			require.NoError(t, err)
			seed, err := name.NewTag(strings.TrimPrefix(server.URL, "https://") + "/acme/app:v1")
			require.NoError(t, err)
			require.NoError(t, v1remote.Write(seed, image, v1remote.WithTransport(server.Client().Transport), v1remote.WithAuth(&authn.Basic{Username: "user", Password: tc.wantPassword})))
			directory := t.TempDir()
			config, err := json.Marshal(map[string]any{"auths": map[string]any{tc.host: map[string]string{"auth": base64.StdEncoding.EncodeToString([]byte("user:docker-token"))}}})
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(filepath.Join(directory, "config.json"), config, 0o600))
			t.Setenv("DOCKER_CONFIG", directory)
			// Route real registry requests to the local TLS server; no external registry is contacted.
			transport := server.Client().Transport.(*http.Transport).Clone()
			transport.TLSClientConfig.ServerName = "example.com"
			transport.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, network, server.Listener.Addr().String())
			}
			previous := v1remote.DefaultTransport
			v1remote.DefaultTransport = transport
			t.Cleanup(func() { v1remote.DefaultTransport = previous; transport.CloseIdleConnections() })
			request := &resolver.Request{Package: tc.host + "/acme/app", Config: &schema.AtmosConfiguration{Settings: schema.AtmosSettings{GithubUsername: "user", AtmosGithubToken: tc.token}}}
			versions, err := (Resolver{}).Versions(t.Context(), request)
			require.NoError(t, err)
			require.Equal(t, []resolver.Candidate{{Version: "v1"}}, versions)
			digest, err := (Resolver{}).Pin(t.Context(), request, "v1")
			require.NoError(t, err)
			require.Equal(t, imageDigest.String(), digest)
		})
	}
}
