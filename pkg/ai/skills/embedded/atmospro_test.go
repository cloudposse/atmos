package embedded_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/ai/skills/embedded"
)

// TestAtmosProSkill_SelfContained verifies that every file the atmos-pro
// SKILL.md references lives inside the skill's own directory.
//
// Why this matters: Claude Code's Path B invocation loads only the
// agent-skills/skills/atmos-pro/ subtree from the remote repo. Any reference
// to a sibling skill (for example "../atmos-terraform/...") or to the Atmos
// binary's internal paths would silently fail at runtime.
func TestAtmosProSkill_SelfContained(t *testing.T) {
	skill, err := embedded.Load("atmos-pro")
	require.NoError(t, err)

	// Every "references/<file>" path mentioned inside the system prompt must
	// resolve to a file under the skill directory.
	refPattern := regexp.MustCompile(`references/[a-z0-9\-]+\.md`)
	matches := refPattern.FindAllString(skill.SystemPrompt, -1)
	assert.NotEmpty(t, matches, "SKILL.md should reference at least one file under references/")

	skillDir := filepath.Join("..", "..", "..", "..", "agent-skills", "skills", "atmos-pro")
	seen := map[string]bool{}
	for _, m := range matches {
		if seen[m] {
			continue
		}
		seen[m] = true
		_, err := os.Stat(filepath.Join(skillDir, m))
		assert.NoErrorf(t, err, "referenced file %q must exist under the skill directory", m)
	}

	// Verify the skill directory contains nothing that escapes via symlink.
	err = filepath.WalkDir(skillDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(path)
			if err != nil {
				return err
			}
			// Symlinks within the skill dir are fine; symlinks pointing outside
			// break Path B remote-load, which fetches only the skill subtree.
			assert.False(t, strings.HasPrefix(target, "/") || strings.HasPrefix(target, ".."),
				"symlink %q points outside skill directory (target: %q)", path, target)
		}
		return nil
	})
	require.NoError(t, err)
}

// TestAtmosProSkill_TemplatesSelfContained ensures the templates/ subtree does
// not reference external files either. Path B agents walk this directory to
// materialize artifacts.
func TestAtmosProSkill_TemplatesSelfContained(t *testing.T) {
	templatesDir := filepath.Join("..", "..", "..", "..", "agent-skills", "skills", "atmos-pro", "templates")

	err := filepath.WalkDir(templatesDir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		info, err := os.Lstat(path)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			t.Errorf("templates subtree contains a symlink at %q — disallowed for remote-load safety", path)
		}
		return nil
	})
	require.NoError(t, err)
}

// TestAtmosProSkill_ReferencesDeclared ensures every reference markdown file on
// disk is also declared in the SKILL.md frontmatter. Otherwise the embedded
// loader won't concatenate it into the system prompt, and the skill ships with
// content the AI cannot see.
func TestAtmosProSkill_ReferencesDeclared(t *testing.T) {
	referencesDir := filepath.Join("..", "..", "..", "..", "agent-skills", "skills", "atmos-pro", "references")
	entries, err := os.ReadDir(referencesDir)
	require.NoError(t, err)

	skill, err := embedded.Load("atmos-pro")
	require.NoError(t, err)

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		want := "## Reference: " + e.Name()
		assert.Containsf(t, skill.SystemPrompt, want,
			"reference file %q must be declared in SKILL.md frontmatter references: so the loader picks it up",
			e.Name())
	}
}
