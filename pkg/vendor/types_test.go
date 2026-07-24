package vendor

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPkgType_String(t *testing.T) {
	tests := []struct {
		name     string
		pkgType  pkgType
		expected string
	}{
		{
			name:     "remote type",
			pkgType:  pkgTypeRemote,
			expected: "remote",
		},
		{
			name:     "oci type",
			pkgType:  pkgTypeOci,
			expected: "oci",
		},
		{
			name:     "local type",
			pkgType:  pkgTypeLocal,
			expected: "local",
		},
		{
			name:     "unknown type - negative",
			pkgType:  pkgType(-1),
			expected: "unknown",
		},
		{
			name:     "unknown type - out of range",
			pkgType:  pkgType(100),
			expected: "unknown",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.pkgType.String()
			assert.Equal(t, tt.expected, result)
		})
	}
}
