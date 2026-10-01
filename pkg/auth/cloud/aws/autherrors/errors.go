// Package autherrors translates AWS credential failures into provider-neutral errors.
package autherrors

import (
	"errors"

	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/smithy-go"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Normalize preserves the original AWS error while marking unavailable credentials.
// Service, syntax, configuration, and unrelated transport errors stay unchanged.
func Normalize(err error) error {
	defer perf.Track(nil, "autherrors.Normalize")()

	if err == nil || errors.Is(err, errUtils.ErrAuthenticationUnavailable) {
		return err
	}
	if unavailable(err) {
		return errUtils.JoinPreservingHints(errUtils.ErrAuthenticationUnavailable, err)
	}
	return err
}

func unavailable(err error) bool {
	var empty *credentials.StaticCredentialsEmptyError
	if errors.As(err, &empty) || errors.Is(err, errUtils.ErrAWSCredentialsNotValid) {
		return true
	}
	var apiError smithy.APIError
	if !errors.As(err, &apiError) {
		return false
	}
	switch apiError.ErrorCode() {
	case "ExpiredToken", "ExpiredTokenException", "InvalidClientTokenId", "UnrecognizedClientException",
		"InvalidSignatureException", "SignatureDoesNotMatch", "AuthFailure":
		return true
	default:
		return false
	}
}
