package aws

import (
	"strings"
	"testing"

	cockroachErrors "github.com/cockroachdb/errors"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// defaultPromptMfaTokenFunc captures the production MFA prompt before any test stubs it.
var defaultPromptMfaTokenFunc = promptMfaTokenFunc

// disableInteractive makes interactive.Available() false deterministically (no real TTY needed).
func disableInteractive(t *testing.T) {
	t.Helper()
	original := viper.GetBool("interactive")
	viper.Set("interactive", false)
	t.Cleanup(func() { viper.Set("interactive", original) })
}

func newPromptTestUser(t *testing.T, name string) *userIdentity {
	t.Helper()
	identity, err := NewUserIdentity(name, &schema.Identity{Kind: "aws/user"})
	require.NoError(t, err)
	return identity.(*userIdentity)
}

func hasHintContaining(err error, fragment string) bool {
	for _, hint := range cockroachErrors.GetAllHints(err) {
		if strings.Contains(hint, fragment) {
			return true
		}
	}
	return false
}

// Regression: the production MFA prompt used to run huh unconditionally; with a captured
// stderr the form was invisible and the process hung. It must now fail fast.
func TestDefaultMfaPrompt_FailsFastWhenNotInteractive(t *testing.T) {
	disableInteractive(t)

	token, err := defaultPromptMfaTokenFunc(&types.AWSCredentials{MfaArn: "arn:aws:iam::123456789012:mfa/user"})

	require.ErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	assert.Empty(t, token)
}

func TestPromptCredentialsGeneric_FailsFastWhenNotInteractive(t *testing.T) {
	disableInteractive(t)

	values, err := promptCredentialsGeneric(types.CredentialPromptSpec{IdentityName: "dev", CloudType: "AWS"})

	require.ErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	assert.Nil(t, values)
}

func TestBuildGetSessionTokenInput_MfaPromptUnavailable(t *testing.T) {
	original := promptMfaTokenFunc
	t.Cleanup(func() { promptMfaTokenFunc = original })
	promptMfaTokenFunc = func(*types.AWSCredentials) (string, error) {
		return "", errUtils.ErrAuthPromptUnavailable
	}
	user := newPromptTestUser(t, "dev-mfa")

	input, err := user.buildGetSessionTokenInput(&types.AWSCredentials{MfaArn: "arn:aws:iam::123456789012:mfa/user"})

	require.ErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	assert.Nil(t, input)
	assert.True(t, hasHintContaining(err, "atmos auth login --identity=dev-mfa"), "hints: %v", cockroachErrors.GetAllHints(err))
}

// Negative path: a failure other than "prompt unavailable" must not be rewritten into the TTY guidance.
func TestBuildGetSessionTokenInput_MfaPromptOtherErrorNotRewritten(t *testing.T) {
	original := promptMfaTokenFunc
	t.Cleanup(func() { promptMfaTokenFunc = original })
	promptMfaTokenFunc = func(*types.AWSCredentials) (string, error) {
		return "", errUtils.ErrAuthenticationFailed
	}
	user := newPromptTestUser(t, "dev-mfa")

	_, err := user.buildGetSessionTokenInput(&types.AWSCredentials{MfaArn: "arn:aws:iam::123456789012:mfa/user"})

	require.ErrorIs(t, err, errUtils.ErrAuthenticationFailed)
	assert.NotErrorIs(t, err, errUtils.ErrAuthPromptUnavailable)
	assert.False(t, hasHintContaining(err, "atmos auth login"))
}

func TestPromptOrError_PromptUnavailableHintsLogin(t *testing.T) {
	original := PromptCredentialsFunc
	t.Cleanup(func() { PromptCredentialsFunc = original })
	PromptCredentialsFunc = func(string, string) (*types.AWSCredentials, error) {
		return nil, errUtils.ErrAuthPromptUnavailable
	}
	user := newPromptTestUser(t, "dev-keys")

	creds, err := user.promptOrError(true, "", "No credentials found", `AWS User credentials not found for identity "dev-keys"`, "atmos auth user configure --identity dev-keys")

	require.ErrorIs(t, err, errUtils.ErrAwsUserNotConfigured)
	assert.Nil(t, creds)
	assert.True(t, hasHintContaining(err, "atmos auth login --identity=dev-keys"), "hints: %v", cockroachErrors.GetAllHints(err))
}
