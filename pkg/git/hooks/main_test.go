package hooks

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"testing"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

const (
	// Makes the test binary behave like `atmos git hooks run <hook> [args]`, so a real Git hook
	// shim can run it without an installed atmos.
	fakeAtmosEnv = "_ATMOS_TEST_FAKE_ATMOS_HOOK"
	// Names a JSON file with the schema.GitConfig the fake atmos runs.
	fakeAtmosConfigEnv = "_ATMOS_TEST_FAKE_ATMOS_CONFIG"
)

func TestMain(m *testing.M) {
	if os.Getenv(fakeAtmosEnv) == "1" {
		os.Exit(runFakeAtmos(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// runFakeAtmos implements `git hooks run <hook> [args...]` against the configuration the test wrote.
func runFakeAtmos(args []string) int {
	if len(args) < 3 || args[0] != "git" || args[1] != "hooks" || args[2] != "run" || len(args) < 4 {
		fmt.Fprintln(os.Stderr, "fake atmos: expected `git hooks run <hook> [args]`, got", args)
		return 2
	}
	data, err := os.ReadFile(os.Getenv(fakeAtmosConfigEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "fake atmos:", err)
		return 2
	}
	var cfg schema.GitConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		fmt.Fprintln(os.Stderr, "fake atmos:", err)
		return 2
	}
	if err := Run(&cfg, args[3], args[4:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		var exitErr errUtils.ExitCodeError
		if errors.As(err, &exitErr) {
			return exitErr.Code
		}
		return 1
	}
	return 0
}
