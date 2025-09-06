package utils

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLikelyTestName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Test functions
		{"basic test", "TestFoo", true},
		{"test with underscore", "Test_bar", true},
		{"test with numbers", "TestCase123", true},
		{"test with subtest", "TestFoo/subtest", true},
		{"test with nested subtest", "TestFoo/bar/baz", true},

		// Example functions
		{"example function", "ExampleFoo", true},
		{"example with underscore", "Example_bar", true},

		// Benchmark functions
		{"benchmark function", "BenchmarkFoo", true},
		{"benchmark with underscore", "Benchmark_bar", true},

		// Multiple tests
		{"multiple tests", "TestA|TestB", true},
		{"multiple mixed", "TestFoo|ExampleBar|BenchmarkBaz", true},

		// Not test names
		{"package path", "./...", false},
		{"relative path", "./pkg/utils", false},
		{"absolute path", "/usr/local/go", false},
		{"module path", "github.com/cloudposse/atmos", false},
		{"random word", "something", false},
		{"starts with test lowercase", "test", false},
		{"contains Test", "myTestFunction", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsLikelyTestName(tt.input)
			assert.Equal(t, tt.expected, result, "IsLikelyTestName(%q)", tt.input)
		})
	}
}

func TestIsPackagePath(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		// Definitely package paths
		{"current dir", ".", true},
		{"parent dir", "..", true},
		{"relative path", "./pkg", true},
		{"relative with subdir", "./pkg/utils", true},
		{"parent relative", "../other", true},
		{"recursive pattern", "./...", true},
		{"deep recursive", "./pkg/...", true},
		{"absolute path", "/home/user/project", true},

		// Module paths
		{"module path", "github.com/cloudposse/atmos", true},
		{"module subpath", "github.com/cloudposse/atmos/pkg", true},

		// Not package paths (test names)
		{"test name", "TestFoo", false},
		{"example name", "ExampleBar", false},
		{"benchmark name", "BenchmarkBaz", false},

		// Ambiguous cases
		{"single word", "utils", false}, // Could be package or test
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IsPackagePath(tt.input)
			assert.Equal(t, tt.expected, result, "IsPackagePath(%q)", tt.input)
		})
	}
}

func TestHasRunFlag(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected bool
	}{
		// Has -run flag
		{"explicit -run", []string{"-run", "TestFoo"}, true},
		{"double dash --run", []string{"--run", "TestFoo"}, true},
		{"run with equals", []string{"-run=TestFoo"}, true},
		{"run with equals double dash", []string{"--run=TestFoo"}, true},
		{"run in middle", []string{"-v", "-run", "TestFoo", "-race"}, true},

		// No -run flag
		{"no flags", []string{"TestFoo"}, false},
		{"other flags", []string{"-v", "-race", "-cover"}, false},
		{"similar flag", []string{"-runtime"}, false},
		{"run as value", []string{"-test", "run"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := HasRunFlag(tt.args)
			assert.Equal(t, tt.expected, result, "HasRunFlag(%v)", tt.args)
		})
	}
}

func TestExtractTestNamesFromArgs(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedFilter string
		expectedPkgs   []string
	}{
		{
			name:           "single test name",
			args:           []string{"TestFoo"},
			expectedFilter: "TestFoo",
			expectedPkgs:   []string{},
		},
		{
			name:           "multiple test names",
			args:           []string{"TestFoo", "TestBar", "TestBaz"},
			expectedFilter: "TestFoo|TestBar|TestBaz",
			expectedPkgs:   []string{},
		},
		{
			name:           "mixed test and package",
			args:           []string{"./pkg", "TestFoo", "./utils"},
			expectedFilter: "TestFoo",
			expectedPkgs:   []string{"./pkg", "./utils"},
		},
		{
			name:           "only packages",
			args:           []string{"./...", "./pkg/utils"},
			expectedFilter: "",
			expectedPkgs:   []string{"./...", "./pkg/utils"},
		},
		{
			name:           "with flags",
			args:           []string{"-v", "TestFoo", "-race"},
			expectedFilter: "TestFoo",
			expectedPkgs:   []string{},
		},
		{
			name:           "example and benchmark",
			args:           []string{"ExampleFoo", "BenchmarkBar"},
			expectedFilter: "ExampleFoo|BenchmarkBar",
			expectedPkgs:   []string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			filter, pkgs := ExtractTestNamesFromArgs(tt.args)
			assert.Equal(t, tt.expectedFilter, filter, "filter")
			assert.Equal(t, tt.expectedPkgs, pkgs, "packages")
		})
	}
}

func TestProcessTestArguments(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectedPkgs   []string
		expectedFilter string
	}{
		{
			name:           "single test name",
			args:           []string{"TestExecute"},
			expectedPkgs:   []string{},
			expectedFilter: "TestExecute",
		},
		{
			name:           "multiple tests",
			args:           []string{"TestA", "TestB"},
			expectedPkgs:   []string{},
			expectedFilter: "TestA|TestB",
		},
		{
			name:           "test with package",
			args:           []string{"./pkg", "TestConfig"},
			expectedPkgs:   []string{"./pkg"},
			expectedFilter: "TestConfig",
		},
		{
			name:           "only packages",
			args:           []string{"./...", "./pkg/utils"},
			expectedPkgs:   []string{"./...", "./pkg/utils"},
			expectedFilter: "",
		},
		{
			name:           "ambiguous single word treated as package",
			args:           []string{"utils"},
			expectedPkgs:   []string{"utils"},
			expectedFilter: "",
		},
		{
			name:           "subtest pattern",
			args:           []string{"TestFoo/subtest"},
			expectedPkgs:   []string{},
			expectedFilter: "TestFoo/subtest",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pkgs, filter := ProcessTestArguments(tt.args)
			assert.Equal(t, tt.expectedPkgs, pkgs, "packages")
			assert.Equal(t, tt.expectedFilter, filter, "filter")
		})
	}
}
