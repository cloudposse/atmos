package aws

import (
	"context"
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func joinedHints(err error) string {
	return strings.Join(cockroachErrors.GetAllHints(err), "\n")
}

// The PRD error table promises an actionable hint for every credential_process failure mode.
func TestRetrieveProcessCredentials_FailureHints(t *testing.T) {
	t.Run("helper failure hints to run the command manually", func(t *testing.T) {
		_, _, err := retrieveWithHelper(t, []string{testCredentialProcessExitEnv + "=3"})

		require.ErrorIs(t, err, errUtils.ErrCredentialProcessFailed)
		hints := joinedHints(err)
		assert.Contains(t, hints, "manually")
		assert.Contains(t, hints, `"corp-base"`, "the hint names the identity")
		assert.Contains(t, err.Error(), "corp-base")
	})

	t.Run("invalid output hints at the expected document and never echoes stdout", func(t *testing.T) {
		_, _, err := retrieveWithHelper(t, []string{testCredentialProcessJSONEnv + `={"Version":2,"AccessKeyId":"` + testAccessKeyValue +
			`","SecretAccessKey":"` + testSecretValue + `"}`})

		require.ErrorIs(t, err, errUtils.ErrCredentialProcessInvalidOutput)
		hints := joinedHints(err)
		assert.Contains(t, hints, "version 1 process-credential JSON document")
		assert.Contains(t, hints, "manually")
		for _, secret := range []string{testSecretValue, testAccessKeyValue, testSessionValue} {
			assert.NotContains(t, hints, secret)
			assert.NotContains(t, err.Error(), secret)
		}
	})

	t.Run("recursion hints at a different identity", func(t *testing.T) {
		_, err := RetrieveProcessCredentials(context.Background(), "corp-base", "fake-helper",
			WithCredentialProcessEnviron([]string{CredentialProcessChainEnvVar + "=corp-base"}))

		require.ErrorIs(t, err, errUtils.ErrCredentialProcessRecursion)
		assert.Contains(t, joinedHints(err), "Point the helper at a different identity")
	})
}

func TestNewInvalidProcessOutputError(t *testing.T) {
	err := NewInvalidProcessOutputError("corp-base", "AccessKeyId is missing")

	require.ErrorIs(t, err, errUtils.ErrCredentialProcessInvalidOutput)
	assert.Contains(t, err.Error(), "corp-base")
	assert.Contains(t, err.Error(), "AccessKeyId is missing")
	assert.Contains(t, joinedHints(err), "manually")
}
