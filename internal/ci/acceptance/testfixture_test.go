package acceptance

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildFixtureTestBinary compiles a real Go test binary (via `go test -c`) from
// a throwaway single-file module, one no-op test func per name in testNames.
// This package deliberately exercises real `go` tooling in tests instead of
// mocking commandRunner (see TestGoCommandEnvironmentDisablesCGO), so plan/run
// functions that shell out to a compiled test binary's -test.list/-test.run
// flags need a real binary to drive.
func buildFixtureTestBinary(t *testing.T, testNames []string) string {
	t.Helper()

	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "go.mod"), []byte("module example.com/fixture\n\ngo 1.21\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	needsFmt := false
	needsTesting := false
	var funcs strings.Builder
	for _, name := range testNames {
		switch {
		case strings.HasPrefix(name, "Benchmark"):
			needsTesting = true
			fmt.Fprintf(&funcs, "func %s(b *testing.B) {}\n\n", name)
		case strings.HasPrefix(name, "Example"):
			// -test.list only enumerates Example funcs that have a checkable
			// "// Output:" comment; a bare no-output Example compiles but is
			// never treated as runnable, so it would silently vanish from
			// listRunnableTests's results.
			needsFmt = true
			fmt.Fprintf(&funcs, "func %s() {\n\tfmt.Println(\"ok\")\n\t// Output:\n\t// ok\n}\n\n", name)
		default:
			needsTesting = true
			fmt.Fprintf(&funcs, "func %s(t *testing.T) {}\n\n", name)
		}
	}

	var body strings.Builder
	body.WriteString("package fixture\n\n")
	if needsFmt || needsTesting {
		body.WriteString("import (\n")
		if needsFmt {
			body.WriteString("\t\"fmt\"\n")
		}
		if needsTesting {
			body.WriteString("\t\"testing\"\n")
		}
		body.WriteString(")\n\n")
	}
	body.WriteString(funcs.String())
	if err := os.WriteFile(filepath.Join(src, "fixture_test.go"), []byte(body.String()), 0o600); err != nil {
		t.Fatal(err)
	}

	binary := filepath.Join(t.TempDir(), "fixture.test")
	cmd := exec.Command("go", "test", "-c", "-covermode=atomic", "-o", binary, ".")
	cmd.Dir = src
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build fixture test binary: %v\n%s", err, output)
	}
	return binary
}
