package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gopkg.in/yaml.v3"
)

func TestCoverageConfig(t *testing.T) {
	// Test basic CoverageConfig structure
	config := CoverageConfig{
		Enabled:  true,
		Profile:  "coverage.out",
		Packages: []string{"github.com/example/pkg1", "github.com/example/pkg2"},
		Analysis: CoverageAnalysis{
			Functions:  true,
			Statements: true,
			Uncovered:  true,
			Exclude:    []string{"mock_*.go", "*_test.go"},
		},
		Output: CoverageOutput{
			Terminal: TerminalOutput{
				Format:        "detailed",
				ShowUncovered: 10,
			},
		},
		Thresholds: CoverageThresholds{
			Total: 80.0,
			Packages: []PackageThreshold{
				{Pattern: "github.com/example/critical/*", Threshold: 95.0},
				{Pattern: "github.com/example/utils/*", Threshold: 70.0},
			},
			FailUnder: true,
		},
	}

	// Test that fields are properly set
	assert.True(t, config.Enabled)
	assert.Equal(t, "coverage.out", config.Profile)
	assert.Len(t, config.Packages, 2)
	assert.True(t, config.Analysis.Functions)
	assert.True(t, config.Analysis.Statements)
	assert.True(t, config.Analysis.Uncovered)
	assert.Contains(t, config.Analysis.Exclude, "mock_*.go")
	assert.Equal(t, "detailed", config.Output.Terminal.Format)
	assert.Equal(t, 10, config.Output.Terminal.ShowUncovered)
	assert.Equal(t, 80.0, config.Thresholds.Total)
	assert.Len(t, config.Thresholds.Packages, 2)
	assert.True(t, config.Thresholds.FailUnder)
}

func TestCoverageAnalysis(t *testing.T) {
	tests := []struct {
		name     string
		analysis CoverageAnalysis
	}{
		{
			name: "all analysis enabled",
			analysis: CoverageAnalysis{
				Functions:  true,
				Statements: true,
				Uncovered:  true,
				Exclude:    []string{"*.pb.go", "vendor/*"},
			},
		},
		{
			name: "minimal analysis",
			analysis: CoverageAnalysis{
				Functions:  false,
				Statements: true,
				Uncovered:  false,
				Exclude:    []string{},
			},
		},
		{
			name:     "default analysis",
			analysis: CoverageAnalysis{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test serialization to YAML
			data, err := yaml.Marshal(&tt.analysis)
			assert.NoError(t, err)
			assert.NotNil(t, data)

			// Test deserialization from YAML
			var decoded CoverageAnalysis
			err = yaml.Unmarshal(data, &decoded)
			assert.NoError(t, err)
			// Compare fields individually due to nil vs empty slice differences in YAML
			assert.Equal(t, tt.analysis.Functions, decoded.Functions)
			assert.Equal(t, tt.analysis.Statements, decoded.Statements)
			assert.Equal(t, tt.analysis.Uncovered, decoded.Uncovered)
			// Check Exclude - handle nil vs empty slice
			if tt.analysis.Exclude == nil && len(decoded.Exclude) == 0 {
				// Both are empty, that's fine
			} else {
				assert.Equal(t, tt.analysis.Exclude, decoded.Exclude)
			}
		})
	}
}

func TestTerminalOutput(t *testing.T) {
	tests := []struct {
		name     string
		output   TerminalOutput
		expected TerminalOutput
	}{
		{
			name: "summary format",
			output: TerminalOutput{
				Format:        "summary",
				ShowUncovered: 5,
			},
			expected: TerminalOutput{
				Format:        "summary",
				ShowUncovered: 5,
			},
		},
		{
			name: "detailed format",
			output: TerminalOutput{
				Format:        "detailed",
				ShowUncovered: 20,
			},
			expected: TerminalOutput{
				Format:        "detailed",
				ShowUncovered: 20,
			},
		},
		{
			name: "none format",
			output: TerminalOutput{
				Format:        "none",
				ShowUncovered: 0,
			},
			expected: TerminalOutput{
				Format:        "none",
				ShowUncovered: 0,
			},
		},
		{
			name:     "default values",
			output:   TerminalOutput{},
			expected: TerminalOutput{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected.Format, tt.output.Format)
			assert.Equal(t, tt.expected.ShowUncovered, tt.output.ShowUncovered)
		})
	}
}

func TestCoverageThresholds(t *testing.T) {
	tests := []struct {
		name       string
		thresholds CoverageThresholds
		checkTotal float64
		checkFail  bool
	}{
		{
			name: "high threshold with failure",
			thresholds: CoverageThresholds{
				Total: 90.0,
				Packages: []PackageThreshold{
					{Pattern: "*/core/*", Threshold: 95.0},
					{Pattern: "*/utils/*", Threshold: 80.0},
				},
				FailUnder: true,
			},
			checkTotal: 90.0,
			checkFail:  true,
		},
		{
			name: "medium threshold without failure",
			thresholds: CoverageThresholds{
				Total:     75.0,
				Packages:  []PackageThreshold{},
				FailUnder: false,
			},
			checkTotal: 75.0,
			checkFail:  false,
		},
		{
			name:       "default thresholds",
			thresholds: CoverageThresholds{},
			checkTotal: 0.0,
			checkFail:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.checkTotal, tt.thresholds.Total)
			assert.Equal(t, tt.checkFail, tt.thresholds.FailUnder)
		})
	}
}

func TestPackageThreshold(t *testing.T) {
	tests := []struct {
		name      string
		threshold PackageThreshold
	}{
		{
			name: "critical package",
			threshold: PackageThreshold{
				Pattern:   "*/security/*",
				Threshold: 100.0,
			},
		},
		{
			name: "normal package",
			threshold: PackageThreshold{
				Pattern:   "*/pkg/*",
				Threshold: 80.0,
			},
		},
		{
			name: "utility package",
			threshold: PackageThreshold{
				Pattern:   "*/internal/utils/*",
				Threshold: 60.0,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.NotEmpty(t, tt.threshold.Pattern)
			assert.GreaterOrEqual(t, tt.threshold.Threshold, 0.0)
			assert.LessOrEqual(t, tt.threshold.Threshold, 100.0)
		})
	}
}

func TestCoverageConfigYAML(t *testing.T) {
	yamlContent := `
enabled: true
profile: coverage.out
packages:
  - github.com/example/pkg1
  - github.com/example/pkg2
analysis:
  functions: true
  statements: true
  uncovered: true
  exclude:
    - mock_*.go
    - vendor/*
output:
  terminal:
    format: detailed
    show_uncovered: 15
thresholds:
  total: 85.0
  packages:
    - pattern: "*/critical/*"
      threshold: 95.0
    - pattern: "*/utils/*"
      threshold: 70.0
  fail_under: true
`

	var config CoverageConfig
	err := yaml.Unmarshal([]byte(yamlContent), &config)
	assert.NoError(t, err)

	// Verify the unmarshaled configuration
	assert.True(t, config.Enabled)
	assert.Equal(t, "coverage.out", config.Profile)
	assert.Len(t, config.Packages, 2)
	assert.Equal(t, "github.com/example/pkg1", config.Packages[0])
	assert.Equal(t, "github.com/example/pkg2", config.Packages[1])

	// Check analysis settings
	assert.True(t, config.Analysis.Functions)
	assert.True(t, config.Analysis.Statements)
	assert.True(t, config.Analysis.Uncovered)
	assert.Len(t, config.Analysis.Exclude, 2)
	assert.Contains(t, config.Analysis.Exclude, "mock_*.go")
	assert.Contains(t, config.Analysis.Exclude, "vendor/*")

	// Check output settings
	assert.Equal(t, "detailed", config.Output.Terminal.Format)
	assert.Equal(t, 15, config.Output.Terminal.ShowUncovered)

	// Check thresholds
	assert.Equal(t, 85.0, config.Thresholds.Total)
	assert.Len(t, config.Thresholds.Packages, 2)
	assert.Equal(t, "*/critical/*", config.Thresholds.Packages[0].Pattern)
	assert.Equal(t, 95.0, config.Thresholds.Packages[0].Threshold)
	assert.Equal(t, "*/utils/*", config.Thresholds.Packages[1].Pattern)
	assert.Equal(t, 70.0, config.Thresholds.Packages[1].Threshold)
	assert.True(t, config.Thresholds.FailUnder)
}

func TestCoverageConfigDefaults(t *testing.T) {
	// Test that empty config has sensible zero values
	var config CoverageConfig

	assert.False(t, config.Enabled)
	assert.Empty(t, config.Profile)
	assert.Empty(t, config.Packages)
	assert.False(t, config.Analysis.Functions)
	assert.False(t, config.Analysis.Statements)
	assert.False(t, config.Analysis.Uncovered)
	assert.Empty(t, config.Analysis.Exclude)
	assert.Empty(t, config.Output.Terminal.Format)
	assert.Equal(t, 0, config.Output.Terminal.ShowUncovered)
	assert.Equal(t, 0.0, config.Thresholds.Total)
	assert.Empty(t, config.Thresholds.Packages)
	assert.False(t, config.Thresholds.FailUnder)
}

func TestCoverageConfigMarshalUnmarshal(t *testing.T) {
	original := CoverageConfig{
		Enabled:  true,
		Profile:  "test.out",
		Packages: []string{"pkg1", "pkg2", "pkg3"},
		Analysis: CoverageAnalysis{
			Functions:  true,
			Statements: false,
			Uncovered:  true,
			Exclude:    []string{"*.pb.go"},
		},
		Output: CoverageOutput{
			Terminal: TerminalOutput{
				Format:        "summary",
				ShowUncovered: 3,
			},
		},
		Thresholds: CoverageThresholds{
			Total: 75.5,
			Packages: []PackageThreshold{
				{Pattern: "core", Threshold: 90.0},
			},
			FailUnder: true,
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&original)
	assert.NoError(t, err)
	assert.NotNil(t, data)

	// Unmarshal back
	var decoded CoverageConfig
	err = yaml.Unmarshal(data, &decoded)
	assert.NoError(t, err)

	// Verify round-trip
	assert.Equal(t, original.Enabled, decoded.Enabled)
	assert.Equal(t, original.Profile, decoded.Profile)
	assert.Equal(t, original.Packages, decoded.Packages)
	assert.Equal(t, original.Analysis, decoded.Analysis)
	assert.Equal(t, original.Output, decoded.Output)
	assert.Equal(t, original.Thresholds.Total, decoded.Thresholds.Total)
	assert.Equal(t, original.Thresholds.FailUnder, decoded.Thresholds.FailUnder)
	assert.Len(t, decoded.Thresholds.Packages, 1)
	if len(decoded.Thresholds.Packages) > 0 {
		assert.Equal(t, original.Thresholds.Packages[0], decoded.Thresholds.Packages[0])
	}
}
