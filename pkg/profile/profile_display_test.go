package profile

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDisplayPath(t *testing.T) {
	root := t.TempDir()
	t.Chdir(root)

	assert.Equal(
		t,
		filepath.Join("profiles", "developer"),
		DisplayPath(filepath.Join(root, "profiles", "developer")),
	)

	parent := filepath.Dir(root)
	assert.Equal(
		t,
		filepath.Join(parent, "profiles", "developer"),
		DisplayPath(filepath.Join(parent, "profiles", "developer")),
	)
}
