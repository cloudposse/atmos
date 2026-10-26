package installer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/toolchain/registry"
	"github.com/cloudposse/atmos/pkg/toolchain/verification"
)

var (
	errConfiguredLatest = errors.New("configured latest lookup failed")
	errAquaLatest       = errors.New("aqua latest lookup failed")
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

func TestLegacyVerifierVersion(t *testing.T) {
	t.Parallel()

	assert.Equal(t, legacyCosignVerifierVersion, legacyVerifierVersion("cosign", []string{
		"verify-blob", "--certificate", "/tmp/cert.pem", "--signature", "/tmp/signature.sig",
	}))
	assert.Empty(t, legacyVerifierVersion("cosign", []string{"verify-blob", "--bundle", "/tmp/evidence.json"}))
	assert.Empty(t, legacyVerifierVersion("minisign", []string{"--certificate", "/tmp/cert.pem", "--signature", "/tmp/signature.sig"}))
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
		latest: "v3.1.1",
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
	assert.Equal(t, "v3.1.1", reg.requestedVersion)
	assert.Equal(t, 1, reg.latestCalls, "cosign bootstrap should prefer latest when lookup succeeds")
}

func TestVerifierCommandRunnerAutoInstallVerifiesChecksum(t *testing.T) {
	testBinary, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)
	digest := sha256.Sum256(testBinary)
	correctChecksum := hex.EncodeToString(digest[:])

	newRegistry := func(checksumBody string) (*verifierBootstrapRegistry, *httptest.Server) {
		mux := http.NewServeMux()
		mux.HandleFunc("/checksums.txt", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(checksumBody))
		})
		mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write(testBinary)
		})
		ts := httptest.NewServer(mux)
		reg := &verifierBootstrapRegistry{
			latest: "v3.1.1",
			tool: &registry.Tool{
				Type:       "http",
				RepoOwner:  "sigstore",
				RepoName:   "cosign",
				Asset:      ts.URL + "/{{.Version}}/cosign",
				Format:     "raw",
				BinaryName: "cosign",
				Checksum: registry.ChecksumConfig{
					Type:       "http",
					URL:        ts.URL + "/checksums.txt",
					FileFormat: "raw",
				},
			},
		}
		return reg, ts
	}

	t.Run("valid checksum allows install", func(t *testing.T) {
		reg, ts := newRegistry(correctChecksum)
		defer ts.Close()

		inst := &Installer{
			cacheDir:         t.TempDir(),
			binDir:           t.TempDir(),
			configuredReg:    reg,
			useConfiguredReg: true,
			registryFactory:  verifierBootstrapFactory{registry: reg},
		}

		t.Setenv("PATH", t.TempDir())
		t.Setenv("ATMOS_VERIFIER_HELPER_PROCESS", "1")

		err := verifierCommandRunner{
			installer: inst,
			policy: verification.Policy{
				VerifierInstall: verification.VerifierInstallAuto,
			},
		}.Run(context.Background(), "cosign", "-test.run=TestVerifierCommandHelperProcess", "--", "success")

		require.NoError(t, err)
	})

	t.Run("mismatched checksum blocks install", func(t *testing.T) {
		wrongDigest := sha256.Sum256([]byte("not the verifier binary"))
		reg, ts := newRegistry(hex.EncodeToString(wrongDigest[:]))
		defer ts.Close()

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
		assert.Contains(t, err.Error(), "checksum mismatch")
	})
}

func TestVerifierCommandRunnerTrustStepFailureDoesNotBlockExecution(t *testing.T) {
	testBinary, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testBinary)
	}))
	defer ts.Close()

	reg := &verifierBootstrapRegistry{
		latest: "v3.1.1",
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

	prevTrustFunc := trustVerifierBinaryFunc
	t.Cleanup(func() { trustVerifierBinaryFunc = prevTrustFunc })
	trustVerifierBinaryFunc = func(string) error {
		return errors.New("simulated trust failure")
	}

	err = verifierCommandRunner{
		installer: inst,
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallAuto,
			VerifierTrust:   verification.VerifierTrustAuto,
		},
	}.Run(context.Background(), "cosign", "-test.run=TestVerifierCommandHelperProcess", "--", "success")

	require.NoError(t, err, "trust-step failure must not block command execution")
}

func TestVerifierCommandRunnerSkipsTrustStepWhenDisabled(t *testing.T) {
	testBinary, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testBinary)
	}))
	defer ts.Close()

	reg := &verifierBootstrapRegistry{
		latest: "v3.1.1",
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

	prevTrustFunc := trustVerifierBinaryFunc
	t.Cleanup(func() { trustVerifierBinaryFunc = prevTrustFunc })
	callCount := 0
	trustVerifierBinaryFunc = func(string) error {
		callCount++
		return nil
	}

	err = verifierCommandRunner{
		installer: inst,
		policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallAuto,
			VerifierTrust:   verification.VerifierTrustDisabled,
		},
	}.Run(context.Background(), "cosign", "-test.run=TestVerifierCommandHelperProcess", "--", "success")

	require.NoError(t, err)
	assert.Equal(t, 0, callCount, "trust step must not run when VerifierTrust is disabled")
}

func TestRunBootstrapVerifierHoldsVersionLockThroughTrustAndExecution(t *testing.T) {
	testBinary, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(testBinary)
	}))
	t.Cleanup(ts.Close)

	reg := &verifierBootstrapRegistry{
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

	previousTrustFunc := trustVerifierBinaryFunc
	t.Cleanup(func() { trustVerifierBinaryFunc = previousTrustFunc })

	trustStarted := make(chan struct{})
	releaseTrust := make(chan struct{})
	var trustOnce sync.Once
	trustCalls := 0
	trustVerifierBinaryFunc = func(string) error {
		trustCalls++
		trustOnce.Do(func() { close(trustStarted) })
		<-releaseTrust
		return nil
	}

	errs := make(chan error, 2)
	run := func() {
		errs <- verifierCommandRunner{policy: verification.Policy{
			VerifierInstall: verification.VerifierInstallAuto,
			VerifierTrust:   verification.VerifierTrustAuto,
		}}.runBootstrapVerifier(context.Background(), &verifierBootstrapRequest{
			name:      "cosign",
			version:   "v3.1.1",
			owner:     "sigstore",
			repo:      "cosign",
			installer: *inst,
			args:      []string{"-test.run=TestVerifierCommandHelperProcess", "--", "success"},
		})
	}
	go run()
	<-trustStarted
	go run()

	assert.Never(t, func() bool { return reg.getToolWithVersionCalls() > 1 }, 100*time.Millisecond, 10*time.Millisecond,
		"the second bootstrap must not reinstall the verifier while the first lifecycle is active")
	close(releaseTrust)
	for range 2 {
		require.NoError(t, <-errs)
	}
	assert.Equal(t, 2, reg.getToolWithVersionCalls())
	assert.Equal(t, 1, trustCalls, "the trust marker must avoid re-trusting the shared verifier")
}

func TestVerifierCommandRunnerAutoInstallFallsBackToPinnedCosignWhenLatestLookupFails(t *testing.T) {
	testBinary, err := os.ReadFile(os.Args[0])
	require.NoError(t, err)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(testBinary)
	}))
	defer ts.Close()

	configuredReg := &verifierBootstrapRegistry{
		latestErr: errConfiguredLatest,
		tool: &registry.Tool{
			Type:       "http",
			RepoOwner:  "sigstore",
			RepoName:   "cosign",
			Asset:      ts.URL + "/{{.Version}}/cosign",
			Format:     "raw",
			BinaryName: "cosign",
		},
	}
	aquaReg := &verifierBootstrapRegistry{latestErr: errAquaLatest}
	inst := &Installer{
		cacheDir:         t.TempDir(),
		binDir:           t.TempDir(),
		configuredReg:    configuredReg,
		useConfiguredReg: true,
		registryFactory:  verifierBootstrapFactory{registry: aquaReg},
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
	assert.Equal(t, "v3.0.6", configuredReg.requestedVersion)
	assert.Equal(t, 1, configuredReg.latestCalls)
	assert.Equal(t, 1, aquaReg.latestCalls)
}

func TestVerifierCommandRunnerAutoInstallFailsBeforeInstallingLatest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unexpected verifier install", http.StatusInternalServerError)
	}))
	defer ts.Close()

	reg := &verifierBootstrapRegistry{
		latest: "",
		tool: &registry.Tool{
			Type:       "http",
			RepoOwner:  "slsa-framework",
			RepoName:   "slsa-verifier",
			Asset:      ts.URL + "/{{.Version}}/slsa-verifier",
			Format:     "raw",
			BinaryName: "slsa-verifier",
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
	}.Run(context.Background(), "slsa-verifier", "-test.run=TestVerifierCommandHelperProcess", "--", "success")

	require.ErrorIs(t, err, verification.ErrVerifierCommandRequired)
	assert.Empty(t, reg.requestedVersion, "bootstrap install must not be called with literal latest")
}

func TestResolveVerifierInstallVersionPreservesLookupErrors(t *testing.T) {
	configuredReg := &verifierBootstrapRegistry{latestErr: errConfiguredLatest}
	aquaReg := &verifierBootstrapRegistry{latestErr: errAquaLatest}
	inst := &Installer{
		configuredReg:    configuredReg,
		useConfiguredReg: true,
		registryFactory:  verifierBootstrapFactory{registry: aquaReg},
	}

	version, err := inst.resolveVerifierInstallVersion("slsa-framework", "slsa-verifier")

	require.Empty(t, version)
	require.ErrorIs(t, err, ErrVerifierVersionUnavailable)
	require.ErrorIs(t, err, errConfiguredLatest)
	require.ErrorIs(t, err, errAquaLatest)
	assert.Contains(t, err.Error(), "configured registry latest version lookup failed")
	assert.Contains(t, err.Error(), "aqua registry latest version lookup failed")
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
		if arg != "--" || i+1 >= len(os.Args) {
			continue
		}
		if os.Args[i+1] == "success" {
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
	mu                       sync.Mutex
	latest                   string
	latestErr                error
	latestCalls              int
	tool                     *registry.Tool
	requestedVersion         string
	toolWithVersionCallCount int
}

func (r *verifierBootstrapRegistry) GetTool(_, _ string) (*registry.Tool, error) {
	return r.tool, nil
}

func (r *verifierBootstrapRegistry) GetToolWithVersion(_, _, version string) (*registry.Tool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.requestedVersion = version
	r.toolWithVersionCallCount++
	return r.tool, nil
}

func (r *verifierBootstrapRegistry) getToolWithVersionCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.toolWithVersionCallCount
}

func (r *verifierBootstrapRegistry) GetLatestVersion(_, _ string) (string, error) {
	r.mu.Lock()
	r.latestCalls++
	r.mu.Unlock()

	if r.latestErr != nil {
		return "", r.latestErr
	}
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
