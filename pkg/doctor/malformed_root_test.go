package doctor

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMalformedConfigurationKeepsItsDirectoryAsTheRoot covers a configuration below the Git root that
// does not parse. Its base_path cannot be read, so doctor must not fall back to the Git-root anchor
// that applies to a configuration without base_path: that would inspect the whole repository and
// report the syntax error against the Git root's own atmos.yaml.
func TestMalformedConfigurationKeepsItsDirectoryAsTheRoot(t *testing.T) {
	for _, tt := range []struct {
		name   string
		config bool // Select the configuration with a relative --config instead of discovering it.
	}{
		{"discovered", false},
		{"relative --config", true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			repository := newProject(t)
			output, err := gitCommand(repository, "init", "--quiet").CombinedOutput()
			require.NoError(t, err, string(output))
			projectFile(t, repository, "atmos.yaml", "base_path: ./\nstacks:\n  base_path: stacks\n")
			projectFile(t, repository, filepath.Join("stacks", "unrelated.yaml"), "vars: [unclosed\n")
			sub := filepath.Join(repository, "sub")
			projectFile(t, repository, filepath.Join("sub", "atmos.yaml"), "base_path: ./\nstacks:\n  base_path: [unclosed\n")

			options := Options{Root: sub, Offline: true}
			if tt.config {
				options.ConfigFiles = []string{"atmos.yaml"}
			}
			r, err := testEngine(t).Run(context.Background(), options)

			require.NoError(t, err)
			assert.Equal(t, canonicalPath(sub), canonicalPath(r.SourceRoot), "the root must stay at the configuration directory")
			schema := ecFindings(&r, ruleSchema)
			require.NotEmpty(t, schema)
			for _, f := range schema {
				assert.Equal(t, "atmos.yaml", f.File, "only the selected configuration is inspected: %+v", f)
			}
			var notes []string
			for _, f := range ecFindings(&r, ruleImports) {
				notes = append(notes, f.Reason)
			}
			assert.Contains(t, notes, "base_path could not be read because atmos.yaml does not parse; only its directory is inspected")
		})
	}
}
