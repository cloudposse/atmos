package testshard

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Exercise real Go selection, including dynamic subtests, runnable examples,
// and fuzz seed cases; matching names in a unit test alone cannot prove this.
func TestGoTestSelection(t *testing.T) {
	root := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(root, "go.mod"), []byte("module fixture\n\ngo 1.26\n"), 0o600))
	source := `package fixture
import("fmt"; "testing")
func TestA(t *testing.T) { t.Run("one",func(t *testing.T){}); t.Run("two",func(t *testing.T){}) }
func TestB(t *testing.T) {}
func Example() { fmt.Println("ok"); /* Output: ok */ }
func FuzzSeed(f *testing.F) { f.Add("seed"); f.Fuzz(func(t *testing.T,s string){}) }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "fixture_test.go"), []byte(source), 0o600))
	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("go", append([]string{"-C", root, "test"}, args...)...)
		output, err := cmd.CombinedOutput()
		require.NoError(t, err, "%s", output)
		return string(output)
	}
	names, err := Discover(run("-list", "."))
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"TestA", "TestB", "Example", "FuzzSeed"}, names)
	groups, err := Plan(names, 3, nil)
	require.NoError(t, err)
	var executed []string
	for _, group := range groups {
		timings, err := ReadTimings(strings.NewReader(run("-json", "-count=1", "-run", Pattern(group))))
		require.NoError(t, err)
		for name := range timings.Tests["fixture"] {
			executed = append(executed, name)
		}
	}
	assert.ElementsMatch(t, names, executed)
}
