package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestValidateAWSCredentials_Success(t *testing.T) {
	// Override the identity function to return success.
	original := getCallerIdentityFn
	getCallerIdentityFn = func(_ context.Context, _ string, _ string, _ time.Duration, _ *schema.AWSAuthContext) (*CallerIdentity, error) {
		return &CallerIdentity{Account: "123456789012", Arn: "arn:aws:iam::123456789012:user/test"}, nil
	}
	t.Cleanup(func() { getCallerIdentityFn = original })

	err := ValidateAWSCredentials(context.Background(), "us-east-1", nil)
	require.NoError(t, err)
}

func TestValidateAWSCredentials_Error_NoAuthCtx(t *testing.T) {
	// Override to return error — no auth context gives default hint.
	original := getCallerIdentityFn
	getCallerIdentityFn = func(_ context.Context, _ string, _ string, _ time.Duration, _ *schema.AWSAuthContext) (*CallerIdentity, error) {
		return nil, errors.New("no credentials found")
	}
	t.Cleanup(func() { getCallerIdentityFn = original })

	err := ValidateAWSCredentials(context.Background(), "", nil)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrAWSCredentialsNotValid)
}

func TestValidateAWSCredentials_Error_WithAuthCtx(t *testing.T) {
	// Override to return error — with auth context.
	original := getCallerIdentityFn
	getCallerIdentityFn = func(_ context.Context, _ string, _ string, _ time.Duration, _ *schema.AWSAuthContext) (*CallerIdentity, error) {
		return nil, errors.New("expired token")
	}
	t.Cleanup(func() { getCallerIdentityFn = original })

	authCtx := &schema.AWSAuthContext{Profile: "test"}
	err := ValidateAWSCredentials(context.Background(), "us-west-2", authCtx)
	require.Error(t, err)
	require.ErrorIs(t, err, errUtils.ErrAWSCredentialsNotValid)
}
