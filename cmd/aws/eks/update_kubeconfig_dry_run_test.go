package eks

import (
	"bytes"
	"context"
	stdio "io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/tools/clientcmd"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/data"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

var ansiSequence = regexp.MustCompile("\x1b\\[[0-9;]*m")

// compactOutput strips ANSI sequences and whitespace so assertions on a path do
// not depend on where the terminal renderer wrapped the line.
func compactOutput(s string) string {
	return strings.Join(strings.Fields(ansiSequence.ReplaceAllString(s, "")), "")
}

// dryRunAuthManager records whether Authenticate was asked to skip the
// identity's auto-provisioned integrations (ECR login, kubeconfig writes).
type dryRunAuthManager struct {
	fakeEKSAuthManager
	skippedIntegrations bool
	authenticated       bool
	executedIntegration bool
}

func (f *dryRunAuthManager) Authenticate(ctx context.Context, name string) (*types.WhoamiInfo, error) {
	f.authenticated = true
	f.skippedIntegrations = auth.IntegrationsSkipped(ctx)
	return f.fakeEKSAuthManager.Authenticate(ctx, name)
}

func (f *dryRunAuthManager) ExecuteIntegration(ctx context.Context, name string, opts ...types.IntegrationOption) error {
	f.executedIntegration = true
	return f.fakeEKSAuthManager.ExecuteIntegration(ctx, name, opts...)
}

// serveDryRunEKS points the AWS SDK EKS client at an httptest server that
// describes the "bar" cluster in eu-central-1.
func serveDryRunEKS(t *testing.T) {
	t.Helper()
	configDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(configDir, "config"), []byte("[default]\nregion = eu-central-1\n"), 0o600))
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(configDir, "config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(configDir, "credentials"))
	t.Setenv("AWS_PROFILE", "default")
	t.Setenv("AWS_IGNORE_CONFIGURED_ENDPOINT_URLS", "false")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/clusters/bar", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, err := w.Write([]byte(`{"cluster":{"name":"bar","arn":"arn:aws:eks:eu-central-1:123456789012:cluster/bar","endpoint":"https://example.eks.amazonaws.com","certificateAuthority":{"data":"dGVzdA=="}}}`))
		assert.NoError(t, err)
	}))
	t.Cleanup(server.Close)
	t.Setenv("AWS_ENDPOINT_URL_EKS", server.URL)
}

// dryRunStreams routes the data (stdout) and UI (stderr) channels to buffers.
type dryRunStreams struct {
	stdout, stderr *bytes.Buffer
}

func (s *dryRunStreams) Input() stdio.Reader     { return &bytes.Buffer{} }
func (s *dryRunStreams) Output() stdio.Writer    { return s.stdout }
func (s *dryRunStreams) Error() stdio.Writer     { return s.stderr }
func (s *dryRunStreams) RawOutput() stdio.Writer { return s.stdout }
func (s *dryRunStreams) RawError() stdio.Writer  { return s.stderr }

// captureDryRunOutput initializes the data and UI writers with buffers.
func captureDryRunOutput(t *testing.T) (stdout, stderr *bytes.Buffer) {
	t.Helper()
	streams := &dryRunStreams{stdout: &bytes.Buffer{}, stderr: &bytes.Buffer{}}
	ioCtx, err := iolib.NewContext(iolib.WithStreams(streams))
	require.NoError(t, err)
	ui.InitFormatter(ioCtx)
	data.InitWriter(ioCtx)
	t.Cleanup(data.Reset)
	return streams.stdout, streams.stderr
}

// setUpdateKubeconfigFlags sets flags on the shared command and restores them.
func setUpdateKubeconfigFlags(t *testing.T, values map[string]string) {
	t.Helper()
	cmdFlags := updateKubeconfigCmd.Flags()
	for name, value := range values {
		flag := cmdFlags.Lookup(name)
		require.NotNil(t, flag, name)
		original := flag.Value.String()
		require.NoError(t, cmdFlags.Set(name, value))
		t.Cleanup(func() {
			_ = flag.Value.Set(original)
			flag.Changed = false
		})
	}
}

// TestUpdateKubeconfig_DryRunDirectWritesNothing reproduces
// `atmos aws eks update-kubeconfig --identity foo --name bar --dry-run`: it must
// not write the kubeconfig and must not run the identity's auto-provisioned
// integrations (such as an ECR login).
func TestUpdateKubeconfig_DryRunDirectWritesNothing(t *testing.T) {
	restoreEKSSeams(t)
	serveDryRunEKS(t)
	initCliConfig = okEKSInitConfig
	fake := &dryRunAuthManager{fakeEKSAuthManager: fakeEKSAuthManager{whoami: &types.WhoamiInfo{Credentials: &types.AWSCredentials{
		AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key",
	}}}}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}
	path := filepath.Join(t.TempDir(), "kube", "dry-config")
	setUpdateKubeconfigFlags(t, map[string]string{
		"identity": "foo", "name": "bar", "region": "eu-central-1", "kubeconfig": path, "dry-run": "true",
	})
	stdout, stderr := captureDryRunOutput(t)

	require.NoError(t, updateKubeconfigCmd.RunE(updateKubeconfigCmd, nil))

	const arn = "arn:aws:eks:eu-central-1:123456789012:cluster/bar"
	preview, err := clientcmd.Load(stdout.Bytes())
	require.NoError(t, err, "stdout is the kubeconfig entries that would be written")
	assert.Equal(t, arn, preview.CurrentContext)
	require.Contains(t, preview.AuthInfos, "atmos-eks-bar-eu-central-1-123456789012-foo")
	assert.Equal(t, []string{"aws", "eks", "token", "--cluster-name", "bar", "--region", "eu-central-1", "--identity=foo"},
		preview.AuthInfos["atmos-eks-bar-eu-central-1-123456789012-foo"].Exec.Args)
	assert.Contains(t, stderr.String(), "Dry run")
	assert.Contains(t, compactOutput(stderr.String()), compactOutput(path))

	assert.NoFileExists(t, path, "--dry-run must not write the kubeconfig")
	assert.NoDirExists(t, filepath.Dir(path), "--dry-run must not create the kubeconfig directory")
	assert.True(t, fake.authenticated)
	assert.True(t, fake.skippedIntegrations, "--dry-run must not run auto-provisioned integrations such as ECR login")
}

// TestUpdateKubeconfig_DryRunIntegrationWritesNothing covers --integration with
// --dry-run: the integration is not executed (it would write the kubeconfig),
// and the identity is authenticated without its auto-provisioned integrations.
func TestUpdateKubeconfig_DryRunIntegrationWritesNothing(t *testing.T) {
	restoreEKSSeams(t)
	serveDryRunEKS(t)
	path := filepath.Join(t.TempDir(), "kube", "integ-config")
	initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
		return schema.AtmosConfiguration{Auth: schema.AuthConfig{Integrations: map[string]schema.Integration{
			"dev/eks": {
				Kind: "aws/eks",
				Via:  &schema.IntegrationVia{Identity: "foo"},
				Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{
					Name: "bar", Region: "eu-central-1", Alias: "configured-alias",
					Kubeconfig: &schema.KubeconfigSettings{Path: filepath.Join(t.TempDir(), "configured")},
				}},
			},
		}}}, nil
	}
	fake := &dryRunAuthManager{fakeEKSAuthManager: fakeEKSAuthManager{whoami: &types.WhoamiInfo{Credentials: &types.AWSCredentials{
		AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key",
	}}}}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}
	setUpdateKubeconfigFlags(t, map[string]string{"integration": "dev/eks", "kubeconfig": path, "alias": "cli-alias", "dry-run": "true"})
	stdout, stderr := captureDryRunOutput(t)

	require.NoError(t, updateKubeconfigCmd.RunE(updateKubeconfigCmd, nil))

	preview, err := clientcmd.Load(stdout.Bytes())
	require.NoError(t, err)
	assert.Equal(t, "cli-alias", preview.CurrentContext, "the CLI alias overrides the configured one")
	assert.Contains(t, compactOutput(stderr.String()), compactOutput(path), "the CLI kubeconfig path overrides the configured one")
	assert.NoFileExists(t, path, "--dry-run must not write the kubeconfig")
	assert.False(t, fake.executedIntegration, "--dry-run must not execute the integration")
	assert.True(t, fake.skippedIntegrations, "--dry-run must not run auto-provisioned integrations such as ECR login")
	assert.Equal(t, "foo", fake.gotName, "the integration's identity is authenticated")
}

// TestUpdateKubeconfig_WithoutDryRunWrites is the negative path: without
// --dry-run the direct path writes the kubeconfig and authenticates with the
// identity's auto-provisioned integrations enabled, as before.
func TestUpdateKubeconfig_WithoutDryRunWrites(t *testing.T) {
	restoreEKSSeams(t)
	serveDryRunEKS(t)
	initCliConfig = okEKSInitConfig
	fake := &dryRunAuthManager{fakeEKSAuthManager: fakeEKSAuthManager{whoami: &types.WhoamiInfo{Credentials: &types.AWSCredentials{
		AccessKeyID: "test-access-key", SecretAccessKey: "test-secret-key",
	}}}}
	newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
		return fake, nil
	}
	path := filepath.Join(t.TempDir(), "kube", "config")
	setUpdateKubeconfigFlags(t, map[string]string{"identity": "foo", "name": "bar", "region": "eu-central-1", "kubeconfig": path})

	require.NoError(t, updateKubeconfigCmd.RunE(updateKubeconfigCmd, nil))

	assert.FileExists(t, path)
	assert.False(t, fake.skippedIntegrations)
}

// TestUpdateKubeconfig_DryRunIntegrationErrors covers integrations that cannot
// be previewed; nothing is authenticated for them.
func TestUpdateKubeconfig_DryRunIntegrationErrors(t *testing.T) {
	validCluster := &schema.IntegrationSpec{Cluster: &schema.Cluster{Name: "bar", Region: "eu-central-1"}}
	for _, tt := range []struct {
		name        string
		integration *schema.Integration
		wantErr     error
	}{
		{name: "not configured", wantErr: errUtils.ErrIntegrationNotFound},
		{name: "not an EKS integration", integration: &schema.Integration{Kind: "aws/ecr", Via: &schema.IntegrationVia{Identity: "foo"}}, wantErr: errUtils.ErrIntegrationFailed},
		{name: "no identity", integration: &schema.Integration{Kind: "aws/eks", Spec: validCluster}, wantErr: errUtils.ErrIntegrationFailed},
		{name: "no cluster", integration: &schema.Integration{Kind: "aws/eks", Via: &schema.IntegrationVia{Identity: "foo"}}, wantErr: errUtils.ErrIntegrationFailed},
		{name: "no region", integration: &schema.Integration{
			Kind: "aws/eks", Via: &schema.IntegrationVia{Identity: "foo"},
			Spec: &schema.IntegrationSpec{Cluster: &schema.Cluster{Name: "bar"}},
		}, wantErr: errUtils.ErrIntegrationFailed},
	} {
		t.Run(tt.name, func(t *testing.T) {
			restoreEKSSeams(t)
			initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
				configured := map[string]schema.Integration{}
				if tt.integration != nil {
					configured["dev/eks"] = *tt.integration
				}
				return schema.AtmosConfiguration{Auth: schema.AuthConfig{Integrations: configured}}, nil
			}
			fake := &dryRunAuthManager{}
			newAuthManager = func(*schema.AuthConfig, types.CredentialStore, types.Validator, *schema.ConfigAndStacksInfo, string) (types.AuthManager, error) {
				return fake, nil
			}

			err := executeEKSUpdateKubeconfigViaIntegrationDryRun("dev/eks", "", "")

			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
			assert.False(t, fake.authenticated)
		})
	}

	t.Run("setup error", func(t *testing.T) {
		restoreEKSSeams(t)
		initCliConfig = func(schema.ConfigAndStacksInfo, bool) (schema.AtmosConfiguration, error) {
			return schema.AtmosConfiguration{}, errEKSBoom
		}
		err := executeEKSUpdateKubeconfigViaIntegrationDryRun("dev/eks", "", "")
		assert.ErrorIs(t, err, errUtils.ErrFailedToInitConfig)
	})
}
