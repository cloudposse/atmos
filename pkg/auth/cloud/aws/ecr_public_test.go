package aws

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ecrpublic"
	ecrpublictypes "github.com/aws/aws-sdk-go-v2/service/ecrpublic/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/auth/types"
)

func TestECRPublicConstants(t *testing.T) {
	assert.Equal(t, "public.ecr.aws", ECRPublicRegistryURL)
	assert.Equal(t, "us-east-1", ECRPublicAuthRegion)
}

func TestValidateECRPublicRegion(t *testing.T) {
	tests := []struct {
		name        string
		region      string
		expectError bool
	}{
		{
			name:        "us-east-1 is valid",
			region:      "us-east-1",
			expectError: false,
		},
		{
			name:        "us-west-2 is valid",
			region:      "us-west-2",
			expectError: false,
		},
		{
			name:        "eu-west-1 is invalid",
			region:      "eu-west-1",
			expectError: true,
		},
		{
			name:        "ap-southeast-1 is invalid",
			region:      "ap-southeast-1",
			expectError: true,
		},
		{
			name:        "cn-north-1 is invalid",
			region:      "cn-north-1",
			expectError: true,
		},
		{
			name:        "us-gov-west-1 is invalid",
			region:      "us-gov-west-1",
			expectError: true,
		},
		{
			name:        "empty string is invalid",
			region:      "",
			expectError: true,
		},
		{
			name:        "us-east-2 is invalid",
			region:      "us-east-2",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateECRPublicRegion(tt.region)
			if tt.expectError {
				assert.ErrorIs(t, err, errUtils.ErrECRPublicInvalidRegion)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestIsECRPublicRegistry(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		expected bool
	}{
		{
			name:     "exact match",
			url:      "public.ecr.aws",
			expected: true,
		},
		{
			name:     "with https prefix",
			url:      "https://public.ecr.aws",
			expected: true,
		},
		{
			name:     "with path",
			url:      "public.ecr.aws/cloudposse/atmos",
			expected: true,
		},
		{
			name:     "with https and path",
			url:      "https://public.ecr.aws/cloudposse/atmos",
			expected: true,
		},
		{
			name:     "private ECR registry",
			url:      "123456789012.dkr.ecr.us-east-1.amazonaws.com",
			expected: false,
		},
		{
			name:     "Docker Hub",
			url:      "docker.io/library/nginx",
			expected: false,
		},
		{
			name:     "GitHub Container Registry",
			url:      "ghcr.io/owner/repo",
			expected: false,
		},
		{
			name:     "empty string",
			url:      "",
			expected: false,
		},
		{
			name:     "partial match - wrong prefix",
			url:      "notpublic.ecr.aws",
			expected: false,
		},
		{
			name:     "partial match - different domain",
			url:      "public.ecr.aws.evil.com",
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsECRPublicRegistry(tt.url)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetPublicAuthorizationToken(t *testing.T) {
	ctx := context.Background()
	validExpiry := time.Now().Add(12 * time.Hour)
	validToken := base64.StdEncoding.EncodeToString([]byte("AWS:test-password"))

	tests := []struct {
		name        string
		setupMock   func(ctrl *gomock.Controller) *MockECRPublicClient
		useMock     bool
		creds       types.ICredentials
		wantErr     bool
		errContains string
		errIs       error
		checkResult func(t *testing.T, result *ECRPublicAuthResult)
	}{
		{
			name:    "success with valid token and expiry",
			useMock: true,
			creds:   &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", Region: "us-east-1"},
			setupMock: func(ctrl *gomock.Controller) *MockECRPublicClient {
				mock := NewMockECRPublicClient(ctrl)
				mock.EXPECT().GetAuthorizationToken(gomock.Any(), gomock.Any()).Return(
					&ecrpublic.GetAuthorizationTokenOutput{
						AuthorizationData: &ecrpublictypes.AuthorizationData{
							AuthorizationToken: &validToken,
							ExpiresAt:          &validExpiry,
						},
					}, nil)
				return mock
			},
			checkResult: func(t *testing.T, result *ECRPublicAuthResult) {
				assert.Equal(t, "AWS", result.Username)
				assert.Equal(t, "test-password", result.Password)
				assert.Equal(t, validExpiry, result.ExpiresAt)
			},
		},
		{
			name:    "success with nil ExpiresAt",
			useMock: true,
			creds:   &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", Region: "us-east-1"},
			setupMock: func(ctrl *gomock.Controller) *MockECRPublicClient {
				mock := NewMockECRPublicClient(ctrl)
				mock.EXPECT().GetAuthorizationToken(gomock.Any(), gomock.Any()).Return(
					&ecrpublic.GetAuthorizationTokenOutput{
						AuthorizationData: &ecrpublictypes.AuthorizationData{
							AuthorizationToken: &validToken,
						},
					}, nil)
				return mock
			},
			checkResult: func(t *testing.T, result *ECRPublicAuthResult) {
				assert.Equal(t, "AWS", result.Username)
				assert.Equal(t, "test-password", result.Password)
				assert.True(t, result.ExpiresAt.IsZero())
			},
		},
		{
			name:    "nil credentials returns error",
			useMock: false,
			creds:   nil,
			wantErr: true,
			errIs:   errUtils.ErrECRPublicAuthFailed,
		},
		{
			name:    "non-AWS credentials returns error",
			useMock: false,
			// mockNonAWSCredentials is defined in ecr_extended_test.go (same package).
			creds:   &mockNonAWSCredentials{},
			wantErr: true,
			errIs:   errUtils.ErrECRPublicAuthFailed,
		},
		{
			name:    "API call error",
			useMock: true,
			creds:   &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", Region: "us-east-1"},
			setupMock: func(ctrl *gomock.Controller) *MockECRPublicClient {
				mock := NewMockECRPublicClient(ctrl)
				mock.EXPECT().GetAuthorizationToken(gomock.Any(), gomock.Any()).Return(
					nil, fmt.Errorf("access denied"))
				return mock
			},
			wantErr: true,
			errIs:   errUtils.ErrECRPublicAuthFailed,
		},
		{
			name:    "nil AuthorizationData",
			useMock: true,
			creds:   &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", Region: "us-east-1"},
			setupMock: func(ctrl *gomock.Controller) *MockECRPublicClient {
				mock := NewMockECRPublicClient(ctrl)
				mock.EXPECT().GetAuthorizationToken(gomock.Any(), gomock.Any()).Return(
					&ecrpublic.GetAuthorizationTokenOutput{
						AuthorizationData: nil,
					}, nil)
				return mock
			},
			wantErr:     true,
			errIs:       errUtils.ErrECRPublicAuthFailed,
			errContains: "no authorization data",
		},
		{
			name:    "nil AuthorizationToken",
			useMock: true,
			creds:   &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", Region: "us-east-1"},
			setupMock: func(ctrl *gomock.Controller) *MockECRPublicClient {
				mock := NewMockECRPublicClient(ctrl)
				mock.EXPECT().GetAuthorizationToken(gomock.Any(), gomock.Any()).Return(
					&ecrpublic.GetAuthorizationTokenOutput{
						AuthorizationData: &ecrpublictypes.AuthorizationData{
							AuthorizationToken: nil,
						},
					}, nil)
				return mock
			},
			wantErr:     true,
			errIs:       errUtils.ErrECRPublicAuthFailed,
			errContains: "no authorization data",
		},
		{
			name:    "invalid base64 token",
			useMock: true,
			creds:   &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", Region: "us-east-1"},
			setupMock: func(ctrl *gomock.Controller) *MockECRPublicClient {
				mock := NewMockECRPublicClient(ctrl)
				badToken := "not-valid-base64!!!"
				mock.EXPECT().GetAuthorizationToken(gomock.Any(), gomock.Any()).Return(
					&ecrpublic.GetAuthorizationTokenOutput{
						AuthorizationData: &ecrpublictypes.AuthorizationData{
							AuthorizationToken: &badToken,
						},
					}, nil)
				return mock
			},
			wantErr:     true,
			errIs:       errUtils.ErrECRPublicAuthFailed,
			errContains: "decode",
		},
		{
			name:    "token without colon separator",
			useMock: true,
			creds:   &types.AWSCredentials{AccessKeyID: "AKIA", SecretAccessKey: "secret", Region: "us-east-1"},
			setupMock: func(ctrl *gomock.Controller) *MockECRPublicClient {
				mock := NewMockECRPublicClient(ctrl)
				noColonToken := base64.StdEncoding.EncodeToString([]byte("justpassword"))
				mock.EXPECT().GetAuthorizationToken(gomock.Any(), gomock.Any()).Return(
					&ecrpublic.GetAuthorizationTokenOutput{
						AuthorizationData: &ecrpublictypes.AuthorizationData{
							AuthorizationToken: &noColonToken,
						},
					}, nil)
				return mock
			},
			wantErr:     true,
			errIs:       errUtils.ErrECRPublicAuthFailed,
			errContains: "invalid token format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var opts []ECRPublicAuthOption
			if tt.useMock && tt.setupMock != nil {
				ctrl := gomock.NewController(t)
				mockClient := tt.setupMock(ctrl)
				opts = append(opts, WithECRPublicClient(mockClient))
			}

			result, err := GetPublicAuthorizationToken(ctx, tt.creds, opts...)

			if !tt.wantErr {
				require.NoError(t, err)
				require.NotNil(t, result)
				if tt.checkResult != nil {
					tt.checkResult(t, result)
				}
				return
			}

			require.Error(t, err)
			assert.Nil(t, result)
			if tt.errIs != nil {
				assert.ErrorIs(t, err, tt.errIs)
			}
			if tt.errContains != "" {
				assert.Contains(t, err.Error(), tt.errContains)
			}
		})
	}
}
