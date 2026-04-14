package atmospro_test

import (
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentskills "github.com/cloudposse/atmos/agent-skills"
	"github.com/cloudposse/atmos/pkg/ai/skills/atmospro"
)

// TestPathParity_AgentSkillsFSMatchesSourceTree proves that the templates Path A
// renders from (via the agent-skills embed.FS compiled into the binary) are
// byte-identical to the templates on disk that Path B would fetch remotely.
//
// If this test fails, the Claude Code agent loading from the remote repo would
// generate artifacts that drift from what the Atmos binary generates — a
// silent contract violation. The PRD declares both paths must produce
// byte-identical output.
func TestPathParity_AgentSkillsFSMatchesSourceTree(t *testing.T) {
	// The embedded FS is rooted at "skills". Narrow to the atmos-pro templates.
	embeddedTemplates, err := fs.Sub(agentskills.SkillsFS, "skills/atmos-pro/templates")
	require.NoError(t, err)

	sourceTemplates := filepath.Join("..", "..", "..", "..", "agent-skills", "skills", "atmos-pro", "templates")

	// For every *.tmpl in the embedded FS, the on-disk copy must match byte-for-byte.
	err = fs.WalkDir(embeddedTemplates, ".", func(relPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() || filepath.Ext(relPath) != ".tmpl" {
			return nil
		}

		embeddedBytes, err := fs.ReadFile(embeddedTemplates, relPath)
		require.NoError(t, err, "read embedded %s", relPath)

		diskBytes, err := os.ReadFile(filepath.Join(sourceTemplates, relPath))
		require.NoError(t, err, "read disk %s", relPath)

		assert.Equalf(t, string(diskBytes), string(embeddedBytes),
			"%s diverged between embed.FS and disk — rebuild after template changes", relPath)
		return nil
	})
	require.NoError(t, err)
}

// TestPathParity_RenderedOutputMatchesGolden proves that rendering every
// embedded template against the fixture produces the same bytes as the
// golden snapshot. This closes the loop: if an agent (Path A or Path B)
// rendered the embedded templates with the fixture's RenderData, the output
// would match the committed golden.
func TestPathParity_RenderedOutputMatchesGolden(t *testing.T) {
	fixtureData, err := os.ReadFile(filepath.Join("..", "..", "..", "..",
		"tests", "fixtures", "scenarios", "atmos-pro-setup", "fixture.json"))
	require.NoError(t, err)
	var rd atmospro.RenderData
	require.NoError(t, json.Unmarshal(fixtureData, &rd))

	embeddedTemplates, err := fs.Sub(agentskills.SkillsFS, "skills/atmos-pro/templates")
	require.NoError(t, err)

	rendered, err := atmospro.RenderAll(embeddedTemplates, &rd)
	require.NoError(t, err)
	require.NotEmpty(t, rendered)

	goldenDir := filepath.Join("..", "..", "..", "..",
		"tests", "fixtures", "scenarios", "atmos-pro-setup", "golden")

	for _, outPath := range atmospro.SortedKeys(rendered) {
		goldenFile := filepath.Join(goldenDir, snapshotFilenameForParity(outPath))
		expected, err := os.ReadFile(goldenFile)
		require.NoError(t, err, "missing golden for %s", outPath)
		assert.Equalf(t, string(expected), rendered[outPath],
			"rendered output for %s drifted from golden (embed.FS rendering path)", outPath)
	}
}

// snapshotFilenameForParity mirrors the helper in render_test.go. Duplicated
// here rather than exported so the snapshot file naming stays a test-local
// implementation detail.
func snapshotFilenameForParity(outputPath string) string {
	s := outputPath
	// Flatten slashes to double-underscore, matching render_test.go.
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '/' {
			out = append(out, '_', '_')
			continue
		}
		out = append(out, s[i])
	}
	return string(out) + ".golden"
}
