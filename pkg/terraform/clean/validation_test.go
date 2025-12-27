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
