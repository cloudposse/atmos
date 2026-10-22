package plugin

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDisplayPath(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	outside := filepath.Join(filepath.Dir(directory), "elsewhere", "bin", "plugin")

	tests := []struct {
		name, path, want string
	}{
		{"nested under the working directory is relative", filepath.Join(directory, "corporate-oidc", "bin", "plugin"), filepath.Join("corporate-oidc", "bin", "plugin")},
		{"the working directory itself", directory, "."},
		{"outside the working directory stays absolute", outside, outside},
		{"a relative path is cleaned", filepath.Join(".", "a", "..", "b"), "b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, displayPath(tt.path))
		})
	}
}
