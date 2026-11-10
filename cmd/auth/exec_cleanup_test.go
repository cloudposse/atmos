package auth

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/signals"
)

var _ = schema.Identity{Kind: "aws/assume-role"}

func TestPrepareGitLabAuthExecScope(t *testing.T) {
	for _, kind := range []string{types.ProviderKindGitLabOIDC, "github/oidc", "aws/iam-identity-center"} {
		t.Run(kind, func(t *testing.T) {
			manager := types.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetIdentities().Return(map[string]schema.Identity{"role": {Kind: "aws/assume-role"}})
			manager.EXPECT().GetProviderKindForIdentity("role").Return(kind, nil)
			execCtx := &authExecContext{authManager: manager, identity: "ROLE"}
			t.Cleanup(func() { require.NoError(t, cleanupGitLabAuthExec(execCtx)) })

			err := prepareGitLabAuthExec(execCtx, &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{Keyring: schema.KeyringConfig{Type: types.CredentialStoreTypeMemory}},
			})

			require.NoError(t, err)
			if kind == types.ProviderKindGitLabOIDC {
				require.DirExists(t, execCtx.credentialDir)
				assert.True(t, filepath.IsAbs(execCtx.credentialDir))
			} else {
				assert.Empty(t, execCtx.credentialDir)
			}
		})
	}
}

// No manager logout or integration cleanup may run; unexpected mock calls fail this test.
func TestCleanupGitLabAuthExecPreservesSessions(t *testing.T) {
	manager := types.NewMockAuthManager(gomock.NewController(t))
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mock-credentials"), []byte("synthetic"), 0o600))

	require.NoError(t, cleanupGitLabAuthExec(&authExecContext{authManager: manager, identity: "role", credentialDir: dir}))

	assert.NoDirExists(t, dir)
	for _, execCtx := range []*authExecContext{nil, {}, {identity: "role"}, {authManager: manager}} {
		require.NoError(t, cleanupGitLabAuthExec(execCtx))
	}
}

func TestCleanupGitLabAuthExecReportsFailure(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("synthetic"), 0o600))

	invalidPath := file + "\x00"
	err := cleanupGitLabAuthExec(&authExecContext{credentialDir: invalidPath})

	require.ErrorContains(t, err, "failed to clean up GitLab OIDC credential directory")
	assert.FileExists(t, file)

	resultErr := errUtils.ErrAuthenticationFailed
	cleanupFailedGitLabAuthExec(&authExecContext{credentialDir: invalidPath}, &resultErr)
	assert.ErrorIs(t, resultErr, errUtils.ErrAuthenticationFailed, "cleanup failure must preserve the original setup error")
	assert.ErrorContains(t, resultErr, "failed to clean up GitLab OIDC credential directory")
}

func TestCleanupFailedGitLabAuthExecAfterShutdown(t *testing.T) {
	ctx, release := signals.WithShutdownContext(context.Background())
	t.Cleanup(release)
	released := false
	execCtx := &authExecContext{
		ctx: ctx, credentialDir: filepath.Join(t.TempDir(), "invalid") + "\x00",
		releaseSignals: func() { released = true; release() },
	}
	require.True(t, signals.HandleShutdownSignal(syscall.SIGTERM))
	resultErr := errUtils.ErrAuthenticationFailed

	cleanupFailedGitLabAuthExec(execCtx, &resultErr)

	assert.Equal(t, 143, errUtils.GetExitCode(resultErr))
	assert.ErrorIs(t, resultErr, errUtils.ErrAuthenticationFailed)
	assert.ErrorContains(t, resultErr, "failed to clean up GitLab OIDC credential directory")
	assert.True(t, released, "cleanup failure must still release signal ownership")
}

func TestPrepareGitLabAuthExecUnknownIdentity(t *testing.T) {
	manager := types.NewMockAuthManager(gomock.NewController(t))
	manager.EXPECT().GetIdentities().Return(map[string]schema.Identity{})
	execCtx := &authExecContext{authManager: manager, identity: "another-profile/role"}

	err := prepareGitLabAuthExec(execCtx, &schema.AtmosConfiguration{
		Auth: schema.AuthConfig{Keyring: schema.KeyringConfig{Type: types.CredentialStoreTypeMemory}},
	})

	require.NoError(t, err)
	assert.Empty(t, execCtx.credentialDir, "unresolved identities must reach the existing profile fallback")
}

func TestPrepareGitLabAuthExecSetupErrors(t *testing.T) {
	for _, lookupFailure := range []bool{false, true} {
		t.Run(fmt.Sprint(lookupFailure), func(t *testing.T) {
			manager := types.NewMockAuthManager(gomock.NewController(t))
			manager.EXPECT().GetIdentities().Return(map[string]schema.Identity{"role": {Kind: "aws/assume-role"}})
			var lookupErr error
			if lookupFailure {
				lookupErr = assert.AnError
			}
			manager.EXPECT().GetProviderKindForIdentity("role").Return(types.ProviderKindGitLabOIDC, lookupErr)
			for _, variable := range []string{"TMPDIR", "TMP", "TEMP"} {
				t.Setenv(variable, filepath.Join(t.TempDir(), "missing"))
			}
			execCtx := &authExecContext{authManager: manager, identity: "role"}
			t.Cleanup(func() { require.NoError(t, cleanupGitLabAuthExec(execCtx)) })

			err := prepareGitLabAuthExec(execCtx, &schema.AtmosConfiguration{
				Auth: schema.AuthConfig{Keyring: schema.KeyringConfig{Type: types.CredentialStoreTypeMemory}},
			})

			require.Error(t, err)
			assert.Empty(t, execCtx.credentialDir)
			if lookupFailure {
				assert.ErrorIs(t, err, lookupErr)
			} else {
				assert.ErrorContains(t, err, "failed to create GitLab OIDC credential directory")
			}
		})
	}
}
