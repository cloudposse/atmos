package function

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIncludeFunctions_Execute(t *testing.T) {
	tests := []struct {
		name        string
		fn          Function
		args        string
		errContains string
	}{
		{
			name:        "include returns ErrSpecialYAMLHandling",
			fn:          NewIncludeFunction(),
			args:        "path/to/file.yaml",
			errContains: "include",
		},
		{
			name:        "include.raw returns ErrSpecialYAMLHandling",
			fn:          NewIncludeRawFunction(),
			args:        "path/to/file.txt",
			errContains: "include.raw",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := tt.fn.Execute(context.Background(), tt.args, nil)
			require.Error(t, err)
			assert.Nil(t, result)
			assert.ErrorIs(t, err, ErrSpecialYAMLHandling)
			assert.Contains(t, err.Error(), tt.errContains)
		})
	}
}

func TestIncludeFunctions_Metadata(t *testing.T) {
	tests := []struct {
		name         string
		fn           Function
		expectedName string
		expectedTag  string
	}{
		{
			name:         "include function metadata",
			fn:           NewIncludeFunction(),
			expectedName: TagInclude,
			expectedTag:  TagInclude,
		},
		{
			name:         "include.raw function metadata",
			fn:           NewIncludeRawFunction(),
			expectedName: TagIncludeRaw,
			expectedTag:  TagIncludeRaw,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.NotNil(t, tt.fn)
			assert.Equal(t, tt.expectedName, tt.fn.Name())
			assert.Equal(t, PreMerge, tt.fn.Phase())
			assert.Nil(t, tt.fn.Aliases())
		})
	}
}
