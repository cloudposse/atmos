package tests

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const jitEngineLogEnv = "ATMOS_TEST_JIT_ENGINE_LOG"

// runJITEngineHelper is dispatched before testing flags are parsed. Reusing the
// compiled test executable keeps the provisioning tests portable without a shell
// script or an installed Terraform binary. It models only successful execution;
// these tests assert Atmos's real source downloads, precedence and engine cwd.
func runJITEngineHelper() bool {
	logPath := os.Getenv(jitEngineLogEnv)
	if logPath == "" {
		return false
	}
	cwd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		panic(err)
	}
	if err := json.NewEncoder(f).Encode(jitEngineCall{Cwd: cwd, Args: os.Args[1:]}); err != nil {
		panic(err)
	}
	if err := f.Close(); err != nil {
		panic(err)
	}
	args := strings.Join(os.Args[1:], " ")
	switch {
	case strings.Contains(args, "version"):
		if strings.Contains(args, "-json") {
			fmt.Println(`{"terraform_version":"1.15.6","platform":"test","provider_selections":{}}`)
		} else {
			fmt.Println("Terraform v1.15.6")
		}
	case strings.HasPrefix(args, "workspace list"):
		fmt.Println("* dev")
	case strings.HasPrefix(args, "workspace show"):
		fmt.Println("dev")
	case strings.HasPrefix(args, "output"):
		fmt.Println(`{}`)
	case strings.HasPrefix(args, "show") && strings.Contains(args, "-json"):
		fmt.Println(`{"format_version":"1.2","terraform_version":"1.15.6"}`)
	}
	return true
}

type jitEngineCall struct {
	Cwd  string
	Args []string
}

func setupJITEngine(t *testing.T) (string, string) {
	t.Helper()
	executable, err := os.Executable()
	require.NoError(t, err)
	logPath := filepath.Join(t.TempDir(), "engine.jsonl")
	t.Setenv(jitEngineLogEnv, logPath)
	return executable, logPath
}

func requireJITEngineCommand(t *testing.T, logPath, workdir, command string) {
	t.Helper()
	data, err := os.ReadFile(logPath)
	require.NoError(t, err, "real execution must invoke the test engine")
	absWorkdir, err := filepath.Abs(workdir)
	require.NoError(t, err)
	absWorkdir, err = filepath.EvalSymlinks(absWorkdir)
	require.NoError(t, err)
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		var call jitEngineCall
		require.NoError(t, json.Unmarshal([]byte(line), &call))
		if len(call.Args) > 0 && call.Args[0] == command {
			cwd, err := filepath.EvalSymlinks(call.Cwd)
			require.NoError(t, err)
			require.Equal(t, absWorkdir, cwd, "engine must execute vendored source, not local component")
			return
		}
	}
	t.Fatalf("expected engine command %q in %s", command, data)
}
