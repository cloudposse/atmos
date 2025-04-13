package utils

import (
	"fmt"
	"os"
	"strconv"
)

// MaxShellDepth is the maximum number of nested shell commands that can be executed
const MaxShellDepth = 10

// getNextShellLevel increments the ATMOS_SHLVL and returns the new value or an error if maximum depth is exceeded
func GetNextShellLevel() (int, error) {
	atmosShellLvl := os.Getenv("ATMOS_SHLVL")
	shellVal := 0
	if atmosShellLvl != "" {
		val, err := strconv.Atoi(atmosShellLvl)
		if err != nil {
			return 0, fmt.Errorf("invalid ATMOS_SHLVL value: %s", atmosShellLvl)
		}
		shellVal = val
	}

	shellVal++

	if shellVal > MaxShellDepth {
		return 0, fmt.Errorf("ATMOS_SHLVL (%d) exceeds maximum allowed depth (%d). Infinite recursion?",
			shellVal, MaxShellDepth)
	}
	return shellVal, nil
}
