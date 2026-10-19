package testhelpers

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDefaultTools covers loadToolVersionPins' mapping into DefaultTools: repository-provided
// pins, missing/malformed .tool-versions entries falling back to defaultToolVersions, and
// FindRepoRoot failing to locate a repo root at all. Each subtest runs from its own temp
// directory (via t.Chdir) so FindRepoRoot's upward walk for a .git directory never crosses into
// the real checkout.
func TestDefaultTools(t *testing.T) {
	const (
		pinnedTofu      = "1.99.0"
		pinnedTerraform = "1.88.0"
	)

	tests := []struct {
		name            string
		withGitDir      bool
		toolVersions    string // Written to .tool-versions when non-empty; omitted entirely when "".
		wantOpentofu    string
		wantTerraform   string
		wantAllFallback bool // True when every tool should resolve to defaultToolVersions.
	}{
		{
			name:            "no repo root found falls back entirely",
			withGitDir:      false,
			toolVersions:    "",
			wantAllFallback: true,
		},
		{
			name:            "repo root found but .tool-versions missing falls back entirely",
			withGitDir:      true,
			toolVersions:    "",
			wantAllFallback: true,
		},
		{
			name:       "malformed .tool-versions falls back entirely",
			withGitDir: true,
			// A line missing its version makes toolchain.LoadToolVersions return an error.
			toolVersions:    "opentofu/opentofu\n",
			wantAllFallback: true,
		},
		{
			name:       "partial pins fall back only for missing entries",
			withGitDir: true,
			toolVersions: "opentofu/opentofu " + pinnedTofu + "\n" +
				"hashicorp/terraform " + pinnedTerraform + "\n",
			wantOpentofu:  pinnedTofu,
			wantTerraform: pinnedTerraform,
		},
	}

	const (
		dirPerm  = 0o755
		filePerm = 0o644
	)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.withGitDir {
				require.NoError(t, os.Mkdir(filepath.Join(dir, ".git"), dirPerm))
			}
			if tt.toolVersions != "" {
				require.NoError(t, os.WriteFile(filepath.Join(dir, ".tool-versions"), []byte(tt.toolVersions), filePerm))
			}
			t.Chdir(dir)

			tools := DefaultTools()
			byRepo := make(map[string]Tool, len(tools))
			for _, tool := range tools {
				byRepo[tool.Repo] = tool
			}

			if tt.wantAllFallback {
				for repo, wantVersion := range defaultToolVersions {
					assert.Equal(t, wantVersion, byRepo[repo].Version, "repo %s should fall back to defaultToolVersions", repo)
				}
				return
			}

			assert.Equal(t, tt.wantOpentofu, byRepo["opentofu/opentofu"].Version)
			assert.Equal(t, tt.wantTerraform, byRepo["hashicorp/terraform"].Version)
			// Everything not explicitly pinned in this case's .tool-versions still falls back.
			assert.Equal(t, defaultToolVersions["hashicorp/packer"], byRepo["hashicorp/packer"].Version)
			assert.Equal(t, defaultToolVersions["helmfile/helmfile"], byRepo["helmfile/helmfile"].Version)
			assert.Equal(t, defaultToolVersions["helm/helm"], byRepo["helm/helm"].Version)
		})
	}
}
