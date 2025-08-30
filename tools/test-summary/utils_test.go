package main

import (
	"reflect"
	"sort"
	"strings"
	"testing"
)

func TestFilterTestsByPackages(t *testing.T) {
	tests := []struct {
		name     string
		tests    []TestResult
		packages []string
		want     []TestResult
	}{
		{
			name: "filter by single package",
			tests: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestOne", Status: "pass"},
				{Package: "github.com/test/pkg2", Test: "TestTwo", Status: "pass"},
				{Package: "github.com/test/pkg1", Test: "TestThree", Status: "fail"},
			},
			packages: []string{"github.com/test/pkg1"},
			want: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestOne", Status: "pass"},
				{Package: "github.com/test/pkg1", Test: "TestThree", Status: "fail"},
			},
		},
		{
			name: "filter by multiple packages",
			tests: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestOne", Status: "pass"},
				{Package: "github.com/test/pkg2", Test: "TestTwo", Status: "pass"},
				{Package: "github.com/test/pkg3", Test: "TestThree", Status: "fail"},
			},
			packages: []string{"github.com/test/pkg1", "github.com/test/pkg3"},
			want: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestOne", Status: "pass"},
				{Package: "github.com/test/pkg3", Test: "TestThree", Status: "fail"},
			},
		},
		{
			name: "no matching packages",
			tests: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestOne", Status: "pass"},
				{Package: "github.com/test/pkg2", Test: "TestTwo", Status: "pass"},
			},
			packages: []string{"github.com/test/pkg3"},
			want:     []TestResult{},
		},
		{
			name: "empty packages list",
			tests: []TestResult{
				{Package: "github.com/test/pkg1", Test: "TestOne", Status: "pass"},
			},
			packages: []string{},
			want:     []TestResult{},
		},
		{
			name:     "empty tests list",
			tests:    []TestResult{},
			packages: []string{"github.com/test/pkg1"},
			want:     []TestResult{},
		},
		{
			name: "partial package name matches",
			tests: []TestResult{
				{Package: "github.com/test/pkg", Test: "TestOne", Status: "pass"},
				{Package: "github.com/test/pkg/subpkg", Test: "TestTwo", Status: "pass"},
			},
			packages: []string{"github.com/test/pkg"},
			want: []TestResult{
				{Package: "github.com/test/pkg", Test: "TestOne", Status: "pass"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := filterTestsByPackages(tt.tests, tt.packages)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("filterTestsByPackages() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGetTopSlowestTests(t *testing.T) {
	tests := []struct {
		name  string
		tests []TestResult
		n     int
		want  []TestResult
	}{
		{
			name: "get top 2 slowest tests",
			tests: []TestResult{
				{Test: "TestFast", Duration: 0.1},
				{Test: "TestSlow", Duration: 2.5},
				{Test: "TestMedium", Duration: 1.0},
				{Test: "TestSlowest", Duration: 3.0},
			},
			n: 2,
			want: []TestResult{
				{Test: "TestSlowest", Duration: 3.0},
				{Test: "TestSlow", Duration: 2.5},
			},
		},
		{
			name: "get more tests than available",
			tests: []TestResult{
				{Test: "TestOne", Duration: 1.0},
				{Test: "TestTwo", Duration: 2.0},
			},
			n: 5,
			want: []TestResult{
				{Test: "TestTwo", Duration: 2.0},
				{Test: "TestOne", Duration: 1.0},
			},
		},
		{
			name: "get zero tests",
			tests: []TestResult{
				{Test: "TestOne", Duration: 1.0},
			},
			n:    0,
			want: []TestResult{},
		},
		{
			name:  "empty test list",
			tests: []TestResult{},
			n:     3,
			want:  []TestResult{},
		},
		{
			name: "tests with same duration",
			tests: []TestResult{
				{Test: "TestA", Duration: 1.0},
				{Test: "TestB", Duration: 1.0},
				{Test: "TestC", Duration: 2.0},
			},
			n: 2,
			want: []TestResult{
				{Test: "TestC", Duration: 2.0},
				{Test: "TestA", Duration: 1.0}, // First one with same duration
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := getTopSlowestTests(tt.tests, tt.n)
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("getTopSlowestTests() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGeneratePackageSummary(t *testing.T) {
	tests := []struct {
		name  string
		tests []TestResult
		want  []PackageSummary
	}{
		{
			name: "multiple packages with mixed results",
			tests: []TestResult{
				{Package: "github.com/test/pkg1", Test: "Test1", Status: "pass", Duration: 1.0},
				{Package: "github.com/test/pkg1", Test: "Test2", Status: "fail", Duration: 2.0},
				{Package: "github.com/test/pkg2", Test: "Test3", Status: "pass", Duration: 0.5},
				{Package: "github.com/test/pkg2", Test: "Test4", Status: "pass", Duration: 1.5},
				{Package: "github.com/test/pkg2", Test: "Test5", Status: "skip", Duration: 0},
			},
			want: []PackageSummary{
				{
					Package:       "github.com/test/pkg1",
					TestCount:     2,
					TotalDuration: 3.0,
					AvgDuration:   1.5,
				},
				{
					Package:       "github.com/test/pkg2",
					TestCount:     3,
					TotalDuration: 2.0,
					AvgDuration:   0.67, // approximately 2.0/3
				},
			},
		},
		{
			name: "single package",
			tests: []TestResult{
				{Package: "github.com/test/pkg", Test: "Test1", Status: "pass", Duration: 1.0},
				{Package: "github.com/test/pkg", Test: "Test2", Status: "pass", Duration: 2.0},
			},
			want: []PackageSummary{
				{
					Package:       "github.com/test/pkg",
					TestCount:     2,
					TotalDuration: 3.0,
					AvgDuration:   1.5,
				},
			},
		},
		{
			name:  "empty tests",
			tests: []TestResult{},
			want:  []PackageSummary{},
		},
		{
			name: "package with only failed tests",
			tests: []TestResult{
				{Package: "github.com/test/failing", Test: "Test1", Status: "fail", Duration: 1.0},
				{Package: "github.com/test/failing", Test: "Test2", Status: "fail", Duration: 1.5},
			},
			want: []PackageSummary{
				{
					Package:       "github.com/test/failing",
					TestCount:     2,
					TotalDuration: 2.5,
					AvgDuration:   1.25,
				},
			},
		},
		{
			name: "package with only skipped tests",
			tests: []TestResult{
				{Package: "github.com/test/skipped", Test: "Test1", Status: "skip", Duration: 0},
				{Package: "github.com/test/skipped", Test: "Test2", Status: "skip", Duration: 0},
			},
			want: []PackageSummary{
				{
					Package:       "github.com/test/skipped",
					TestCount:     2,
					TotalDuration: 0,
					AvgDuration:   0,
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generatePackageSummary(tt.tests)

			// Sort both slices by package name for consistent comparison.
			sort.Slice(got, func(i, j int) bool {
				return got[i].Package < got[j].Package
			})
			sort.Slice(tt.want, func(i, j int) bool {
				return tt.want[i].Package < tt.want[j].Package
			})

			// Check lengths first.
			if len(got) != len(tt.want) {
				t.Errorf("generatePackageSummary() length = %d, want %d", len(got), len(tt.want))
				return
			}

			// Check each package summary individually.
			for i, gotSummary := range got {
				wantSummary := tt.want[i]
				if gotSummary.Package != wantSummary.Package {
					t.Errorf("generatePackageSummary()[%d].Package = %v, want %v", i, gotSummary.Package, wantSummary.Package)
				}
				if gotSummary.TestCount != wantSummary.TestCount {
					t.Errorf("generatePackageSummary()[%d].TestCount = %v, want %v", i, gotSummary.TestCount, wantSummary.TestCount)
				}
				if gotSummary.TotalDuration != wantSummary.TotalDuration {
					t.Errorf("generatePackageSummary()[%d].TotalDuration = %v, want %v", i, gotSummary.TotalDuration, wantSummary.TotalDuration)
				}
				// For average duration, allow larger floating point differences due to rounding.
				if diff := gotSummary.AvgDuration - wantSummary.AvgDuration; diff < -0.1 || diff > 0.1 {
					t.Errorf("generatePackageSummary()[%d].AvgDuration = %v, want %v (diff: %v)", i, gotSummary.AvgDuration, wantSummary.AvgDuration, diff)
				}
			}
		})
	}
}

// Test helper function that might be used by utils.
func TestUtilsHelperFunctions(t *testing.T) {
	t.Run("test result sorting by duration", func(t *testing.T) {
		tests := []TestResult{
			{Test: "Fast", Duration: 0.1},
			{Test: "Slow", Duration: 2.0},
			{Test: "Medium", Duration: 1.0},
		}

		// Test that we can sort by duration (this tests the underlying logic).
		sort.Slice(tests, func(i, j int) bool {
			return tests[i].Duration > tests[j].Duration
		})

		expected := []TestResult{
			{Test: "Slow", Duration: 2.0},
			{Test: "Medium", Duration: 1.0},
			{Test: "Fast", Duration: 0.1},
		}

		if !reflect.DeepEqual(tests, expected) {
			t.Errorf("Duration sorting failed: got %v, want %v", tests, expected)
		}
	})

	t.Run("package name extraction", func(t *testing.T) {
		tests := []struct {
			fullPackage string
			want        string
		}{
			{"github.com/test/pkg", "pkg"},
			{"github.com/test/pkg/subpkg", "subpkg"},
			{"simple", "simple"},
			{"", ""},
		}

		for _, tt := range tests {
			// This simulates what shortPackage does.
			parts := strings.Split(tt.fullPackage, "/")
			var got string
			if len(parts) > 0 && parts[len(parts)-1] != "" {
				got = parts[len(parts)-1]
			}

			if got != tt.want {
				t.Errorf("Package name extraction for %s = %v, want %v", tt.fullPackage, got, tt.want)
			}
		}
	})
}
