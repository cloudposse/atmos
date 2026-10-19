package doctor

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFixInSubdirectoryWithoutBasePath covers a configuration that sits below the Git root and declares
// no base_path. Atmos then anchors the project at the Git root, so the project root differs from the
// directory the configuration was found in, and verification must locate the configuration the way the
// first discovery did instead of searching from the moved root.
func TestFixInSubdirectoryWithoutBasePath(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config bool // Select the configuration with a relative --config instead of discovering it.
		dryRun bool
	}{
		{"discovered", false, false},
		{"discovered dry run", false, true},
		{"relative --config", true, false},
		{"relative --config dry run", true, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := newProject(t)
			output, err := gitCommand(repository, "init", "--quiet").CombinedOutput()
			require.NoError(t, err, string(output))
			sub := filepath.Join(repository, "sub")
			original := "components:\n  terraform:\n    init_run_reconfigure: false\n"
			projectFile(t, repository, filepath.Join("sub", "atmos.yaml"), original)

			options := Options{Root: sub, Fix: true, DryRun: tt.dryRun, Offline: true, Only: []string{ruleTerraformReconfigure}}
			if tt.config {
				options.ConfigFiles = []string{"atmos.yaml"}
			}
			r, err := testEngine(t).Run(context.Background(), options)

			require.NoError(t, err)
			found := ecFindings(&r, ruleTerraformReconfigure)
			require.Len(t, found, 1)
			content, readErr := os.ReadFile(filepath.Join(sub, "atmos.yaml"))
			require.NoError(t, readErr)
			if tt.dryRun {
				assert.Equal(t, "available", found[0].Status, found[0].Reason)
				assert.Equal(t, original, string(content), "a dry run must not write")
				return
			}
			assert.Equal(t, statusFixed, found[0].Status, found[0].Reason)
			assert.Contains(t, string(content), "reconfigure: never")
		})
	}
}
