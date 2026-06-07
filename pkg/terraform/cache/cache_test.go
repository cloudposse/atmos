package cache

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/downloader"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestStart_DisabledReturnsNil(t *testing.T) {
	cfg := &schema.AtmosConfiguration{} // Cache == nil.
	setup, err := Start(t.Context(), cfg)
	require.NoError(t, err)
	assert.Nil(t, setup)

	cfg.Components.Terraform.Cache = &schema.TerraformCacheConfig{Enabled: false}
	setup, err = Start(t.Context(), cfg)
	require.NoError(t, err)
	assert.Nil(t, setup)
}

func TestParseDuration(t *testing.T) {
	assert.Equal(t, defaultMetadataTTL, parseDuration("", defaultMetadataTTL))
	assert.Equal(t, defaultMetadataTTL, parseDuration("not-a-duration", defaultMetadataTTL))
	assert.Equal(t, 2*time.Hour, parseDuration("2h", defaultMetadataTTL))
}

func TestResolveRoot_ExplicitLocation(t *testing.T) {
	dir := t.TempDir()
	root, err := resolveRoot(&schema.TerraformCacheConfig{Location: dir})
	require.NoError(t, err)
	assert.Equal(t, dir, root)
}

func TestResolveRoot_XDGDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ATMOS_XDG_CACHE_HOME", dir)
	root, err := resolveRoot(&schema.TerraformCacheConfig{})
	require.NoError(t, err)
	assert.Contains(t, filepath.ToSlash(root), "terraform/registry")
}

func TestEnsureLayout(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, ensureLayout(dir))
	for _, sub := range layoutDirs {
		info, err := os.Stat(filepath.Join(dir, sub))
		require.NoError(t, err)
		assert.True(t, info.IsDir())
	}
}

func TestContribute_Shape(t *testing.T) {
	setup := &Setup{proxyURL: "http://127.0.0.1:5000/"}
	contrib := setup.Contribute()

	// provider_installation has a network_mirror pointed at the proxy + a direct fallback.
	pi, ok := contrib["provider_installation"].([]any)
	require.True(t, ok)
	require.Len(t, pi, 2)
	nm := pi[0].(map[string]any)["network_mirror"].(map[string]any)
	assert.Equal(t, "http://127.0.0.1:5000/providers/", nm["url"])
	_, hasDirect := pi[1].(map[string]any)["direct"]
	assert.True(t, hasDirect)

	// host overrides modules.v1 for the public registries.
	hosts := contrib["host"].(map[string]any)
	for _, h := range publicModuleHosts {
		services := hosts[h].(map[string]any)["services"].(map[string]any)
		assert.Equal(t, "http://127.0.0.1:5000/modules/"+h+"/", services["modules.v1"])
	}
}

func TestClose_NilSafe(t *testing.T) {
	var setup *Setup
	assert.NoError(t, setup.Close(t.Context()))
}

func TestStart_EnabledStartsProxy(t *testing.T) {
	root := t.TempDir()
	cfg := &schema.AtmosConfiguration{}
	cfg.Components.Terraform.Cache = &schema.TerraformCacheConfig{Enabled: true, Location: root}

	setup, err := Start(t.Context(), cfg)
	require.NoError(t, err)
	require.NotNil(t, setup)
	t.Cleanup(func() { _ = setup.Close(context.Background()) })

	// A TLS certificate was generated under the cache root.
	assert.Equal(t, filepath.Join(root, tlsDirName, tlsCertFile), setup.CertPath())
	_, statErr := os.Stat(setup.CertPath())
	require.NoError(t, statErr)

	// The proxy is listening and Contribute reflects the live (ephemeral-port) URL.
	require.NotEmpty(t, setup.proxyURL)
	contrib := setup.Contribute()
	pi := contrib["provider_installation"].([]any)
	nm := pi[0].(map[string]any)["network_mirror"].(map[string]any)
	assert.Equal(t, setup.proxyURL+"providers/", nm["url"])
	assert.True(t, strings.HasPrefix(setup.proxyURL, "https://"), "proxy serves TLS")
}

func TestSetup_CertPathAndTrustEnv(t *testing.T) {
	// Nil setup is safe and yields empty/nil.
	var nilSetup *Setup
	assert.Empty(t, nilSetup.CertPath())
	env, err := nilSetup.TrustEnv()
	require.NoError(t, err)
	assert.Nil(t, env)

	// Real cert + a deterministic system bundle via SSL_CERT_FILE.
	dir := filepath.Join(t.TempDir(), tlsDirName)
	certPath := filepath.Join(dir, tlsCertFile)
	keyPath := filepath.Join(dir, tlsKeyFile)
	_, err = generateAndWriteProxyCert(dir, certPath, keyPath)
	require.NoError(t, err)
	sysRoots := filepath.Join(t.TempDir(), "roots.pem")
	require.NoError(t, os.WriteFile(sysRoots, []byte("ROOTS\n"), tlsCertPerm))
	t.Setenv("SSL_CERT_FILE", sysRoots)

	setup := &Setup{certPath: certPath}
	assert.Equal(t, certPath, setup.CertPath())
	env, err = setup.TrustEnv()
	require.NoError(t, err)
	require.Len(t, env, 1)
	assert.True(t, strings.HasPrefix(env[0], "SSL_CERT_FILE="))
}

func TestGoGetterResolver_Resolve(t *testing.T) {
	ctrl := gomock.NewController(t)
	md := downloader.NewMockFileDownloader(ctrl)
	md.EXPECT().
		Fetch("git::https://example.com/repo.git", "/dest/dir", downloader.ClientModeAny, moduleSourceFetchTimeout).
		Return(nil)

	r := goGetterResolver{downloader: md}
	require.NoError(t, r.Resolve(context.Background(), "git::https://example.com/repo.git", "/dest/dir"))
}
