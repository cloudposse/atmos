package autherrors

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestNormalize(t *testing.T) {
	for _, code := range []string{"ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException", "InvalidSignatureException", "SignatureDoesNotMatch", "AuthFailure"} {
		t.Run(code, func(t *testing.T) {
			original := &smithy.GenericAPIError{Code: code, Message: "unavailable"}
			err := Normalize(fmt.Errorf("SDK operation: %w", original))
			require.ErrorIs(t, err, errUtils.ErrAuthenticationUnavailable)
			require.ErrorIs(t, err, original)
		})
	}
	require.ErrorIs(t, Normalize(&credentials.StaticCredentialsEmptyError{}), errUtils.ErrAuthenticationUnavailable)
	require.Nil(t, Normalize(nil))
	for _, err := range []error{errors.New("network unavailable"), &smithy.GenericAPIError{Code: "ValidationException"}, errUtils.ErrInvalidAuthConfig} {
		require.Same(t, err, Normalize(err))
	}
}

func TestNormalizePreservesAuthorizationFailures(t *testing.T) {
	for _, code := range []string{"AccessDenied", "AccessDeniedException"} {
		t.Run(code, func(t *testing.T) {
			original := fmt.Errorf("SDK operation: %w", &smithy.GenericAPIError{Code: code, Message: "permission denied"})
			require.Same(t, original, Normalize(original))
			require.NotErrorIs(t, Normalize(original), errUtils.ErrAuthenticationUnavailable)
		})
	}
}
