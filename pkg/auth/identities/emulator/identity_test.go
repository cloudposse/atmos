package emulator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Compile-time sentinels: a kind rename must immediately fail the build.
var (
	_ = schema.Identity{Kind: types.IdentityKindAWSEmulator, Emulator: "aws"}
	_ = schema.Identity{Kind: types.IdentityKindKubernetesEmulator, Emulator: "k3s"}
)

// fakeResolver is a test double for types.EmulatorResolver.
type fakeResolver struct {
	env        map[string]string
	kubeconfig []byte
	err        error

	gotName string
}

func (f *fakeResolver) ResolveEmulator(_ context.Context, name string) (map[string]string, []byte, error) {
	f.gotName = name
	return f.env, f.kubeconfig, f.err
}

func newAWSIdentity(t *testing.T) *Identity {
	t.Helper()
	id, err := New("local-aws", &schema.Identity{Kind: types.IdentityKindAWSEmulator, Emulator: "local/aws"})
	require.NoError(t, err)
	return id
}

func TestNew_RejectsNonEmulatorKind(t *testing.T) {
	_, err := New("x", &schema.Identity{Kind: "ambient"})
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrInvalidIdentityKind)
}

func TestKindAndNameAndProvider(t *testing.T) {
	id := newAWSIdentity(t)
	assert.Equal(t, types.IdentityKindAWSEmulator, id.Kind())
	assert.Equal(t, "local-aws", id.Name())

	provider, err := id.GetProviderName()
	require.NoError(t, err)
	assert.Equal(t, "local-aws", provider, "emulator identities are their own root provider")
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  *schema.Identity
		wantErr error
	}{
		{
			name:   "valid cloud",
			config: &schema.Identity{Kind: types.IdentityKindAWSEmulator, Emulator: "aws"},
		},
		{
			name:    "missing emulator reference",
			config:  &schema.Identity{Kind: types.IdentityKindAWSEmulator},
			wantErr: errUtils.ErrInvalidIdentityConfig,
		},
		{
			name:    "must not define via",
			config:  &schema.Identity{Kind: types.IdentityKindAWSEmulator, Emulator: "aws", Via: &schema.IdentityVia{Provider: "p"}},
			wantErr: errUtils.ErrInvalidIdentityConfig,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := &Identity{name: "id", config: tt.config}
			err := id.Validate()
			if tt.wantErr == nil {
				require.NoError(t, err)
				return
			}
			require.Error(t, err)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestAuthenticate_IsNoOp(t *testing.T) {
	id := newAWSIdentity(t)
	creds, err := id.Authenticate(context.Background(), nil)
	require.NoError(t, err)
	assert.Nil(t, creds)
}

func TestPrepareEnvironment_CloudMergesEnvAndUsesEmulatorReference(t *testing.T) {
	id := newAWSIdentity(t)
	resolver := &fakeResolver{env: map[string]string{
		"AWS_ENDPOINT_URL":  "http://localhost:34566",
		"AWS_ACCESS_KEY_ID": "test",
	}}
	id.SetEmulatorResolver(resolver)

	out, err := id.PrepareEnvironment(context.Background(), map[string]string{"EXISTING": "1"})
	require.NoError(t, err)

	assert.Equal(t, "1", out["EXISTING"], "existing env preserved")
	assert.Equal(t, "http://localhost:34566", out["AWS_ENDPOINT_URL"])
	assert.Equal(t, "test", out["AWS_ACCESS_KEY_ID"])
	assert.Equal(t, "local/aws", resolver.gotName)
}

func TestPrepareEnvironment_DoesNotMutateInput(t *testing.T) {
	id := newAWSIdentity(t)
	id.SetEmulatorResolver(&fakeResolver{env: map[string]string{"NEW": "x"}})

	input := map[string]string{"KEEP": "y"}
	_, err := id.PrepareEnvironment(context.Background(), input)
	require.NoError(t, err)

	_, leaked := input["NEW"]
	assert.False(t, leaked, "PrepareEnvironment must not mutate the input map")
}

func TestPrepareEnvironment_NilResolverErrors(t *testing.T) {
	id := newAWSIdentity(t)
	_, err := id.PrepareEnvironment(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorResolverUnavailable)
}

func TestPrepareEnvironment_ResolverErrorPropagates(t *testing.T) {
	id := newAWSIdentity(t)
	id.SetEmulatorResolver(&fakeResolver{err: errUtils.ErrEmulatorNotRunning})
	_, err := id.PrepareEnvironment(context.Background(), nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrEmulatorNotRunning)
}

func TestPrepareEnvironment_KubernetesWritesKubeconfigAndAppends(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", tmp)
	t.Setenv("ATMOS_XDG_CONFIG_HOME", "")

	id, err := New("local-k8s", &schema.Identity{Kind: types.IdentityKindKubernetesEmulator, Emulator: "k3s"})
	require.NoError(t, err)
	id.SetRealm("repo-realm")
	const kubeconfigBody = "apiVersion: v1\nclusters: []\n"
	id.SetEmulatorResolver(&fakeResolver{kubeconfig: []byte(kubeconfigBody)})

	preExisting := filepath.Join(tmp, "preexisting.kubeconfig")
	out, err := id.PrepareEnvironment(context.Background(), map[string]string{"KUBECONFIG": preExisting})
	require.NoError(t, err)

	// The written path is appended to the existing KUBECONFIG, not replacing it.
	parts := strings.Split(out["KUBECONFIG"], string(os.PathListSeparator))
	require.Len(t, parts, 2)
	assert.Equal(t, preExisting, parts[0], "pre-existing KUBECONFIG entry preserved first")
	written := parts[1]
	assert.True(t, strings.HasSuffix(filepath.ToSlash(written), "repo-realm/emulator/local-k8s.kubeconfig"),
		"realm-scoped kubeconfig path, got %q", written)

	data, readErr := os.ReadFile(written)
	require.NoError(t, readErr)
	assert.Equal(t, kubeconfigBody, string(data), "harvested kubeconfig written verbatim")

	// Logout removes the written file.
	require.NoError(t, id.Logout(context.Background()))
	_, statErr := os.Stat(written)
	assert.True(t, os.IsNotExist(statErr), "Logout removes the kubeconfig file")
}

func TestStandaloneIdentity(t *testing.T) {
	id := newAWSIdentity(t)

	// Emulator-bound identities authenticate without an upstream provider step.
	var standalone types.StandaloneIdentity = id
	assert.True(t, standalone.IsStandalone())

	// Emulator identities mint no credentials (the profile is injected at
	// environment-preparation time), so standalone auth returns nil creds.
	creds, err := standalone.AuthenticateStandalone(context.Background())
	require.NoError(t, err)
	assert.Nil(t, creds, "standalone emulator auth mints no credentials")
}

func TestAppendPathList(t *testing.T) {
	sep := string(os.PathListSeparator)
	assert.Equal(t, "a", appendPathList("", "a"))
	assert.Equal(t, "a", appendPathList("a", ""))
	assert.Equal(t, "a"+sep+"b", appendPathList("a", "b"))
	assert.Equal(t, "a"+sep+"b", appendPathList("a"+sep+"b", "a"), "deduplicates existing entries")
}
