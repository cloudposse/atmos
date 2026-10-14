package starlark

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	cockroach "github.com/cockroachdb/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/script"
)

// writeScriptTree creates scripts/main.star (loads lib/util.star), scripts/lib/util.star and an
// unrelated working directory. It returns the project root and the working directory.
func writeScriptTree(t *testing.T, main string) (root, workDir string) {
	t.Helper()
	root = t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(root, "scripts", "lib"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(root, "scripts", "lib", "util.star"), []byte("def greet(who):\n    return \"hello, \" + who\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "scripts", "main.star"), []byte(main), 0o600))
	workDir = filepath.Join(root, "elsewhere")
	require.NoError(t, os.Mkdir(workDir, 0o700))
	return root, workDir
}

func TestSourcePathAnchorsLoadToTheScriptFile(t *testing.T) {
	t.Parallel()
	root, workDir := writeScriptTree(t, "load(\"lib/util.star\", \"greet\")\noutput = greet(\"main\")\n")
	mainPath := filepath.Join(root, "scripts", "main.star")
	source, err := os.ReadFile(mainPath)
	require.NoError(t, err)

	t.Run("load resolves against the script file, not working_directory", func(t *testing.T) {
		t.Parallel()
		result, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: string(source), SourcePath: mainPath, WorkingDirectory: workDir,
		})
		require.NoError(t, runErr)
		assert.Equal(t, "hello, main", result.Value)
	})

	t.Run("dry run parses without loading", func(t *testing.T) {
		t.Parallel()
		_, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: string(source), SourcePath: mainPath, WorkingDirectory: workDir, DryRun: true,
		})
		require.NoError(t, runErr)
	})

	t.Run("inline script still resolves against working_directory", func(t *testing.T) {
		t.Parallel()
		_, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: string(source), WorkingDirectory: workDir,
		})
		require.Error(t, runErr)
		assert.Contains(t, runErr.Error(), filepath.Join(workDir, "lib", "util.star"))
	})
}

// Not parallel: it changes the process working directory.
func TestSourcePathRelativeIsMadeAbsolute(t *testing.T) {
	root, workDir := writeScriptTree(t, "load(\"lib/util.star\", \"greet\")\noutput = greet(\"main\")\n")
	source, err := os.ReadFile(filepath.Join(root, "scripts", "main.star"))
	require.NoError(t, err)
	t.Chdir(filepath.Join(root, "scripts"))

	result, runErr := New().Execute(context.Background(), script.Spec{
		Name: "step", Source: string(source), SourcePath: "main.star", WorkingDirectory: workDir,
	})
	require.NoError(t, runErr)
	assert.Equal(t, "hello, main", result.Value)
}

func TestSourcePathNamesTheFileInTracebacks(t *testing.T) {
	t.Parallel()
	root, workDir := writeScriptTree(t, "def explode(values):\n    return values[5]\n\nexplode([1])\n")
	mainPath := filepath.Join(root, "scripts", "main.star")
	source, err := os.ReadFile(mainPath)
	require.NoError(t, err)

	_, runErr := New().Execute(context.Background(), script.Spec{
		Name: "step", Source: string(source), SourcePath: mainPath, WorkingDirectory: workDir,
	})
	require.Error(t, runErr)
	assert.Contains(t, runErr.Error(), "out of range")
	assert.Contains(t, errorDetails(runErr), mainPath+":4:8: in <toplevel>")
	assert.Contains(t, errorDetails(runErr), mainPath+":2:18: in explode")
}

func TestSourcePathExposesTheScriptFileToTheScript(t *testing.T) {
	t.Parallel()
	root, workDir := writeScriptTree(t, "")
	mainPath := filepath.Join(root, "scripts", "main.star")

	result, err := New().Execute(context.Background(), script.Spec{
		Name:       "step",
		Source:     "output = [ctx.script.path, ctx.script.directory]",
		SourcePath: mainPath, WorkingDirectory: workDir,
	})
	require.NoError(t, err)
	assert.JSONEq(t, `[`+quoteJSON(mainPath)+`,`+quoteJSON(filepath.Dir(mainPath))+`]`, result.Value)

	inline, err := New().Execute(context.Background(), script.Spec{Name: "step", Source: "output = ctx.script", WorkingDirectory: workDir})
	require.NoError(t, err)
	assert.False(t, inline.HasOutput, "None means no output")
}

// errorDetails joins every explanation attached to err, which is where tracebacks live.
func errorDetails(err error) string {
	return strings.Join(cockroach.GetAllDetails(err), "\n")
}

func quoteJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func TestProjectRootShowsRelativePathsInTracebacks(t *testing.T) {
	t.Parallel()
	root, workDir := writeScriptTree(t, "load(\"lib/boom.star\", \"explode\")\nexplode([1])\n")
	require.NoError(t, os.WriteFile(filepath.Join(root, "scripts", "lib", "boom.star"), []byte("def explode(values):\n    return values[5]\n"), 0o600))
	mainPath := filepath.Join(root, "scripts", "main.star")
	source, err := os.ReadFile(mainPath)
	require.NoError(t, err)

	t.Run("entry script and loaded module are project-relative", func(t *testing.T) {
		t.Parallel()
		_, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: string(source), SourcePath: mainPath, ProjectRoot: root, WorkingDirectory: workDir,
		})
		require.Error(t, runErr)
		details := errorDetails(runErr)
		assert.Contains(t, details, filepath.Join("scripts", "main.star")+":2:8: in <toplevel>")
		assert.Contains(t, details, filepath.Join("scripts", "lib", "boom.star")+":2:18: in explode")
		assert.NotContains(t, details, root, "no absolute project path in the traceback")
		assert.NotContains(t, details, "..", "a relative path never climbs out of the root")
	})

	t.Run("without a project root paths stay absolute", func(t *testing.T) {
		t.Parallel()
		_, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: string(source), SourcePath: mainPath, WorkingDirectory: workDir,
		})
		require.Error(t, runErr)
		assert.Contains(t, errorDetails(runErr), mainPath+":2:8: in <toplevel>")
	})

	t.Run("a script outside the project root keeps its absolute path", func(t *testing.T) {
		t.Parallel()
		otherRoot := t.TempDir()
		_, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: string(source), SourcePath: mainPath, ProjectRoot: otherRoot, WorkingDirectory: workDir,
		})
		require.Error(t, runErr)
		assert.Contains(t, errorDetails(runErr), mainPath+":2:8: in <toplevel>")
	})

	t.Run("load still resolves against the script file", func(t *testing.T) {
		t.Parallel()
		_, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: "load(\"lib/missing.star\", \"x\")\n", SourcePath: mainPath, ProjectRoot: root, WorkingDirectory: workDir,
		})
		require.Error(t, runErr)
		assert.Contains(t, runErr.Error(), filepath.Join("scripts", "lib", "missing.star"))
		assert.NotContains(t, runErr.Error(), root)
	})
}

func TestProjectRootShowsRelativePathsInSyntaxErrors(t *testing.T) {
	t.Parallel()
	root, workDir := writeScriptTree(t, "x = (\n")
	mainPath := filepath.Join(root, "scripts", "main.star")

	for _, dryRun := range []bool{false, true} {
		_, runErr := New().Execute(context.Background(), script.Spec{
			Name: "step", Source: "x = (\n", SourcePath: mainPath, ProjectRoot: root, WorkingDirectory: workDir, DryRun: dryRun,
		})
		require.Error(t, runErr)
		assert.Contains(t, runErr.Error(), filepath.Join("scripts", "main.star")+":")
		assert.NotContains(t, runErr.Error(), root)
	}
}

// Not parallel: it changes the process working directory.
func TestProjectRootRelativeIsMadeAbsolute(t *testing.T) {
	root, workDir := writeScriptTree(t, "explode = [1][5]\n")
	mainPath := filepath.Join(root, "scripts", "main.star")
	source, err := os.ReadFile(mainPath)
	require.NoError(t, err)
	t.Chdir(root)

	_, runErr := New().Execute(context.Background(), script.Spec{
		Name: "step", Source: string(source), SourcePath: mainPath, ProjectRoot: ".", WorkingDirectory: workDir,
	})
	require.Error(t, runErr)
	assert.Contains(t, errorDetails(runErr), filepath.Join("scripts", "main.star")+":1:")
	assert.NotContains(t, errorDetails(runErr), root)
}

func TestProjectRootShowsRelativePathsInTaskTracebacks(t *testing.T) {
	t.Parallel()
	source := "def boom():\n    fail(\"broken\")\nsteps.parallel(tasks = [steps.task(name = \"a\", function = boom)])\n"
	root, workDir := writeScriptTree(t, source)
	mainPath := filepath.Join(root, "scripts", "main.star")

	_, runErr := New().Execute(context.Background(), script.Spec{
		Name: "step", Source: source, SourcePath: mainPath, ProjectRoot: root, WorkingDirectory: workDir,
	})
	require.Error(t, runErr)
	details := errorDetails(runErr)
	assert.Contains(t, details, `Traceback for task "a"`)
	assert.Contains(t, details, filepath.Join("scripts", "main.star")+":2:9: in boom")
	assert.NotContains(t, details, root)
	assert.NotContains(t, runErr.Error(), root)
}
