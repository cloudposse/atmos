package container

import (
	execpkg "github.com/cloudposse/atmos/pkg/exec"
)

// globalExecutor holds the package-level executor instance.
// Tests can override this to inject mock executors.
var globalExecutor execpkg.CommandExecutor = execpkg.Default()

// setExecutor sets the global executor (for testing).
func setExecutor(executor execpkg.CommandExecutor) {
	globalExecutor = executor
}

// resetExecutor resets the global executor to default (for testing).
func resetExecutor() {
	globalExecutor = execpkg.Default()
}
