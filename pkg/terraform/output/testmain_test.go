package output

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
)

const (
	// Variable envInitHelper gates the test binary's "fake terraform init" subprocess mode.
	envInitHelper = "_ATMOS_TEST_INIT_HELPER"
	// Variable envInitHelperOut is the file the fake init writes its observed state to.
	envInitHelperOut = "_ATMOS_TEST_INIT_OUT"
	// Variable envInitHelperExit is the exit code the fake init returns.
	envInitHelperExit = "_ATMOS_TEST_INIT_EXIT"
	// Variable envInitHelperStderr is the text the fake init prints to stderr.
	envInitHelperStderr = "_ATMOS_TEST_INIT_STDERR"
)

// initHelperObservation is what the fake init subprocess records about how it was launched.
type initHelperObservation struct {
	Args []string          `json:"args"`
	Cwd  string            `json:"cwd"`
	Env  map[string]string `json:"env"`
}

// TestMain lets the test binary double as a cross-platform fake terraform executable: when
// _ATMOS_TEST_INIT_HELPER=1 it records its args, cwd and environment and exits with the requested code.
func TestMain(m *testing.M) {
	if os.Getenv(envInitHelper) == "1" {
		os.Exit(runInitHelper())
	}
	os.Exit(m.Run())
}

func runInitHelper() int {
	cwd, _ := os.Getwd()
	obs := initHelperObservation{Args: os.Args[1:], Cwd: cwd, Env: map[string]string{}}
	for _, kv := range os.Environ() {
		if k, v, ok := strings.Cut(kv, "="); ok {
			obs.Env[k] = v
		}
	}
	data, err := json.Marshal(obs)
	if err != nil {
		return 3
	}
	if out := os.Getenv(envInitHelperOut); out != "" {
		if err := os.WriteFile(out, data, 0o600); err != nil {
			return 4
		}
	}
	if text := os.Getenv(envInitHelperStderr); text != "" {
		fmt.Fprint(os.Stderr, text)
	}
	if os.Getenv(envInitHelperExit) == "1" {
		return 1
	}
	return 0
}
