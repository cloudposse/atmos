package aws

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/auth/types"
	pkghttp "github.com/cloudposse/atmos/pkg/http"
)

func TestConsoleURLGenerator_GetConsoleURL(t *testing.T) {
	tests := []struct {
		name             string
		creds            types.ICredentials
		options          types.ConsoleURLOptions
		mockSigninToken  string
		mockHTTPResponse string
		mockHTTPError    error
		expectError      bool
		expectedDuration time.Duration
		validateURL      func(t *testing.T, url string)
	}{
		{
			name: "basic URL generation with default options",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				SessionDuration: 1 * time.Hour,
			},
			mockSigninToken:  "VeryLongSigninTokenString123...",
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString123..."}`,
			expectError:      false,
			expectedDuration: 1 * time.Hour,
			validateURL: func(t *testing.T, generatedURL string) {
				assert.Contains(t, generatedURL, "signin.aws.amazon.com/federation")
				assert.Contains(t, generatedURL, "Action=login")
				assert.Contains(t, generatedURL, "SigninToken=")
				assert.Contains(t, generatedURL, "Issuer=atmos")
				assert.Contains(t, generatedURL, url.QueryEscape(AWSConsoleDestination))
			},
		},
		{
			name: "custom destination",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				Destination:     "https://console.aws.amazon.com/s3",
				SessionDuration: 2 * time.Hour,
				Issuer:          "my-org",
			},
			mockSigninToken:  "VeryLongSigninTokenString456...",
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString456..."}`,
			expectError:      false,
			expectedDuration: 2 * time.Hour,
			validateURL: func(t *testing.T, generatedURL string) {
				assert.Contains(t, generatedURL, "signin.aws.amazon.com/federation")
				assert.Contains(t, generatedURL, "Action=login")
				assert.Contains(t, generatedURL, "SigninToken=")
				assert.Contains(t, generatedURL, "Issuer=my-org")
				assert.Contains(t, generatedURL, url.QueryEscape("https://console.aws.amazon.com/s3"))
			},
		},
		{
			name: "session duration capped at 12 hours",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				SessionDuration: 24 * time.Hour, // Exceeds AWS max.
			},
			mockSigninToken:  "VeryLongSigninTokenString789...",
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString789..."}`,
			expectError:      false,
			expectedDuration: AWSMaxSessionDuration, // Should be capped.
			validateURL: func(t *testing.T, generatedURL string) {
				assert.Contains(t, generatedURL, "signin.aws.amazon.com/federation")
			},
		},
		{
			name: "missing session token",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				// SessionToken missing - permanent IAM user credentials.
			},
			options:     types.ConsoleURLOptions{},
			expectError: true,
		},
		{
			name: "missing access key",
			creds: &types.AWSCredentials{
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options:     types.ConsoleURLOptions{},
			expectError: true,
		},
		{
			name: "missing secret key",
			creds: &types.AWSCredentials{
				AccessKeyID:  "AKIAIOSFODNN7EXAMPLE",
				SessionToken: "FwoGZXIvYXdzEBQaDExample...",
			},
			options:     types.ConsoleURLOptions{},
			expectError: true,
		},
		{
			name:        "wrong credential type",
			creds:       &types.OIDCCredentials{}, // Wrong type.
			options:     types.ConsoleURLOptions{},
			expectError: true,
		},
		{
			name: "federation endpoint returns empty signin token",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				SessionDuration: 1 * time.Hour,
			},
			mockHTTPResponse: `{"SigninToken": ""}`, // Empty token.
			expectError:      true,
		},
		{
			name: "federation endpoint returns invalid JSON",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				SessionDuration: 1 * time.Hour,
			},
			mockHTTPResponse: `{invalid json`, // Malformed JSON.
			expectError:      true,
		},
		{
			name: "HTTP request fails",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				SessionDuration: 1 * time.Hour,
			},
			mockHTTPError: fmt.Errorf("network error"),
			expectError:   true,
		},
		{
			name: "destination alias: s3",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				Destination:     "s3",
				SessionDuration: 1 * time.Hour,
			},
			mockSigninToken:  "VeryLongSigninTokenString...",
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString..."}`,
			expectError:      false,
			expectedDuration: 1 * time.Hour,
			validateURL: func(t *testing.T, generatedURL string) {
				assert.Contains(t, generatedURL, url.QueryEscape("https://console.aws.amazon.com/s3"))
			},
		},
		{
			name: "destination alias: ec2",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				Destination:     "ec2",
				SessionDuration: 1 * time.Hour,
			},
			mockSigninToken:  "VeryLongSigninTokenString...",
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString..."}`,
			expectError:      false,
			expectedDuration: 1 * time.Hour,
			validateURL: func(t *testing.T, generatedURL string) {
				assert.Contains(t, generatedURL, url.QueryEscape("https://console.aws.amazon.com/ec2"))
			},
		},
		{
			name: "destination alias: cloudformation",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				Destination:     "cloudformation",
				SessionDuration: 1 * time.Hour,
			},
			mockSigninToken:  "VeryLongSigninTokenString...",
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString..."}`,
			expectError:      false,
			expectedDuration: 1 * time.Hour,
			validateURL: func(t *testing.T, generatedURL string) {
				assert.Contains(t, generatedURL, url.QueryEscape("https://console.aws.amazon.com/cloudformation"))
			},
		},
		{
			name: "destination alias: uppercase Lambda",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				Destination:     "LAMBDA",
				SessionDuration: 1 * time.Hour,
			},
			mockSigninToken:  "VeryLongSigninTokenString...",
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString..."}`,
			expectError:      false,
			expectedDuration: 1 * time.Hour,
			validateURL: func(t *testing.T, generatedURL string) {
				assert.Contains(t, generatedURL, url.QueryEscape("https://console.aws.amazon.com/lambda"))
			},
		},
		{
			name: "unknown destination alias",
			creds: &types.AWSCredentials{
				AccessKeyID:     "AKIAIOSFODNN7EXAMPLE",
				SecretAccessKey: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				SessionToken:    "FwoGZXIvYXdzEBQaDExample...",
			},
			options: types.ConsoleURLOptions{
				Destination: "invalid-service-name",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHTTPClient := pkghttp.NewMockClient(ctrl)

			// Set up mock HTTP client expectations if we expect HTTP calls.
			if !tt.expectError || tt.mockHTTPError != nil || tt.mockHTTPResponse != "" {
				if tt.mockHTTPError != nil {
					// Simulate HTTP error.
					mockHTTPClient.EXPECT().
						Do(gomock.Any()).
						Return(nil, tt.mockHTTPError).
						AnyTimes()
				} else if tt.mockHTTPResponse != "" {
					// Simulate successful HTTP response.
					responseBody := io.NopCloser(bytes.NewBufferString(tt.mockHTTPResponse))
					mockHTTPClient.EXPECT().
						Do(gomock.Any()).
						Return(&http.Response{
							StatusCode: http.StatusOK,
							Body:       responseBody,
						}, nil).
						AnyTimes()
				}
			}

			generator := NewConsoleURLGenerator(mockHTTPClient)
			ctx := context.Background()

			generatedURL, duration, err := generator.GetConsoleURL(ctx, tt.creds, tt.options)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotEmpty(t, generatedURL)

			if tt.expectedDuration > 0 {
				assert.Equal(t, tt.expectedDuration, duration)
			}

			if tt.validateURL != nil {
				tt.validateURL(t, generatedURL)
			}
		})
	}
}

// TestGetSigninTokenURL_NoSessionDuration verifies that SessionDuration parameter
// is NEVER included in the getSigninToken request URL.
//
// CRITICAL: AWS federation endpoint returns 400 Bad Request when SessionDuration
// is included with role-chained credentials (SSO → PermissionSet → AssumeRole).
//
// Per AWS documentation:
// - SessionDuration is ONLY valid with direct AssumeRole* operations
// - SessionDuration is NOT valid with GetFederationToken
// - SessionDuration is NOT valid with role chaining
//
// Since Atmos uses role chaining, we must NEVER include SessionDuration in the
// getSigninToken request. AWS automatically uses the remaining validity of the
// source credentials (typically 1 hour for role chaining).
//
// This test prevents regression of the bug fixed in commit [hash].
func TestGetSigninTokenURL_NoSessionDuration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := pkghttp.NewMockClient(ctrl)

	// Capture the actual HTTP request to inspect the URL.
	var capturedRequest *http.Request
	mockHTTPClient.EXPECT().
		Do(gomock.Any()).
		DoAndReturn(func(req *http.Request) (*http.Response, error) {
			capturedRequest = req
			responseBody := io.NopCloser(bytes.NewBufferString(`{"SigninToken": "test-token"}`))
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       responseBody,
			}, nil
		})

	generator := NewConsoleURLGenerator(mockHTTPClient)
	ctx := context.Background()

	sessionData := mustMarshal(t, map[string]string{
		"sessionId":    "ASIAQ7VMTGAYPX2VJ2V7",
		"sessionKey":   "test-secret-key",
		"sessionToken": "test-session-token",
	})

	_, err := generator.getSigninToken(ctx, sessionData, 1*time.Hour)
	require.NoError(t, err)
	require.NotNil(t, capturedRequest, "HTTP request should have been made")

	// CRITICAL ASSERTION: SessionDuration must NEVER be in the URL.
	requestURL := capturedRequest.URL.String()
	assert.NotContains(t, requestURL, "SessionDuration",
		"SessionDuration parameter MUST NOT be present in getSigninToken URL (causes 400 Bad Request with role-chained credentials)")

	// Verify required parameters ARE present.
	assert.Contains(t, requestURL, "Action=getSigninToken")
	assert.Contains(t, requestURL, "Session=")
}

// TestLoginURL_NoSessionDuration verifies that SessionDuration parameter
// is NEVER included in the final login URL.
//
// The login URL should only contain: Action, Issuer, Destination, SigninToken.
// SessionDuration belongs ONLY in the getSigninToken request (and even there,
// only for non-role-chained credentials).
//
// This test prevents regression of adding SessionDuration to the login URL.
func TestLoginURL_NoSessionDuration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockHTTPClient := pkghttp.NewMockClient(ctrl)

	// Mock the getSigninToken HTTP call.
	mockHTTPClient.EXPECT().
		Do(gomock.Any()).
		Return(&http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(bytes.NewBufferString(`{"SigninToken": "test-signin-token-123"}`)),
		}, nil)

	generator := NewConsoleURLGenerator(mockHTTPClient)
	ctx := context.Background()

	creds := &types.AWSCredentials{
		AccessKeyID:     "ASIAQ7VMTGAYPX2VJ2V7",
		SecretAccessKey: "test-secret-key",
		SessionToken:    "test-session-token",
	}

	options := types.ConsoleURLOptions{
		Destination:     "https://console.aws.amazon.com/s3",
		SessionDuration: 2 * time.Hour,
		Issuer:          "atmos-test",
	}

	loginURL, _, err := generator.GetConsoleURL(ctx, creds, options)
	require.NoError(t, err)
	require.NotEmpty(t, loginURL)

	// CRITICAL ASSERTION: SessionDuration must NEVER be in the login URL.
	assert.NotContains(t, loginURL, "SessionDuration",
		"SessionDuration parameter MUST NOT be present in login URL (not a valid parameter for Action=login)")

	// Verify required parameters ARE present.
	assert.Contains(t, loginURL, "Action=login")
	assert.Contains(t, loginURL, "Issuer=atmos-test")
	assert.Contains(t, loginURL, "Destination=")
	assert.Contains(t, loginURL, "SigninToken=test-signin-token-123")
}

func TestConsoleURLGenerator_getSigninToken(t *testing.T) {
	tests := []struct {
		name             string
		sessionData      []byte
		duration         time.Duration
		mockHTTPResponse string
		mockHTTPError    error
		mockStatusCode   int
		expectError      bool
		expectedToken    string
	}{
		{
			name: "successful signin token retrieval",
			sessionData: mustMarshal(t, map[string]string{
				"sessionId":    "AKIAIOSFODNN7EXAMPLE",
				"sessionKey":   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"sessionToken": "FwoGZXIvYXdzEBQaDExample...",
			}),
			duration:         1 * time.Hour,
			mockHTTPResponse: `{"SigninToken": "VeryLongSigninTokenString..."}`,
			mockStatusCode:   http.StatusOK,
			expectError:      false,
			expectedToken:    "VeryLongSigninTokenString...",
		},
		{
			name: "HTTP request fails",
			sessionData: mustMarshal(t, map[string]string{
				"sessionId":    "AKIAIOSFODNN7EXAMPLE",
				"sessionKey":   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"sessionToken": "FwoGZXIvYXdzEBQaDExample...",
			}),
			duration:      1 * time.Hour,
			mockHTTPError: fmt.Errorf("network error"),
			expectError:   true,
		},
		{
			name: "HTTP returns non-200 status code",
			sessionData: mustMarshal(t, map[string]string{
				"sessionId":    "AKIAIOSFODNN7EXAMPLE",
				"sessionKey":   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"sessionToken": "FwoGZXIvYXdzEBQaDExample...",
			}),
			duration:         1 * time.Hour,
			mockHTTPResponse: `{"error": "invalid credentials"}`,
			mockStatusCode:   http.StatusForbidden,
			expectError:      true,
		},
		{
			name: "invalid JSON response",
			sessionData: mustMarshal(t, map[string]string{
				"sessionId":    "AKIAIOSFODNN7EXAMPLE",
				"sessionKey":   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"sessionToken": "FwoGZXIvYXdzEBQaDExample...",
			}),
			duration:         1 * time.Hour,
			mockHTTPResponse: `{invalid json`,
			mockStatusCode:   http.StatusOK,
			expectError:      true,
		},
		{
			name: "empty signin token",
			sessionData: mustMarshal(t, map[string]string{
				"sessionId":    "AKIAIOSFODNN7EXAMPLE",
				"sessionKey":   "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
				"sessionToken": "FwoGZXIvYXdzEBQaDExample...",
			}),
			duration:         1 * time.Hour,
			mockHTTPResponse: `{"SigninToken": ""}`,
			mockStatusCode:   http.StatusOK,
			expectError:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockHTTPClient := pkghttp.NewMockClient(ctrl)

			if tt.mockHTTPError != nil {
				// Simulate HTTP error.
				mockHTTPClient.EXPECT().
					Do(gomock.Any()).
					Return(nil, tt.mockHTTPError)
			} else {
				// Simulate HTTP response.
				statusCode := tt.mockStatusCode
				if statusCode == 0 {
					statusCode = http.StatusOK
				}
				responseBody := io.NopCloser(bytes.NewBufferString(tt.mockHTTPResponse))
				mockHTTPClient.EXPECT().
					Do(gomock.Any()).
					Return(&http.Response{
						StatusCode: statusCode,
						Body:       responseBody,
					}, nil)
			}

			generator := NewConsoleURLGenerator(mockHTTPClient)
			ctx := context.Background()

			token, err := generator.getSigninToken(ctx, tt.sessionData, tt.duration)

			if tt.expectError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.Equal(t, tt.expectedToken, token)
		})
	}
}

func TestConsoleURLGenerator_SupportsConsoleAccess(t *testing.T) {
	generator := NewConsoleURLGenerator(nil)
	assert.True(t, generator.SupportsConsoleAccess())
}

func TestNewConsoleURLGenerator(t *testing.T) {
	t.Run("with provided HTTP client", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockHTTPClient := pkghttp.NewMockClient(ctrl)
		generator := NewConsoleURLGenerator(mockHTTPClient)

		assert.NotNil(t, generator)
		assert.Equal(t, mockHTTPClient, generator.httpClient)
	})

	t.Run("with nil HTTP client (uses default)", func(t *testing.T) {
		generator := NewConsoleURLGenerator(nil)

		assert.NotNil(t, generator)
		assert.NotNil(t, generator.httpClient)
	})
}

// mustMarshal is a helper function that marshals data to JSON or fails the test.
func mustMarshal(t *testing.T, v interface{}) []byte {
	data, err := json.Marshal(v)
	require.NoError(t, err)
	return data
}
