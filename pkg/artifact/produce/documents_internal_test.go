package produce

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact/fixtures"
)

func TestOpenChecked_RejectsAFileOtherThanTheInspectedOne(t *testing.T) {
	dir := t.TempDir()
	fixtures.WriteFile(t, dir, "inspected.md", "inspected", 0o644)
	fixtures.WriteFile(t, dir, "replacement.md", "replacement", 0o644)
	inspected, err := os.Lstat(filepath.Join(dir, "inspected.md"))
	require.NoError(t, err)

	tests := []struct {
		name    string
		path    string
		wantErr error
	}{
		{"the inspected file", filepath.Join(dir, "inspected.md"), nil},
		{"a different file at the opened path", filepath.Join(dir, "replacement.md"), errUtils.ErrArtifactSourceChanged},
		{"a directory at the opened path", dir, errUtils.ErrArtifactSourceChanged},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, err := openChecked(tt.path, inspected)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, file)
				return
			}
			require.NoError(t, err)
			require.NoError(t, file.Close())
		})
	}
}
