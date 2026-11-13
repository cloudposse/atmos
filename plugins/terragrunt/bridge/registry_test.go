package atmosbridge

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRegistry serves the Terraform module registry protocol over TLS.
type fakeRegistry struct {
	server      *httptest.Server
	authHeaders map[string]string
	download    string
	requests    []string
}

func newFakeRegistry(t *testing.T, versions ...string) *fakeRegistry {
	t.Helper()
	registry := &fakeRegistry{download: "git::https://example.test/acme/vpc.git?ref=v%s", authHeaders: make(map[string]string)}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/terraform.json", func(w http.ResponseWriter, r *http.Request) {
		registry.record(r)
		fmt.Fprint(w, `{"modules.v1":"/v1/modules/"}`)
	})
	mux.HandleFunc("/v1/modules/acme/vpc/aws/versions", func(w http.ResponseWriter, r *http.Request) {
		registry.record(r)
		body := `{"modules":[{"versions":[`
		for i, version := range versions {
			if i > 0 {
				body += ","
			}
			body += fmt.Sprintf(`{"version":%q}`, version)
		}
		fmt.Fprint(w, body+`]}]}`)
	})
	mux.HandleFunc("/v1/modules/acme/vpc/aws/", func(w http.ResponseWriter, r *http.Request) {
		registry.record(r)
		version := filepath.Base(filepath.Dir(r.URL.Path))
		w.Header().Set("X-Terraform-Get", fmt.Sprintf(registry.download, version))
		w.WriteHeader(http.StatusNoContent)
	})
	registry.server = httptest.NewTLSServer(mux)
	t.Cleanup(registry.server.Close)
	return registry
}

func (r *fakeRegistry) record(req *http.Request) {
	r.requests = append(r.requests, req.URL.Path)
	r.authHeaders[req.URL.Path] = req.Header.Get("Authorization")
}

func (r *fakeRegistry) source(path, query string) string {
	address, _ := url.Parse(r.server.URL)
	return "tfr://" + address.Host + path + query
}

func resolveTestSource(t *testing.T, registry *fakeRegistry, source, constraint string, env map[string]string) (string, error) {
	t.Helper()
	logger := log.New(log.WithOutput(io.Discard))
	return ResolveRegistrySource(context.Background(), logger, RegistryRequest{
		Client: registry.server.Client(), Source: source, Constraint: constraint, Environment: env, OpenTofu: true,
	})
}

func TestResolveRegistrySourceVersionSelection(t *testing.T) {
	tests := []struct {
		name, query, constraint, want string
	}{
		{"exact version", "?version=1.2.0", "", "git::https://example.test/acme/vpc.git?ref=v1.2.0"},
		{"latest stable skips prereleases", "", "", "git::https://example.test/acme/vpc.git?ref=v1.2.0"},
		{"terraform block constraint", "", "~> 1.0, < 1.2", "git::https://example.test/acme/vpc.git?ref=v1.0.0"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry := newFakeRegistry(t, "1.0.0", "1.2.0", "2.0.0-beta.1")
			got, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws", test.query), test.constraint, map[string]string{})
			require.NoError(t, err)
			assert.Equal(t, test.want, got)
		})
	}
}

func TestResolveRegistrySourceSubdirectories(t *testing.T) {
	registry := newFakeRegistry(t, "1.0.0")
	got, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws//modules/nat", "?version=1.0.0"), "", map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "git::https://example.test/acme/vpc.git//modules/nat?ref=v1.0.0", got)

	// A subdirectory on both the registry's answer and the source joins them.
	registry.download = "git::https://example.test/acme/vpc.git//root?ref=v%s"
	got, err = resolveTestSource(t, registry, registry.source("/acme/vpc/aws//modules/nat", "?version=1.0.0"), "", map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, "git::https://example.test/acme/vpc.git//root/modules/nat?ref=v1.0.0", got)
}

func TestResolveRegistrySourceRelativeDownload(t *testing.T) {
	registry := newFakeRegistry(t, "1.0.0")
	registry.download = "/archives/%s.tar.gz"
	got, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws", "?version=1.0.0"), "", map[string]string{})
	require.NoError(t, err)
	assert.Equal(t, registry.server.URL+"/archives/1.0.0.tar.gz", got)
}

func TestResolveRegistrySourceCredentials(t *testing.T) {
	t.Run("TG_TF_REGISTRY_TOKEN is sent as a bearer token", func(t *testing.T) {
		registry := newFakeRegistry(t, "1.0.0")
		_, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws", "?version=1.0.0"), "", map[string]string{"TG_TF_REGISTRY_TOKEN": "bearer-secret"})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"/.well-known/terraform.json": "Bearer bearer-secret", "/v1/modules/acme/vpc/aws/1.0.0/download": "Bearer bearer-secret"}, registry.authHeaders)
	})
	t.Run("TF_TOKEN_host wins over the CLI configuration", func(t *testing.T) {
		registry := newFakeRegistry(t, "1.0.0")
		_, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws", "?version=1.0.0"), "", map[string]string{"TF_TOKEN_127_0_0_1": "env-secret", "TG_TF_REGISTRY_TOKEN": "ignored", "HOME": t.TempDir()})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"/.well-known/terraform.json": "Bearer env-secret", "/v1/modules/acme/vpc/aws/1.0.0/download": "Bearer env-secret"}, registry.authHeaders)
	})
	t.Run("the CLI configuration credentials block is used", func(t *testing.T) {
		registry := newFakeRegistry(t, "1.0.0")
		address, _ := url.Parse(registry.server.URL)
		rc := filepath.Join(t.TempDir(), "terraform.rc")
		require.NoError(t, os.WriteFile(rc, []byte(fmt.Sprintf("credentials %q {\n  token = \"rc-secret\"\n}\n", address.Hostname())), 0o600))
		_, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws", "?version=1.0.0"), "", map[string]string{"TF_CLI_CONFIG_FILE": rc, "TOFU_CLI_CONFIG_FILE": rc, "HOME": t.TempDir()})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"/.well-known/terraform.json": "Bearer rc-secret", "/v1/modules/acme/vpc/aws/1.0.0/download": "Bearer rc-secret"}, registry.authHeaders)
	})
	t.Run("versions request uses credentials", func(t *testing.T) {
		registry := newFakeRegistry(t, "1.0.0")
		_, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws", ""), "", map[string]string{"TG_TF_REGISTRY_TOKEN": "bearer-secret"})
		require.NoError(t, err)
		assert.Equal(t, "Bearer bearer-secret", registry.authHeaders["/.well-known/terraform.json"])
		assert.Equal(t, "Bearer bearer-secret", registry.authHeaders["/v1/modules/acme/vpc/aws/versions"])
		assert.Equal(t, "Bearer bearer-secret", registry.authHeaders["/v1/modules/acme/vpc/aws/1.0.0/download"])
	})
	t.Run("anonymous requests carry no authorization", func(t *testing.T) {
		registry := newFakeRegistry(t, "1.0.0")
		_, err := resolveTestSource(t, registry, registry.source("/acme/vpc/aws", "?version=1.0.0"), "", map[string]string{"HOME": t.TempDir()})
		require.NoError(t, err)
		assert.Equal(t, map[string]string{"/.well-known/terraform.json": "", "/v1/modules/acme/vpc/aws/1.0.0/download": ""}, registry.authHeaders)
	})
}

func TestResolveRegistrySourceErrors(t *testing.T) {
	registry := newFakeRegistry(t, "1.0.0")
	tests := []struct{ name, source, constraint, wantErr string }{
		{"not a registry source", "https://example.test/module.zip", "", "not a tfr://"},
		{"constraint in the query", registry.source("/acme/vpc/aws", "?version=~%3E1.0"), "", "exact version only"},
		{"empty version", registry.source("/acme/vpc/aws", "?version="), "", "must appear once"},
		{"constraint with no match", registry.source("/acme/vpc/aws", ""), ">= 9.0", "satisfies constraint"},
		{"unknown module", registry.source("/acme/other/aws", "?version=1.0.0"), "", "Error downloading module"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := resolveTestSource(t, registry, test.source, test.constraint, map[string]string{})
			require.Error(t, err)
			assert.Contains(t, err.Error(), test.wantErr)
		})
	}
	for _, test := range []struct {
		source string
		want   error
	}{
		{"https://example.test/module.zip", ErrInvalidRegistrySource},
		{registry.source("/acme/vpc/aws", "?version="), ErrMalformedRegistryVersion},
		{registry.source("/acme/vpc/aws", "?version=~%3E1.0"), ErrRegistryVersionConstraint},
	} {
		_, err := resolveTestSource(t, registry, test.source, "", map[string]string{})
		require.True(t, errors.Is(err, test.want), "expected %v in %v", test.want, err)
	}
}

func TestWithSubdir(t *testing.T) {
	assert.Equal(t, "https://x/y.zip", withSubdir("https://x/y.zip", ""))
	assert.Equal(t, "https://x/y.zip", withSubdir("https://x/y.zip", "."))
	assert.Equal(t, "https://x/y.zip//a/b", withSubdir("https://x/y.zip", "/a/b"))
	assert.Equal(t, "git::https://x/y.git//a?ref=v1", withSubdir("git::https://x/y.git/?ref=v1", "a"))
}
