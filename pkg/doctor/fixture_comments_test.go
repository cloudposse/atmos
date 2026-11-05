package doctor

import (
	"io/fs"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// copyScenario copies a doctor fixture scenario outside the repository, so the repository's own
// configuration cannot layer over it and the fixture itself is never edited.
func copyScenario(t *testing.T, name string) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "tests", "fixtures", "scenarios", "doctor", name))
	require.NoError(t, err)
	root := newProject(t)
	copied := 0
	err = filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		copied++
		projectFile(t, root, rel, string(content))
		return nil
	})
	require.NoError(t, err)
	require.Positive(t, copied, "the %s scenario must contain files", name)
	return root
}

func TestCommentsScenarioRepairsKeepEveryComment(t *testing.T) {
	root := copyScenario(t, "comments")
	originals := map[string]string{}
	for _, name := range []string{"atmos.yaml", filepath.Join("stacks", "deploy", "dev.yaml")} {
		content, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		originals[name] = string(content)
	}

	r := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})

	require.ElementsMatch(t, []string{"atmos.yaml", "stacks/deploy/dev.yaml"}, r.ChangedFiles)
	for _, rule := range []string{ruleTerraformReconfigure, ruleCommandProvides, ruleHookEvent} {
		found := ecFindings(&r, rule)
		require.NotEmpty(t, found, rule)
		for i := range found {
			require.Equal(t, statusFixed, found[i].Status, "%s: %s", rule, found[i].Reason)
		}
	}
	require.Equal(t, 0, r.ExitCode("error"))
	for name, original := range originals {
		content, err := os.ReadFile(filepath.Join(root, name))
		require.NoError(t, err)
		require.ElementsMatch(t, commentTexts(original), commentTexts(string(content)), name)
	}

	atmos, err := os.ReadFile(filepath.Join(root, "atmos.yaml"))
	require.NoError(t, err)
	for _, kept := range []string{`name_template: "{{ .vars.stage }}"`, `description: "Inspect {{ .Arguments.target }} in a stack"`, `echo "inspect {{ .Arguments.target }} in {{ .Flags.stack }}"`, "reconfigure: never # keep reconfiguring off", "provides: component # legacy semantic argument"} {
		require.Contains(t, string(atmos), kept)
	}

	second := ecRun(t, &Options{Root: root, Offline: true, Fix: true, Only: []string{categoryDeprecations}})
	require.Empty(t, second.ChangedFiles, "a second run must find nothing to repair")
}
