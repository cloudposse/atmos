package aws

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestGetRDSToken_InvalidCredentials(t *testing.T) {
	// Non-AWS credentials should fail.
	_, _, err := GetRDSToken(context.Background(), nil, "mydb.abc123.us-east-2.rds.amazonaws.com:5432", "us-east-2", "app")
	require.Error(t, err)
	assert.ErrorIs(t, err, errUtils.ErrRDSTokenGeneration)
}

func TestGetRDSToken_Success(t *testing.T) {
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "FwoGZXIvYXdzEBYaDH...",
		Region:          "us-east-2",
	}

	const (
		endpoint = "mydb.abc123.us-east-2.rds.amazonaws.com:5432"
		dbUser   = "app"
	)

	token, expiresAt, err := GetRDSToken(context.Background(), creds, endpoint, "us-east-2", dbUser)
	require.NoError(t, err)

	// The token is a SigV4-presigned RDS "connect" URL used as the DB password.
	// Assert the meaningful parts are present rather than the whole string, since
	// the signature embeds a live X-Amz-Date and is not reproducible.
	assert.Contains(t, token, endpoint, "token should embed the host:port endpoint")
	assert.Contains(t, token, "Action=connect", "token should be an RDS connect request")
	assert.Contains(t, token, "DBUser="+dbUser, "token should carry the DB user")
	assert.Contains(t, token, "X-Amz-Signature=", "token should be SigV4-signed")

	// Expiration is the AWS-fixed ~15-minute window.
	assert.True(t, expiresAt.After(time.Now()), "expiry should be in the future")
	assert.WithinDuration(t, time.Now().Add(15*time.Minute), expiresAt, time.Minute)
}

func TestGetRDSToken_RegionFromCredentials(t *testing.T) {
	// When the region argument is empty, the credentials' region is used for signing.
	// The endpoint host deliberately does NOT contain the region, so asserting the token carries
	// "eu-west-1" exercises the SigV4 signing region (via the credential scope
	// .../eu-west-1/rds-db/aws4_request) rather than trivially matching the hostname.
	creds := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		Region:          "eu-west-1",
	}

	token, _, err := GetRDSToken(context.Background(), creds, "mydb.internal.example:5432", "", "app")
	require.NoError(t, err)
	assert.True(t, strings.Contains(token, "eu-west-1"), "token should be signed for the credentials' region")
}
