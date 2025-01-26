package utils

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// Store descriptions to avoid repetition
var printedDescriptions = make(map[string]bool)

// LogTestDescription logs a test description only once to avoid repetition
func LogTestDescription(t *testing.T, description string) {
	if description == "" {
		return
	}

	// Create a unique key for the description to avoid repetition within the same test
	key := fmt.Sprintf("%s-%s", t.Name(), description)
	if !printedDescriptions[key] {
		t.Logf("Test Description: %s", description)
		printedDescriptions[key] = true
	}
}

// LogTestFailure logs detailed failure information
func LogTestFailure(t *testing.T, description string, expected, actual interface{}, extraInfo ...string) {
	LogTestDescription(t, description)
	t.Errorf("\nExpected: %v\nGot: %v", expected, actual)
	if len(extraInfo) > 0 {
		t.Logf("Additional Info:\n%s", strings.Join(extraInfo, "\n"))
	}
}

// AssertTestResult is a wrapper for testify/assert
func AssertTestResult(t *testing.T, assertion func() bool, description string, msgAndArgs ...interface{}) bool {
	result := assertion()
	if !result {
		LogTestDescription(t, description)
		return assert.True(t, result, msgAndArgs...)
	}
	return result
}
