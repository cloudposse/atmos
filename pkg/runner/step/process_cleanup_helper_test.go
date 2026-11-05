package step

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Helper-mode contract for the process-cleanup tests. The test binary re-executes itself (see
// TestMain) as a stand-in for a command that starts a worker: "spawn-grandchild" records its pid
// and starts a "pidfile-sleep" copy of itself, which records its own pid and sleeps. Both inherit
// the marker arguments the test passed, so a survivor can be recognized.
const (
	// Names the directory the helpers record their pids in.
	stepHelperPIDDirEnv = "_ATMOS_STEP_FAKE_PID_DIR"
	// Outlives every timeout the tests use, so a helper that is gone was killed.
	stepHelperSleep = 5 * time.Minute
)

// writeStepPIDFile records the current process id as <name>.pid in the helper directory.
func writeStepPIDFile(name string) {
	dir := os.Getenv(stepHelperPIDDirEnv)
	if dir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, name+".pid"), []byte(strconv.Itoa(os.Getpid())), 0o600)
}

// runStepSpawnGrandchild is the "spawn-grandchild" helper mode.
func runStepSpawnGrandchild() {
	writeStepPIDFile("parent")
	exe, err := os.Executable()
	if err != nil {
		os.Exit(2)
	}
	worker := exec.Command(exe, os.Args[1:]...)
	worker.Env = append(os.Environ(), "_ATMOS_STEP_FAKE=pidfile-sleep")
	if err := worker.Start(); err != nil {
		os.Exit(2)
	}
	time.Sleep(stepHelperSleep)
}

// stepHelperPID returns the pid a helper recorded, or 0 when it has not recorded one yet.
func stepHelperPID(dir, name string) int {
	data, err := os.ReadFile(filepath.Join(dir, name+".pid"))
	if err != nil {
		return 0
	}
	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0
	}
	return pid
}

// killStepHelpersAtCleanup makes sure no helper outlives the test run, whatever the assertions did.
func killStepHelpersAtCleanup(t *testing.T, dir string) {
	t.Helper()
	t.Cleanup(func() {
		for _, name := range []string{"parent", "child"} {
			if pid := stepHelperPID(dir, name); pid > 0 {
				if process, err := os.FindProcess(pid); err == nil {
					_ = process.Kill()
				}
			}
		}
	})
}
