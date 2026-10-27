package config

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSelectPhysicalStackFiles(t *testing.T) {
	root := t.TempDir()
	files := []string{filepath.Join(root, "dev.yaml"), filepath.Join(root, "deploy", "prod.yml"), filepath.Join(root, "dev.yml")}
	for _, tc := range []struct {
		stack string
		want  []string
	}{
		{"dev", []string{files[0], files[2]}},
		{"deploy/prod", []string{files[1]}},
		{"prod", files},
		{"logical-name", files},
		{"", files},
	} {
		t.Run(tc.stack, func(t *testing.T) {
			original := append([]string(nil), files...)
			require.Equal(t, tc.want, SelectPhysicalStackFiles(root, tc.stack, files))
			require.Equal(t, original, files, "selection must not mutate discovery shared with other callers")
		})
	}
	require.Equal(t, []string{files[0], files[2]}, SelectPhysicalStackFiles("", "dev", files))
}
