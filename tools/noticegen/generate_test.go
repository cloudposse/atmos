package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildStubGoLicenses compiles a throwaway "go-licenses" binary that prints
// csv to stdout and exits 0, regardless of the arguments it's invoked with.
// It's a real native executable (built via `go build`, not a shell script)
// so this works unmodified on Windows, matching this repo's cross-platform
// test conventions.
func buildStubGoLicenses(t *testing.T, csv string) string {
	t.Helper()

	srcDir := t.TempDir()
	mainSrc := "package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Print(" + strconv.Quote(csv) + ")\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "main.go"), []byte(mainSrc), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(srcDir, "go.mod"), []byte("module noticegenstub\n\ngo 1.26\n"), 0o644))

	name := "go-licenses"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, name)

	cmd := exec.Command("go", "build", "-o", binPath, ".")
	cmd.Dir = srcDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))

	return binDir
}

func TestGenerateEndToEnd(t *testing.T) {
	stubDir := buildStubGoLicenses(t, "example.com/dep,https://example.com/dep/LICENSE,MIT\n")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	writeTestModule(t, root)
	outPath := filepath.Join(root, "NOTICE")

	summary, err := Generate(root, outPath)

	require.NoError(t, err)
	assert.Equal(t, Summary{Total: 1, MIT: 1}, summary)

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "MIT LICENSED DEPENDENCIES")
	assert.Contains(t, string(data), "example.com/dep")
	assert.NotContains(t, string(data), "MOZILLA PUBLIC LICENSE", "no MPL entries were reported")
}

func TestGenerateNoDependencies(t *testing.T) {
	stubDir := buildStubGoLicenses(t, "")
	t.Setenv("PATH", stubDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	root := t.TempDir()
	writeTestModule(t, root)
	outPath := filepath.Join(root, "NOTICE")

	summary, err := Generate(root, outPath)

	require.NoError(t, err)
	assert.Equal(t, Summary{}, summary)

	data, err := os.ReadFile(outPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "APACHE 2.0 LICENSED DEPENDENCIES")
}

func TestGeneratePropagatesGoLicensesInstallFailure(t *testing.T) {
	// An empty PATH means go-licenses isn't found and `go install` (which
	// itself needs `go` on PATH) fails fast, without a real network call.
	t.Setenv("PATH", "")

	root := t.TempDir()
	writeTestModule(t, root)

	_, err := Generate(root, filepath.Join(root, "NOTICE"))
	require.Error(t, err)
}
