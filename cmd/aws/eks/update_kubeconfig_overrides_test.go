package eks

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// restoreEKSSeams snapshots the package seams and restores them on cleanup.
func restoreEKSSeams(t *testing.T) {
	t.Helper()
	origInit, origMgr := initCliConfig, newAuthManager
	t.Cleanup(func() { initCliConfig, newAuthManager = origInit, origMgr })
}

// okEKSInitConfig is a no-op config loader seam.
func okEKSInitConfig(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
	return schema.AtmosConfiguration{}, nil
}

var errEKSBoom = errors.New("boom")

// fakeEKSAuthManager embeds the (nil) AuthManager interface and overrides only
// the two methods these tests exercise, so we can drive the command without a
// live AWS account.
type fakeEKSAuthManager struct {
	types.AuthManager
	gotName      string
	gotOverrides types.IntegrationOverrides
	execErr      error
	whoami       *types.WhoamiInfo
	authErr      error
}

func (f *fakeEKSAuthManager) ExecuteIntegration(_ context.Context, name string, opts ...types.IntegrationOption) error {
	f.gotName = name
	for _, opt := range opts {
		opt(&f.gotOverrides)
	}
	return f.execErr
}

func (f *fakeEKSAuthManager) Authenticate(_ context.Context, name string) (*types.WhoamiInfo, error) {
	f.gotName = name
	return f.whoami, f.authErr
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_ForwardsOverrides is the
// regression guard for issue #3386 on the AWS side: --kubeconfig/--alias passed
// alongside --integration must reach ExecuteIntegration as overrides instead of
// being silently dropped.
func TestExecuteEKSUpdateKubeconfigViaIntegration_ForwardsOverrides(t *testing.T) {
	restoreEKSSeams(t)

	initCliConfig = okEKSInitConfig
	fake := &fakeEKSAuthManager{}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}

	err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "/home/dev/.config/atmos/kube/prod", "prod-eks")
	require.NoError(t, err)

	assert.Equal(t, "prod/eks", fake.gotName)
	assert.Equal(t, "/home/dev/.config/atmos/kube/prod", fake.gotOverrides.KubeconfigPath)
	assert.Equal(t, "prod-eks", fake.gotOverrides.Alias)
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_EmptyOverrides confirms unset
// flags forward as empty overrides (no-ops), so the integration's configured
// values still win.
func TestExecuteEKSUpdateKubeconfigViaIntegration_EmptyOverrides(t *testing.T) {
	restoreEKSSeams(t)

	initCliConfig = okEKSInitConfig
	fake := &fakeEKSAuthManager{}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}

	err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "", "")
	require.NoError(t, err)

	assert.Equal(t, "prod/eks", fake.gotName)
	assert.Empty(t, fake.gotOverrides.KubeconfigPath)
	assert.Empty(t, fake.gotOverrides.Alias)
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_PropagatesError confirms an
// integration execution error is surfaced.
func TestExecuteEKSUpdateKubeconfigViaIntegration_PropagatesError(t *testing.T) {
	restoreEKSSeams(t)

	initCliConfig = okEKSInitConfig
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return &fakeEKSAuthManager{execErr: errEKSBoom}, nil
	}

	err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "", "")
	require.Error(t, err)
	assert.ErrorIs(t, err, errEKSBoom)
}

// TestExecuteEKSUpdateKubeconfigViaIntegration_SetupErrors covers the config-load
// and auth-manager-construction failure branches.
func TestExecuteEKSUpdateKubeconfigViaIntegration_SetupErrors(t *testing.T) {
	tests := []struct {
		name      string
		initErr   error
		mgrErr    error
		wantErrIs error
	}{
		{name: "config init fails", initErr: errEKSBoom, wantErrIs: errUtils.ErrFailedToInitConfig},
		{name: "manager create fails", mgrErr: errEKSBoom, wantErrIs: errUtils.ErrFailedToInitializeAuthManager},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreEKSSeams(t)

			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, tt.initErr
			}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return &fakeEKSAuthManager{}, tt.mgrErr
			}

			err := executeEKSUpdateKubeconfigViaIntegration("prod/eks", "", "")
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErrIs)
		})
	}
}

// TestExecuteEKSUpdateKubeconfigDirect_Errors drives the direct (non-integration)
// path's failure branches - including the nil-whoami guard that must not panic.
func TestExecuteEKSUpdateKubeconfigDirect_Errors(t *testing.T) {
	tests := []struct {
		name      string
		initErr   error
		mgrErr    error
		whoami    *types.WhoamiInfo
		authErr   error
		wantErrIs []error
	}{
		{name: "config init fails", initErr: errEKSBoom, wantErrIs: []error{errUtils.ErrFailedToInitConfig, errEKSBoom}},
		{name: "manager create fails", mgrErr: errEKSBoom, wantErrIs: []error{errUtils.ErrFailedToInitializeAuthManager, errEKSBoom}},
		{name: "authenticate fails", authErr: errEKSBoom, wantErrIs: []error{errUtils.ErrIdentityAuthFailed, errEKSBoom}},
		{name: "nil credentials", whoami: &types.WhoamiInfo{Credentials: nil}, wantErrIs: []error{errUtils.ErrIdentityAuthFailed}},
		// Authenticate returning (nil, nil) must be handled without a panic.
		{name: "nil whoami result", whoami: nil, wantErrIs: []error{errUtils.ErrIdentityAuthFailed}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			restoreEKSSeams(t)

			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, tt.initErr
			}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return &fakeEKSAuthManager{whoami: tt.whoami, authErr: tt.authErr}, tt.mgrErr
			}

			err := executeEKSUpdateKubeconfigDirect("dev-cluster", "us-east-2", "", "", "dev-admin")
			require.Error(t, err)
			for _, target := range tt.wantErrIs {
				assert.ErrorIs(t, err, target)
			}
		})
	}
}

// TestEKSSeamsDefaultToRealImplementations guards against an accidental nil seam.
func TestEKSSeamsDefaultToRealImplementations(t *testing.T) {
	assert.NotNil(t, initCliConfig)
	assert.NotNil(t, newAuthManager)
}

// Exercise the real SDK and kubeconfig writer with an omitted region, as in #3674.
func TestExecuteEKSUpdateKubeconfigDirect_AuthContext(t *testing.T) {
	for _, tt := range []struct {
		name         string
		unavailable  bool
		writeFailure bool
		wantErr      error
	}{
		{name: "preserves auth context"},
		{name: "working directory unavailable", unavailable: true, wantErr: errUtils.ErrWorkingDirAccessFailed},
		{name: "kubeconfig parent is a file", writeFailure: true, wantErr: errUtils.ErrKubeconfigWrite},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if tt.unavailable && runtime.GOOS != "linux" {
				t.Skip("Only Linux reliably fails Getwd after removing the current directory")
			}
			restoreEKSSeams(t)
			initCliConfig = okEKSInitConfig
			fake := &fakeEKSAuthManager{whoami: &types.WhoamiInfo{Credentials: &types.AWSCredentials{
				AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key",
			}}}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return fake, nil
			}
			projectDir := t.TempDir()
			t.Chdir(projectDir)
			workingDir, err := os.Getwd()
			require.NoError(t, err)
			configDir := t.TempDir()
			require.NoError(t, os.WriteFile(filepath.Join(configDir, "config"), []byte("[default]\nregion = eu-central-1\n"), 0o600))
			t.Setenv("AWS_CONFIG_FILE", filepath.Join(configDir, "config"))
			t.Setenv("AWS_REGION", "")
			t.Setenv("AWS_DEFAULT_REGION", "")
			t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(configDir, "credentials"))
			t.Setenv("AWS_PROFILE", "default")
			t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "false")
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				assert.Equal(t, "/clusters/bar", r.URL.Path)
				assert.Equal(t, http.MethodGet, r.Method)
				if tt.unavailable {
					assert.NoError(t, os.Remove(projectDir))
				}
				w.Header().Set("Content-Type", "application/json")
				_, writeErr := w.Write([]byte(`{"cluster":{"name":"bar","arn":"arn:aws:eks:eu-central-1:123456789012:cluster/bar","endpoint":"https://example.eks.amazonaws.com","certificateAuthority":{"data":"dGVzdA=="}}}`))
				assert.NoError(t, writeErr)
			}))
			defer server.Close()
			t.Setenv("AWS_ENDPOINT_URL_EKS", server.URL)
			path := filepath.Join(configDir, "kubeconfig")
			if tt.writeFailure {
				// The existing AWS config is a file, so it cannot contain a kubeconfig.
				path = filepath.Join(configDir, "config", "kubeconfig")
			}
			err = executeEKSUpdateKubeconfigDirect("bar", "", path, "bar-context", "foo")
			if tt.wantErr != nil {
				require.ErrorIs(t, err, errUtils.ErrEKSIntegrationFailed)
				require.ErrorIs(t, err, tt.wantErr)
				if tt.unavailable {
					assert.NoFileExists(t, path)
				}
				return
			}
			require.NoError(t, err)
			assert.Equal(t, "foo", fake.gotName)
			t.Chdir(t.TempDir())
			config, err := clientcmd.LoadFromFile(path)
			require.NoError(t, err)
			assert.Equal(t, "bar-context", config.CurrentContext)
			user := config.AuthInfos["atmos-eks-bar-eu-central-1"]
			require.NotNil(t, user)
			assert.Equal(t, []string{"aws", "eks", "token", "--cluster-name", "bar", "--region", "eu-central-1", "--identity=foo"}, user.Exec.Args)
			env := map[string]string{}
			for _, entry := range user.Exec.Env {
				env[entry.Name] = entry.Value
			}
			assert.Equal(t, workingDir, env["ATMOS_CHDIR"])
			assert.Equal(t, "foo", env["ATMOS_IDENTITY"])
		})
	}
}
