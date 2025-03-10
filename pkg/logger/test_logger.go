package logger

import (
	"errors"
	"time"
)

var ErrTest = errors.New("this is an error")

const (
	KeyComponent = "component"
	KeyStack     = "stack"
	KeyError     = "err"
	KeyDuration  = "duration"
	KeyPath      = "path"
	KeyDetails   = "details"
)

func main() {
	// Test individual log functions
	testIndividualLogFunctions()

	// Test complete Logger struct
	testLoggerStruct()

	// Test direct charmbracelet logger
	testCharmLogger()
}

func testIndividualLogFunctions() {
	Info("This is an info message")
	Debug("This is a debug message with context", KeyComponent, "station", KeyDuration, "500ms")
	Warn("This is a warning message", KeyStack, "prod-ue1")
	Error("Whoops! Something went wrong", KeyError, "kitchen on fire", KeyComponent, "weather")

	time.Sleep(500 * time.Millisecond)
}

func testLoggerStruct() {
	atmosLogger, err := InitializeLogger(LogLevelTrace, "")
	if err != nil {
		panic(err)
	}

	atmosLogger.Trace("This is a trace message")
	atmosLogger.Debug("This is a debug message")
	atmosLogger.Info("This is an info message")
	atmosLogger.Warning("This is a warning message")
	atmosLogger.Error(ErrTest)

	time.Sleep(500 * time.Millisecond)
}

func testCharmLogger() {
	charmLogger := GetCharmLogger()

	charmLogger.SetTimeFormat(time.Kitchen)

	charmLogger.Info("Processing component", KeyComponent, "station", KeyStack, "dev-ue1")
	charmLogger.Debug("Found configuration", KeyPath, "/stacks/deploy/us-east-1/dev/station.yaml")
	charmLogger.Warn("Component configuration outdated", KeyComponent, "weather", "lastUpdated", "90 days ago")
	charmLogger.Error("Failed to apply changes",
		KeyError, "validation failed",
		KeyComponent, "weather",
		KeyDetails, "required variables missing",
		KeyStack, "dev-ue1")
}
