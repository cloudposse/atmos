package aws

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/aws/smithy-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
	"github.com/cloudposse/atmos/pkg/schema"
)

// stsErrorServer returns an httptest server that responds to every request
// with an AWS-style XML error envelope. The SDK's smithy XML decoder turns
// this into a *smithy.GenericAPIError carrying the supplied code and message.
//
// The httpStatus argument selects the HTTP status code (e.g. 400 for client
// errors). The AWS SDK is sensitive to status — non-2xx is required for the
// smithy framework to treat the response as an error. The fault is hard-coded
// to "Sender" because every regression test in this file exercises a
// client-side STS rejection.
func stsErrorServer(t *testing.T, httpStatus int, errorCode, errorMessage string) *httptest.Server {
	t.Helper()
	body := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<ErrorResponse xmlns="https://sts.amazonaws.com/doc/2011-06-15/">
  <Error>
    <Type>Sender</Type>
    <Code>%s</Code>
    <Message>%s</Message>
  </Error>
  <RequestId>test-request-id-fixed</RequestId>
</ErrorResponse>`, errorCode, errorMessage)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(httpStatus)
		_, _ = w.Write([]byte(body))
	}))
}

// resolverCredentials returns a Credentials map that points the AWS SDK
// at a custom endpoint URL via the existing resolver mechanism in
// pkg/auth/cloud/aws/resolver.go.
func resolverCredentials(url string) map[string]interface{} {
	return map[string]interface{}{
		"aws": map[string]interface{}{
			"resolver": map[string]interface{}{
				"url": url,
			},
		},
	}
}

// TestAssumeRoleIdentity_Authenticate_StandardAssumeRole_PreservesSDKError
// verifies that when the underlying STS AssumeRole call fails, the AWS-side
// error code and message are preserved in the returned error chain rather
// than being swallowed by the enriched ErrAuthenticationFailed sentinel.
//
// This is the regression test for the bug fixed in this commit: prior to
// the WithCause(err) addition, the operator only saw "authentication failed"
// with no AWS context, making it impossible to distinguish AccessDenied
// from NoSuchEntity, expired tokens, etc.
func TestAssumeRoleIdentity_Authenticate_StandardAssumeRole_PreservesSDKError(t *testing.T) {
	server := stsErrorServer(t, http.StatusForbidden, "AccessDenied",
		"User: arn:aws:iam::111111111111:user/test is not authorized to perform: sts:AssumeRole on resource: arn:aws:iam::222222222222:role/MismatchedTrust")
	defer server.Close()

	identity := &assumeRoleIdentity{
		name: "test-role",
		config: &schema.Identity{
			Kind: "aws/assume-role",
			Via:  &schema.IdentityVia{Provider: "test-provider"},
			Principal: map[string]interface{}{
				"assume_role": "arn:aws:iam::222222222222:role/MismatchedTrust",
				"region":      "us-east-1",
			},
			Credentials: resolverCredentials(server.URL),
		},
	}
	require.NoError(t, identity.Validate())

	awsBase := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "test-session-token",
		Region:          "us-east-1",
	}

	_, err := identity.Authenticate(context.Background(), awsBase)
	require.Error(t, err)

	// Sentinel must still match — WithCause preserves it.
	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed,
		"sentinel ErrAuthenticationFailed must remain reachable via errors.Is")

	// AWS SDK error code and message must surface in the rendered error.
	errMsg := err.Error()
	assert.Contains(t, errMsg, "AccessDenied",
		"AWS error code must be preserved in the error chain")
	assert.Contains(t, errMsg, "MismatchedTrust",
		"AWS error message detail must be preserved in the error chain")

	// The smithy.APIError interface should also be reachable through the chain.
	var apiErr smithy.APIError
	assert.True(t, errors.As(err, &apiErr),
		"underlying smithy.APIError must be reachable via errors.As")
	if apiErr != nil {
		assert.Equal(t, "AccessDenied", apiErr.ErrorCode())
	}
}

// TestAssumeRoleIdentity_Authenticate_WebIdentity_PreservesSDKError verifies
// the same property on the AssumeRoleWithWebIdentity path used by OIDC-based
// CI flows (GitHub Actions, GitLab CI, etc.). Each subtest exercises a
// different STS error code to confirm the chain isn't hard-coded to one —
// any code the SDK surfaces should pass through.
func TestAssumeRoleIdentity_Authenticate_WebIdentity_PreservesSDKError(t *testing.T) {
	// Strip ambient AWS env so the SDK must use the (anonymous) credentials
	// path that AssumeRoleWithWebIdentity sets up internally. Hoisted to the
	// parent so the cleanup runs once after all subtests; subtests are
	// sequential, so this is safe.
	t.Setenv("AWS_ACCESS_KEY_ID", "")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "")
	t.Setenv("AWS_SESSION_TOKEN", "")

	tests := []struct {
		name             string
		httpStatus       int
		errorCode        string
		errorMessage     string
		oidcToken        string
		expectSubstrings []string
	}{
		{
			name:         "AccessDenied",
			httpStatus:   http.StatusForbidden,
			errorCode:    "AccessDenied",
			errorMessage: "Not authorized to perform sts:AssumeRoleWithWebIdentity",
			// JWT shape doesn't matter — the test server doesn't validate it.
			oidcToken: "eyJhbGciOiJSUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiJ0ZXN0In0.sig",
			// Operation name in the substring list pins both the SDK code
			// AND the rendered operation context to the chained error.
			expectSubstrings: []string{"AccessDenied", "AssumeRoleWithWebIdentity"},
		},
		{
			name:             "InvalidIdentityToken",
			httpStatus:       http.StatusBadRequest,
			errorCode:        "InvalidIdentityToken",
			errorMessage:     "The web identity token that was passed could not be validated",
			oidcToken:        "garbage.token.value",
			expectSubstrings: []string{"InvalidIdentityToken"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := stsErrorServer(t, tc.httpStatus, tc.errorCode, tc.errorMessage)
			defer server.Close()

			identity := &assumeRoleIdentity{
				name: "github-role",
				config: &schema.Identity{
					Kind: "aws/assume-role",
					Via:  &schema.IdentityVia{Provider: "github-oidc"},
					Principal: map[string]interface{}{
						"assume_role": "arn:aws:iam::111111111111:role/GitHubActionsRole",
						"region":      "us-east-1",
					},
					Credentials: resolverCredentials(server.URL),
				},
			}
			require.NoError(t, identity.Validate())

			oidcCreds := &types.OIDCCredentials{
				Token:    tc.oidcToken,
				Provider: "github",
				Audience: "sts.amazonaws.com",
			}

			_, err := identity.Authenticate(context.Background(), oidcCreds)
			require.Error(t, err)

			// Sentinel must still match — WithCause preserves it.
			assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed,
				"sentinel ErrAuthenticationFailed must remain reachable via errors.Is")

			// Every expected substring (SDK code, operation name, ...) must
			// surface in the rendered error.
			errMsg := err.Error()
			for _, s := range tc.expectSubstrings {
				assert.Contains(t, errMsg, s,
					"SDK error substring %q must be preserved in the error chain", s)
			}

			// The smithy.APIError interface should be reachable through the
			// chain, and its ErrorCode must equal the SDK code we returned.
			var apiErr smithy.APIError
			if assert.True(t, errors.As(err, &apiErr),
				"underlying smithy.APIError must be reachable via errors.As") {
				assert.Equal(t, tc.errorCode, apiErr.ErrorCode())
			}
		})
	}
}

// TestAssumeRootIdentity_Authenticate_PreservesSDKError mirrors the assume-role
// tests but for the AssumeRoot path used by centralized root access via
// Identity Center permission sets.
func TestAssumeRootIdentity_Authenticate_PreservesSDKError(t *testing.T) {
	server := stsErrorServer(t, http.StatusForbidden, "AccessDenied",
		"User: arn:aws:sts::111111111111:assumed-role/PermSet/test is not authorized to perform: sts:AssumeRoot on resource: arn:aws:iam::222222222222:root")
	defer server.Close()

	identity := &assumeRootIdentity{
		name: "root-access",
		config: &schema.Identity{
			Kind: "aws/assume-root",
			Via:  &schema.IdentityVia{Identity: "permset"},
			Principal: map[string]interface{}{
				"target_principal": "222222222222",
				"task_policy_arn":  "arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
				"region":           "us-east-1",
			},
			Credentials: resolverCredentials(server.URL),
		},
	}
	require.NoError(t, identity.Validate())

	awsBase := &types.AWSCredentials{
		AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
		SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		SessionToken:    "test-session-token",
		Region:          "us-east-1",
	}

	_, err := identity.Authenticate(context.Background(), awsBase)
	require.Error(t, err)

	assert.ErrorIs(t, err, errUtils.ErrAuthenticationFailed,
		"sentinel ErrAuthenticationFailed must remain reachable via errors.Is")

	errMsg := err.Error()
	assert.Contains(t, errMsg, "AccessDenied",
		"AWS error code must be preserved in the error chain")
	assert.Contains(t, errMsg, "AssumeRoot",
		"AWS error message detail must be preserved in the error chain")

	var apiErr smithy.APIError
	assert.True(t, errors.As(err, &apiErr),
		"underlying smithy.APIError must be reachable via errors.As")
	if apiErr != nil {
		assert.Equal(t, "AccessDenied", apiErr.ErrorCode())
	}
}
