//go:build mage

package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"

	"github.com/cloudposse/atmos/internal/ci/acceptance"
)

// Test groups targets that run this repository's own test suite, as opposed
// to Acceptance, which drives the CLI acceptance suite as a subprocess.
type Test mg.Namespace

// racePackagePrefix excludes the CLI acceptance suite (github.com/.../tests
// and its subpackages) from the race sweep: the `test` job's own comment
// measures its unsharded runtime at ~90m on Linux, which is why that job
// shards it 10-way per OS. The acceptance suite also builds and shells out to
// a plain (non -race) `atmos` binary, so instrumenting the driving test
// process with -race provides no race coverage on the binary under test.
const racePackagePrefix = "github.com/cloudposse/atmos/tests"

// raceTestTimeout is longer than the default per-package 10m. This runs
// every package in the repository unsharded, and the toolchain package's
// own tests install real tool binaries from real registries with no mock
// seam, so every package's network/registry calls compete for the same
// runner instead of getting a shard's worth of headroom the way the sharded
// `test` job's packages get. The race detector's own CPU/memory overhead
// compounds that contention. See docs/fixes for the incident where 10m
// wasn't enough headroom in CI.
const raceTestTimeout = "20m"

// Race runs the full test suite (excluding ./tests/..., the CLI acceptance
// suite) with the race detector and shuffled test order. This is the Go
// implementation backing the `atmos test race` custom command
// (.atmos.d/test.yaml) and the `[race] full test suite` CI job
// (.github/workflows/test.yml).
func (Test) Race() error {
	root, err := mageRepoRoot()
	if err != nil {
		return err
	}

	packages, err := racePackagesFromEnv(root)
	if err != nil {
		return err
	}

	testArgs, err := acceptance.SplitCommandLine(os.Getenv("TESTARGS"))
	if err != nil {
		return err
	}

	args := make([]string, 0, len(packages)+len(testArgs)+5)
	args = append(args, "test", "-race", "-shuffle=on")
	args = append(args, packages...)
	args = append(args, testArgs...)
	args = append(args, "-timeout", raceTestTimeout)

	return runIn(root, nil, "go", args...)
}

// racePackagesFromEnv returns the packages to race-test: TEST's value
// (shell-quoted, matching every other TEST override in this repo) when set,
// or racePackages(root) otherwise.
func racePackagesFromEnv(root string) ([]string, error) {
	if raw := os.Getenv("TEST"); raw != "" {
		return acceptance.SplitCommandLine(raw)
	}
	return racePackages(root)
}

// racePackages lists every package under root except the CLI acceptance
// suite (racePackagePrefix and its subpackages).
func racePackages(root string) ([]string, error) {
	output, err := sh.Output("go", "-C", root, "list", "./...")
	if err != nil {
		return nil, fmt.Errorf("mage: go list ./...: %w", err)
	}
	return filterRacePackages(output), nil
}

// filterRacePackages splits a `go list ./...`-style newline-separated
// package list and drops racePackagePrefix and its subpackages.
func filterRacePackages(goListOutput string) []string {
	var packages []string
	for _, line := range strings.Split(goListOutput, "\n") {
		pkg := strings.TrimSpace(line)
		if pkg == "" {
			continue
		}
		if pkg == racePackagePrefix || strings.HasPrefix(pkg, racePackagePrefix+"/") {
			continue
		}
		packages = append(packages, pkg)
	}
	return packages
}
