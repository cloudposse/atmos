package utils

import (
	"os"
)

const (
	LogLevelTrace = "Trace"
	LogLevelDebug = "Debug"
)

// OsExit is a variable for testing, so we can mock os.Exit.
var OsExit = os.Exit
