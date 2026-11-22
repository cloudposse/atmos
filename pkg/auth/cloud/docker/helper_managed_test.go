package docker

// Tests for registries routed to a credential helper that cannot store, such
// as docker-credential-ecr-login (amazon-ecr-credential-helper), which gets
// ECR credentials itself and returns "not implemented" from store and erase.

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestIsNotImplementedReply(t *testing.T) {
	for _, tt := range []struct {
		name, stdout, stderr string
		want                 bool
	}{
		{name: "stdout reply", stdout: "not implemented\n", want: true},
		{name: "stderr reply", stderr: "not implemented\n", want: true},
		{name: "case and spacing", stdout: "  Not Implemented \r\n", want: true},
		{name: "prefixed reply", stdout: "error: not implemented", want: true},
		{name: "not found", stdout: helperNotFoundMessage},
		{name: "genuine failure", stdout: "keychain is locked", stderr: "diagnostics"},
		{name: "mentions it mid-sentence", stdout: "not implemented for this keychain, retry later"},
		{name: "empty"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isNotImplementedReply(tt.stdout, tt.stderr))
		})
	}
}

func TestExecCredentialHelper_StoreNotImplemented(t *testing.T) {
	for _, mode := range []string{"notimplemented", "notimplemented-stderr"} {
		t.Run(mode, func(t *testing.T) {
			installFakeCredentialHelper(t, mode)

			err := newExecCredentialHelper().Store(fakeHelperName, testECRRegistry, testECRUser, testECRPassword)

			require.Error(t, err)
			assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
			assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelperStoreUnsupported)
		})
	}
}

// TestExecCredentialHelper_StoreFailureIsNotUnsupported is the negative path:
// a genuine helper failure is not classified as unsupported.
func TestExecCredentialHelper_StoreFailureIsNotUnsupported(t *testing.T) {
	installFakeCredentialHelper(t, "fail")

	err := newExecCredentialHelper().Store(fakeHelperName, testECRRegistry, testECRUser, testECRPassword)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelper)
	assert.NotErrorIs(t, err, errUtils.ErrDockerCredentialHelperStoreUnsupported)
}

// TestExecCredentialHelper_EraseNotImplemented verifies that erasing from a
// helper that cannot store is a no-op: it holds nothing of ours.
func TestExecCredentialHelper_EraseNotImplemented(t *testing.T) {
	installFakeCredentialHelper(t, "notimplemented")

	assert.NoError(t, newExecCredentialHelper().Erase(fakeHelperName, testECRRegistry))
}

// TestConfigManager_WriteAuth_HelperManagedRegistry is the ecr-login case end
// to end: WriteAuth reports a helper-managed registry, HelperManagedNotice
// explains it, and config.json is left exactly as it was.
func TestConfigManager_WriteAuth_HelperManagedRegistry(t *testing.T) {
	installFakeCredentialHelper(t, "notimplemented")
	content := `{"credHelpers": {"` + testECRRegistry + `": "` + fakeHelperName + `"}}`
	manager := writeTestConfig(t, content, 0o600)

	err := manager.WriteAuth("https://"+testECRRegistry+"/", testECRUser, testECRPassword)

	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrDockerCredentialHelperStoreUnsupported)
	notice, managed := HelperManagedNotice(err)
	require.True(t, managed)
	assert.Equal(t, "Docker uses docker-credential-"+fakeHelperName+" for "+testECRRegistry+
		", which gets registry credentials itself, so Atmos did not write any to config.json", notice)
	assert.Equal(t, content, string(readConfigFile(t, manager)), "config.json must be untouched")

	// A wrapped error is still recognized, as callers wrap WriteAuth errors.
	_, managed = HelperManagedNotice(fmt.Errorf("%w: %w", errUtils.ErrDockerConfigWrite, err))
	assert.True(t, managed)
}

// TestHelperManagedNotice_OtherErrors is the negative path: a genuinely
// failing helper and unrelated errors are not helper-managed.
func TestHelperManagedNotice_OtherErrors(t *testing.T) {
	installFakeCredentialHelper(t, "fail")
	manager := writeTestConfig(t, `{"credsStore": "`+fakeHelperName+`"}`, 0o600)
	err := manager.WriteAuth(testECRRegistry, testECRUser, testECRPassword)
	require.Error(t, err)

	for _, candidate := range []error{nil, errors.New("boom"), errUtils.ErrDockerCredentialHelperStoreUnsupported, err} {
		notice, managed := HelperManagedNotice(candidate)
		assert.False(t, managed, "error %v", candidate)
		assert.Empty(t, notice)
	}
}

// TestConfigManager_RemoveAuth_NotImplementedEraseRemovesEntry verifies that
// logout still removes the registry's auths entry when its helper cannot
// erase (it never stored anything).
func TestConfigManager_RemoveAuth_NotImplementedEraseRemovesEntry(t *testing.T) {
	installFakeCredentialHelper(t, "notimplemented")
	manager := writeTestConfig(t, `{"credsStore": "`+fakeHelperName+`", "auths": {"`+testECRRegistry+`": {}}}`, 0o600)

	require.NoError(t, manager.RemoveAuth(testECRRegistry))

	got := decodeGeneric(t, readConfigFile(t, manager))
	assert.Empty(t, got["auths"])
	assert.Equal(t, fakeHelperName, got["credsStore"])
}
