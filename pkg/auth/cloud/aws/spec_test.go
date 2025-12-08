package aws

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

func TestGetFilesBasePath(t *testing.T) {
	tests := []struct {
		name     string
		provider *schema.Provider
		want     string
	}{
		{
			name:     "nil provider",
			provider: nil,
			want:     "",
		},
		{
			name: "nil spec",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: nil,
			},
			want: "",
		},
		{
			name: "no files section",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"other": "value",
				},
			},
			want: "",
		},
		{
			name: "files section without base_path",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"other": "value",
					},
				},
			},
			want: "",
		},
		{
			name: "base_path configured",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "~/.custom/path",
					},
				},
			},
			want: "~/.custom/path",
		},
		{
			name: "base_path with absolute path",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/opt/atmos/credentials",
					},
				},
			},
			want: "/opt/atmos/credentials",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetFilesBasePath(tt.provider)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestValidateFilesBasePath(t *testing.T) {
	tests := []struct {
		name      string
		provider  *schema.Provider
		wantError bool
		errorType error
	}{
		{
			name: "no base_path configured",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{},
			},
			wantError: false,
		},
		{
			name: "valid base_path with tilde",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "~/.custom/path",
					},
				},
			},
			wantError: false,
		},
		{
			name: "valid base_path absolute",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/opt/atmos/credentials",
					},
				},
			},
			wantError: false,
		},
		{
			name: "empty base_path",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "",
					},
				},
			},
			wantError: false, // Empty string means not configured.
		},
		{
			name: "whitespace-only base_path",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "   ",
					},
				},
			},
			wantError: true,
			errorType: errUtils.ErrInvalidProviderConfig,
		},
		{
			name: "base_path with null character",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/path/with\x00null",
					},
				},
			},
			wantError: true,
			errorType: errUtils.ErrInvalidProviderConfig,
		},
		{
			name: "base_path with newline",
			provider: &schema.Provider{
				Kind: "aws/iam-identity-center",
				Spec: map[string]interface{}{
					"files": map[string]interface{}{
						"base_path": "/path/with\nnewline",
					},
				},
			},
			wantError: true,
			errorType: errUtils.ErrInvalidProviderConfig,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateFilesBasePath(tt.provider)
			if tt.wantError {
				assert.Error(t, err)
				if tt.errorType != nil {
					assert.ErrorIs(t, err, tt.errorType)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
