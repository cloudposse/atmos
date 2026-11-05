package acceptance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPrecompiledTestsPreserveBuildEnvironment(t *testing.T) {
	// Hosted Windows runners enable CGO by default. Precompiled tests must
	// propagate their build settings to any Go commands they launch.
	t.Setenv("CGO_ENABLED", "1")
	t.Setenv("GOFIPS140", "off")
	t.Setenv("ATMOS_CI_ENV_SENTINEL", "inherited")
	src := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/environmentfixture\n\ngo 1.26.0\n",
		"environment_test.go": `package environmentfixture
import (
    "os"
    "os/exec"
    "strings"
    "testing"
)
func TestEnvironment(t *testing.T) {
    if os.Getenv("ATMOS_CI_ENV_SENTINEL") != "inherited" {
        t.Fatal("parent environment was lost")
    }
    output, err := exec.Command("go", "env", "CGO_ENABLED", "GOFIPS140").CombinedOutput()
    if err != nil {
        t.Fatalf("nested go env: %v: %s", err, output)
    }
    if got := strings.Fields(string(output)); len(got) != 2 || got[0] != "0" || got[1] != "latest" {
        t.Fatalf("nested build settings = %q, want CGO_ENABLED=0 GOFIPS140=latest", output)
    }
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(src, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	binary := filepath.Join(t.TempDir(), "environment.test.exe")
	if _, err := newCommandRunner().output(t.Context(), src, goCommandEnvironment(), "go", "test", "-c", "-covermode=atomic", "-o", binary, "."); err != nil {
		t.Fatal(err)
	}
	repo := t.TempDir()
	for _, dir := range []string{"tests", filepath.Join("internal", "exec"), "cmd"} {
		if err := os.MkdirAll(filepath.Join(repo, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	options := &RunOptions{
		RepoRoot: repo, Target: TargetWindows, Mode: ModeTest,
		TestsBinary: binary, ExecTestBinary: binary, CmdTestBinary: binary,
		Shard: Shard{Index: 1, Count: 1},
	}
	t.Run("tests", func(t *testing.T) {
		if err := runWindowsTests(t.Context(), newCommandRunner(), options); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("internal-exec", func(t *testing.T) {
		if err := runWindowsExecTests(t.Context(), newCommandRunner(), options); err != nil {
			t.Fatal(err)
		}
	})
	t.Run("cmd", func(t *testing.T) {
		if _, err := runCmdTests(t.Context(), newCommandRunner(), options); err != nil {
			t.Fatal(err)
		}
	})
}
