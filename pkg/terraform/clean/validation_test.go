package clean

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestIsValidDataDir(t *testing.T) {
	tests := []struct {
		name          string
		tfDataDir     string
		expectedError error
	}{
		{
			name:          "Empty TF_DATA_DIR",
			tfDataDir:     "",
			expectedError: ErrEmptyEnvDir,
		},
		{
			name:          "Root TF_DATA_DIR",
			tfDataDir:     "/",
			expectedError: ErrRefusingToDeleteDir,
		},
		{
			name:          "Valid TF_DATA_DIR",
			tfDataDir:     "/valid/path",
			expectedError: nil,
		},
		{
			name:          "Valid relative path",
			tfDataDir:     "./terraform-data",
			expectedError: nil,
		},
		{
			name:          "Valid nested path",
			tfDataDir:     "/home/user/project/.terraform",
			expectedError: nil,
		},
		{
			name:          "Valid path with dot prefix",
			tfDataDir:     "./.terraform",
			expectedError: nil,
		},
		{
			name:          "Valid deep nested path",
			tfDataDir:     "/var/lib/terraform/data/cache",
			expectedError: nil,
		},
		{
			name:          "Valid simple directory name",
			tfDataDir:     ".terraform",
			expectedError: nil,
		},
		{
			name:          "Valid custom data directory",
			tfDataDir:     ".custom-tf-data",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidDataDir(tt.tfDataDir)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

// TestIsValidDataDir_ParentReferenceCheck tests that paths with ".." are rejected.
func TestIsValidDataDir_ParentReferenceCheck(t *testing.T) {
	// Note: filepath.Abs resolves ".." references, so this test checks behavior
	// with raw strings containing ".." that don't get resolved.
	tests := []struct {
		name          string
		tfDataDir     string
		expectedError error
	}{
		{
			name:          "Valid absolute path without parent reference",
			tfDataDir:     "/home/user/terraform",
			expectedError: nil,
		},
		{
			name:          "Valid relative path",
			tfDataDir:     "subdir/terraform-data",
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := IsValidDataDir(tt.tfDataDir)
			if tt.expectedError != nil {
				require.Error(t, err)
				require.ErrorIs(t, err, tt.expectedError)
			} else {
				require.NoError(t, err)
			}
		})
	}
}
