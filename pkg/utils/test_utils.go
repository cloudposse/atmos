package utils

import (
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestVerbosity controls the verbosity level of test output
type TestVerbosity int

const (
	// TestVerbosityQuiet minimal output, only failures
	TestVerbosityQuiet TestVerbosity = iota
	// TestVerbosityNormal standard test output
	TestVerbosityNormal
	// TestVerbosityVerbose detailed test output
	TestVerbosityVerbose
)

var (
	// DefaultTestVerbosity is the default verbosity level for tests
	DefaultTestVerbosity = TestVerbosityNormal
	// Store descriptions to avoid repetition
	printedDescriptions = make(map[string]bool)
)

// GetTestVerbosity returns the current test verbosity level
func GetTestVerbosity() TestVerbosity {
	verbStr := os.Getenv("ATMOS_TEST_VERBOSITY")
	switch verbStr {
	case "quiet":
		return TestVerbosityQuiet
	case "verbose":
		return TestVerbosityVerbose
	default:
		return DefaultTestVerbosity
	}
}

// TestLogf logs a message during test execution based on verbosity level
func TestLogf(t *testing.T, minVerbosity TestVerbosity, format string, args ...interface{}) {
	if GetTestVerbosity() >= minVerbosity {
		t.Logf(format, args...)
	}
}

// TestLog logs a message during test execution based on verbosity level
func TestLog(t *testing.T, minVerbosity TestVerbosity, args ...interface{}) {
	if GetTestVerbosity() >= minVerbosity {
		t.Log(args...)
	}
}

// LogTestDescription logs a test description only once to avoid repetition
func LogTestDescription(t *testing.T, description string) {
	if description == "" {
		return
	}

	// Create a unique key for the description to avoid repetition within the same test
	key := fmt.Sprintf("%s-%s", t.Name(), description)
	if !printedDescriptions[key] {
		if GetTestVerbosity() == TestVerbosityQuiet {
			t.Logf("Test Description: %s", description)
		} else {
			t.Logf("\nTest Description: %s", description)
		}
		printedDescriptions[key] = true
	}
}

// LogTestFailure logs detailed failure information based on verbosity
func LogTestFailure(t *testing.T, description string, expected, actual interface{}, extraInfo ...string) {
	LogTestDescription(t, description)

	verbosity := GetTestVerbosity()
	if verbosity == TestVerbosityQuiet {
		t.Errorf("Expected: %v\nGot: %v", expected, actual)
	} else {
		t.Errorf("\nExpected: %v\nGot: %v", expected, actual)
		if len(extraInfo) > 0 && verbosity >= TestVerbosityNormal {
			t.Logf("Additional Info:\n%s", strings.Join(extraInfo, "\n"))
		}
	}
}

// AssertTestResult is a wrapper for testify/assert that respects verbosity
func AssertTestResult(t *testing.T, assertion func() bool, description string, msgAndArgs ...interface{}) bool {
	result := assertion()
	if !result {
		LogTestDescription(t, description)
		return assert.True(t, result, msgAndArgs...)
	}
	if GetTestVerbosity() >= TestVerbosityVerbose {
		LogTestDescription(t, description)
		return assert.True(t, result, msgAndArgs...)
	}
	return result
}
