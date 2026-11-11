package cmd

import (
	"testing"

	"github.com/cloudposse/atmos/tests"
)

// requirePacker skips the test unless packer is available, and keeps it available to
// the in-process Atmos command the test runs.
func requirePacker(t *testing.T) {
	t.Helper()
	tests.RequirePacker(t)
	keepInheritedToolchainPath(t)
}

// requireHelmfile skips the test unless helmfile is available, and keeps it available
// to the in-process Atmos command the test runs.
func requireHelmfile(t *testing.T) {
	t.Helper()
	tests.RequireHelmfile(t)
	keepInheritedToolchainPath(t)
}

// keepInheritedToolchainPath treats toolchain directories inherited from an outer Atmos
// run (for example, `atmos test`) as user-supplied PATH entries for this test.
//
// The outer run records the directories it added to PATH in installedProjectPathEnv.
// In-process command startup strips those directories when the fixture project selects
// different tools, as fixtures without a .tool-versions do. Without this, a tool the
// precondition just found on PATH would be gone by the time the command runs it.
func keepInheritedToolchainPath(t *testing.T) {
	t.Helper()
	t.Setenv(installedProjectPathEnv, "")
}
