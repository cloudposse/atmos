package testdata

// LogLevelTrace constant for testing.
const LogLevelTrace = "Trace"

// LogLevelDebug constant for testing.
const LogLevelDebug = "Debug"

// Config represents a test config structure.
type Config struct {
	Logs LogsConfig
}

// LogsConfig represents logs configuration.
type LogsConfig struct {
	Level string
}

// BadLogLevelComparison uses log level to control UI behavior.
func BadLogLevelComparison(atmosConfig *Config) { // want "missing defer perf.Track\\(\\) call at start of public function BadLogLevelComparison"
	if atmosConfig.Logs.Level == LogLevelTrace || atmosConfig.Logs.Level == LogLevelDebug { // want "comparing log levels outside of logger package is not allowed" "comparing log levels outside of logger package is not allowed"
		// Show spinner - bad pattern!
	}
}

// BadLogLevelAccess directly accesses log level.
func BadLogLevelAccess(atmosConfig *Config) string { // want "missing defer perf.Track\\(\\) call at start of public function BadLogLevelAccess"
	return atmosConfig.Logs.Level // want "accessing atmosConfig.Logs.Level outside of logger package is not allowed"
}

// BadLogLevelCheck checks log level for UI decision.
func BadLogLevelCheck(atmosConfig *Config) { // want "missing defer perf.Track\\(\\) call at start of public function BadLogLevelCheck"
	if atmosConfig.Logs.Level == LogLevelDebug { // want "comparing log levels outside of logger package is not allowed"
		// Conditional UI behavior - bad!
	}
}

// GoodExplicitFlag uses explicit flag for UI control.
func GoodExplicitFlag(showSpinner bool) { // want "missing defer perf.Track\\(\\) call at start of public function GoodExplicitFlag"
	// OK: using explicit flag, not log level.
	if showSpinner {
		// Show spinner - good pattern!
	}
}
