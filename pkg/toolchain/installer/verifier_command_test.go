package installer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

func TestVerifierTool(t *testing.T) {
	tests := []struct {
		name      string
		wantOwner string
		wantRepo  string
		wantOK    bool
	}{
		{name: "cosign", wantOwner: "sigstore", wantRepo: "cosign", wantOK: true},
		{name: "slsa-verifier", wantOwner: "slsa-framework", wantRepo: "slsa-verifier", wantOK: true},
		{name: "gh", wantOwner: "cli", wantRepo: "cli", wantOK: true},
		{name: "minisign", wantOwner: "jedisct1", wantRepo: "minisign", wantOK: true},
		{name: "unknown"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			owner, repo, ok := verifierTool(tt.name)
			assert.Equal(t, tt.wantOwner, owner)
			assert.Equal(t, tt.wantRepo, repo)
			assert.Equal(t, tt.wantOK, ok)
		})
	}
}

func TestVerifierCommandRunnerRequiresPathWhenAutoInstallDisabled(t *testing.T) {
	err := verifierCommandRunner{
		installer: &Installer{},
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallPathOnly,
		},
	}.Run(context.Background(), "definitely-not-an-atmos-verifier")

	require.ErrorIs(t, err, verification.ErrVerifierCommandRequired)
}

func TestVerifierCommandRunnerRequiresKnownVerifierForAutoInstall(t *testing.T) {
	err := verifierCommandRunner{
		installer: &Installer{},
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallAuto,
		},
	}.Run(context.Background(), "definitely-not-an-atmos-verifier")

	require.ErrorIs(t, err, verification.ErrVerifierCommandRequired)
}

func TestVerifierCommandRunnerUsesExistingCommandOnPath(t *testing.T) {
	exe := os.Args[0]
	t.Setenv("PATH", filepath.Dir(exe)+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("ATMOS_VERIFIER_HELPER_PROCESS", "1")

	err := verifierCommandRunner{
		installer: &Installer{},
		policy:    verification.Policy{VerifierInstall: verification.VerifierInstallAuto},
	}.Run(context.Background(), filepath.Base(exe), "-test.run=TestVerifierCommandHelperProcess", "--", "success")

	require.NoError(t, err)
}

func TestVerifierCommandRunnerAutoInstallUsesResolvedVersion(t *testing.T) {
	testBinary, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(testBinary)
	}))
	defer ts.Close()

	reg := &verifierBootstrapRegistry{
		latest: "v3.0.6",
		tool: &registry.Tool{
			Type:       "http",
			RepoOwner:  "sigstore",
			RepoName:   "cosign",
			Asset:      ts.URL + "/{{.Version}}/cosign",
			Format:     "raw",
			BinaryName: "cosign",
		},
	}
	inst := &Installer{
		cacheDir:         t.TempDir(),
		binDir:           t.TempDir(),
		configuredReg:    reg,
		useConfiguredReg: true,
		registryFactory:  verifierBootstrapFactory{registry: reg},
	}

	t.Setenv("PATH", t.TempDir())
	t.Setenv("ATMOS_VERIFIER_HELPER_PROCESS", "1")

	err = verifierCommandRunner{
		installer: inst,
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallAuto,
		},
	}.Run(context.Background(), "cosign", "-test.run=TestVerifierCommandHelperProcess", "--", "success")

	require.NoError(t, err)
	assert.Equal(t, "v3.0.6", reg.requestedVersion)
}

func TestVerifierCommandRunnerAutoInstallFailsBeforeInstallingLatest(t *testing.T) {
	reg := &verifierBootstrapRegistry{
		latest: "",
		tool: &registry.Tool{
			Type:       "http",
			RepoOwner:  "sigstore",
			RepoName:   "cosign",
			Asset:      "https://example.com/{{.Version}}/cosign",
			Format:     "raw",
			BinaryName: "cosign",
		},
	}
	inst := &Installer{
		cacheDir:         t.TempDir(),
		binDir:           t.TempDir(),
		configuredReg:    reg,
		useConfiguredReg: true,
		registryFactory:  verifierBootstrapFactory{registry: reg},
	}

	t.Setenv("PATH", t.TempDir())

	err := verifierCommandRunner{
		installer: inst,
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallAuto,
		},
	}.Run(context.Background(), "cosign", "-test.run=TestVerifierCommandHelperProcess", "--", "success")

	require.ErrorIs(t, err, verification.ErrVerifierCommandRequired)
	assert.Empty(t, reg.requestedVersion, "bootstrap install must not be called with literal latest")
}

func TestRunVerifierCommandFailure(t *testing.T) {
	t.Setenv("ATMOS_VERIFIER_HELPER_PROCESS", "1")

	err := runVerifierCommand(context.Background(), os.Args[0], "-test.run=TestVerifierCommandHelperProcess", "--", "fail")

	require.ErrorIs(t, err, verification.ErrSignatureFailed)
}

func TestVerifierCommandHelperProcess(t *testing.T) {
	if os.Getenv("ATMOS_VERIFIER_HELPER_PROCESS") != "1" {
		return
	}
	for i, arg := range os.Args {
		if arg == "--" && i+1 < len(os.Args) && os.Args[i+1] == "success" {
			os.Exit(0)
		}
	}
	os.Exit(1)
}

type verifierBootstrapFactory struct {
	registry registry.ToolRegistry
}

func (f verifierBootstrapFactory) NewAquaRegistry() registry.ToolRegistry {
	return f.registry
}

type verifierBootstrapRegistry struct {
	mu               sync.Mutex
	latest           string
	tool             *registry.Tool
	requestedVersion string
}

func (r *verifierBootstrapRegistry) GetTool(_, _ string) (*registry.Tool, error) {
	return r.tool, nil
}

func (r *verifierBootstrapRegistry) GetToolWithVersion(_, _, version string) (*registry.Tool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requestedVersion = version
	return r.tool, nil
}

func (r *verifierBootstrapRegistry) GetLatestVersion(_, _ string) (string, error) {
	return r.latest, nil
}

func (r *verifierBootstrapRegistry) LoadLocalConfig(_ string) error { return nil }

func (r *verifierBootstrapRegistry) Search(_ context.Context, _ string, _ ...registry.SearchOption) ([]*registry.Tool, error) {
	return nil, nil
}

func (r *verifierBootstrapRegistry) ListAll(_ context.Context, _ ...registry.ListOption) ([]*registry.Tool, error) {
	return nil, nil
}

func (r *verifierBootstrapRegistry) GetMetadata(_ context.Context) (*registry.RegistryMetadata, error) {
	return nil, nil
}
