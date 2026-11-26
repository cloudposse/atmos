package atmosbridge

import (
	"path/filepath"
	"testing"

	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Terragrunt's copy writes a module manifest into every directory it copies, not only the top one. A
// manifest in a subdirectory, such as the empty state directory a local backend leaves in the unit, is
// copy bookkeeping and never a unit file: exporting it made the working directory differ from a
// reviewed plan.
func TestCollectOverlayFilesDropsNestedCopyManifests(t *testing.T) {
	memory := afero.NewMemMapFs()
	destination := filepath.Join(string(filepath.Separator), "destination")
	for name, content := range map[string]string{
		"main.tf":                                   "# unit file\n",
		".terragrunt-module-manifest":               "manifest",
		"state/.terragrunt-module-manifest":         "manifest",
		"nested/deeper/.terragrunt-module-manifest": "manifest",
		"nested/deeper/keep.txt":                    "data",
	} {
		require.NoError(t, afero.WriteFile(memory, filepath.Join(destination, filepath.FromSlash(name)), []byte(content), 0o644))
	}

	files, _, err := collectOverlayFiles(memory, memory, destination, destination)

	require.NoError(t, err)
	assert.Equal(t, map[string][]byte{"main.tf": []byte("# unit file\n"), "nested/deeper/keep.txt": []byte("data")}, files)
}
