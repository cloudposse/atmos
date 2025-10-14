package main

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/alicebob/miniredis/v2"
)

func TestMainHooksAndStoreIntegration(t *testing.T) {
	// Run the miniredis server so we can store values across calls to main()
	s := miniredis.RunT(t)
	defer s.Close()

	redisUrl := fmt.Sprintf("redis://%s", s.Addr())
	t.Setenv("ATMOS_REDIS_URL", redisUrl)

	origDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}
	defer os.RemoveAll(filepath.Join(origDir, "tests", "fixtures", "scenarios", "hooks-test", ".terraform"))
	defer os.Chdir(origDir)

	if err := os.Chdir("tests/fixtures/scenarios/hooks-test"); err != nil {
		t.Fatalf("failed to change directory: %v", err)
	}

	// Capture the original arguments
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	// Set the arguments for the first call to main() to deploy the `component1` component, which uses a `hook` to set a
	// value in Redis
	os.Args = []string{"atmos", "terraform", "deploy", "component1", "-s", "test"}
	main()

	// Set the arguments for the second call to main() to deploy the `component2` component, which uses a `store` to read a
	// value  that was set in the first apply.
	os.Args = []string{"atmos", "terraform", "deploy", "component2", "-s", "test"}
	main()
}
