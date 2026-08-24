package shell

import (
	"fmt"
	"os"
	"strconv"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
)

// LevelEnvVar is the environment variable tracking how deeply nested the
// current Atmos-managed shell is.
const LevelEnvVar = "ATMOS_SHLVL"

// Level retrieves the current ATMOS_SHLVL value (0 when unset or invalid).
func Level() int {
	defer perf.Track(nil, "shell.Level")()

	atmosShellLvl := os.Getenv(LevelEnvVar) //nolint:forbidigo // ATMOS_SHLVL is a runtime variable that changes during shell execution, not a config variable.
	if atmosShellLvl == "" {
		return 0
	}
	val, err := strconv.Atoi(atmosShellLvl)
	if err != nil {
		return 0
	}
	return val
}

// SetLevel sets the ATMOS_SHLVL environment variable.
func SetLevel(level int) error {
	defer perf.Track(nil, "shell.SetLevel")()

	return os.Setenv(LevelEnvVar, fmt.Sprintf("%d", level))
}

// DecrementLevel decrements the ATMOS_SHLVL environment variable, stopping at 0.
func DecrementLevel() {
	defer perf.Track(nil, "shell.DecrementLevel")()

	currentLevel := Level()
	if currentLevel <= 0 {
		return
	}
	newLevel := currentLevel - 1
	if err := SetLevel(newLevel); err != nil {
		log.Warn("Failed to update ATMOS_SHLVL", "error", err)
	}
}
