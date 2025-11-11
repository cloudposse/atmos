package merge

import (
	"strings"
	"testing"
)

func TestThreeWayMerger_AutoDetection(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		base     string
		ours     string
		theirs   string
		want     string
		wantErr  bool
	}{
		{
			name:     "detects YAML file with .yaml extension",
			fileName: "config.yaml",
			base: `settings:
  enabled: true
`,
			ours: `settings:
  enabled: true
  timeout: 30
`,
			theirs: `settings:
  enabled: true
  retries: 3
`,
			// Note: YAML merger preserves "ours" key ordering
			want: `settings:
  enabled: true
  timeout: 30
  retries: 3
`,
			wantErr: false,
		},
		{
			name:     "detects YAML file with .yml extension",
			fileName: "config.yml",
			base: `version: 1
`,
			ours: `version: 2
`,
			theirs: `version: 2
`,
			want: `version: 2
`,
			wantErr: false,
		},
		{
			name:     "uses text merge for .txt files",
			fileName: "readme.txt",
			base: `Line 1
Line 2
Line 3
`,
			ours: `Line 1 modified
Line 2
Line 3
`,
			theirs: `Line 1
Line 2
Line 3 modified
`,
			want: `Line 1 modified
Line 2
Line 3 modified
`,
			wantErr: false,
		},
		{
			name:     "uses text merge for .go files (clean merge)",
			fileName: "main.go",
			base: `package main

func main() {
	println("Hello")
}
`,
			ours: `package main

func init() {
	setup()
}

func main() {
	println("Hello")
}
`,
			theirs: `package main

func main() {
	println("Hello")
	println("World")
}
`,
			want: `package main

func init() {
	setup()
}

func main() {
	println("Hello")
	println("World")
}
`,
			wantErr: false,
		},
		{
			name:     "uses text merge for files without extension",
			fileName: "Dockerfile",
			base: `FROM golang:1.21
WORKDIR /app
`,
			ours: `FROM golang:1.22
WORKDIR /app
`,
			theirs: `FROM golang:1.21
WORKDIR /app
COPY . .
`,
			want: `FROM golang:1.22
WORKDIR /app
COPY . .
`,
			wantErr: false,
		},
		{
			name:     "case insensitive extension detection",
			fileName: "config.YAML",
			base: `key: value
`,
			ours: `key: modified
`,
			theirs: `key: modified
`,
			want: `key: modified
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewThreeWayMerger(100) // High threshold for auto-detection tests
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs, tt.fileName)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err == nil {
				// Normalize whitespace for comparison
				got := strings.TrimSpace(result.Content)
				want := strings.TrimSpace(tt.want)

				if got != want {
					t.Errorf("Merge result mismatch\nGot:\n%s\n\nWant:\n%s", got, want)
				}
			}
		})
	}
}

func TestThreeWayMerger_MergeWithStrategy(t *testing.T) {
	tests := []struct {
		name     string
		strategy MergeStrategy
		base     string
		ours     string
		theirs   string
		want     string
		wantErr  bool
	}{
		{
			name:     "explicit YAML strategy",
			strategy: StrategyYAML,
			base: `config:
  feature_a: true
`,
			ours: `config:
  feature_a: true
  feature_b: true
`,
			theirs: `config:
  feature_a: true
  feature_c: true
`,
			want: `config:
  feature_a: true
  feature_b: true
  feature_c: true
`,
			wantErr: false,
		},
		{
			name:     "explicit text strategy",
			strategy: StrategyText,
			base: `Line 1
Line 2
Line 3
Line 4
Line 5
`,
			ours: `Line 1
Line 2 modified by user
Line 3
Line 4
Line 5
`,
			theirs: `Line 1
Line 2
Line 3
Line 4
Line 5 modified by template
`,
			want: `Line 1
Line 2 modified by user
Line 3
Line 4
Line 5 modified by template
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewThreeWayMerger(100) // High threshold for strategy tests
			result, err := merger.MergeWithStrategy(tt.base, tt.ours, tt.theirs, tt.strategy)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err == nil {
				got := strings.TrimSpace(result.Content)
				want := strings.TrimSpace(tt.want)

				if got != want {
					t.Errorf("Merge result mismatch\nGot:\n%s\n\nWant:\n%s", got, want)
				}
			}
		})
	}
}

func TestThreeWayMerger_RealWorldScenarios(t *testing.T) {
	tests := []struct {
		name     string
		fileName string
		base     string
		ours     string
		theirs   string
		want     string
		wantErr  bool
	}{
		{
			name:     "atmos.yaml: user changes path, template adds features",
			fileName: "atmos.yaml",
			base: `components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
`,
			ours: `components:
  terraform:
    base_path: "infrastructure/terraform"
    apply_auto_approve: false
`,
			theirs: `components:
  terraform:
    base_path: "components/terraform"
    apply_auto_approve: false
    deploy_run_init: true
`,
			// Note: YAML merger preserves "ours" key ordering
			want: `components:
  terraform:
    base_path: "infrastructure/terraform"
    apply_auto_approve: false
    deploy_run_init: true
`,
			wantErr: false,
		},
		{
			name:     "Terraform file: user changes value, template adds resource",
			fileName: "main.tf",
			base: `resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}
`,
			ours: `resource "aws_vpc" "main" {
  cidr_block = "10.1.0.0/16"
}
`,
			theirs: `resource "aws_vpc" "main" {
  cidr_block = "10.0.0.0/16"
}

resource "aws_subnet" "public" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}
`,
			want: `resource "aws_vpc" "main" {
  cidr_block = "10.1.0.0/16"
}

resource "aws_subnet" "public" {
  vpc_id     = aws_vpc.main.id
  cidr_block = "10.0.1.0/24"
}
`,
			wantErr: false,
		},
		{
			name:     "stack config YAML: user adds vars, template adds settings",
			fileName: "stacks/prod.yaml",
			base: `components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.0.0.0/16"
`,
			ours: `components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.1.0.0/16"
        enable_dns: true
`,
			theirs: `components:
  terraform:
    vpc:
      settings:
        depends_on:
          - networking/config
      vars:
        cidr_block: "10.0.0.0/16"
`,
			want: `components:
  terraform:
    vpc:
      vars:
        cidr_block: "10.1.0.0/16"
        enable_dns: true
      settings:
        depends_on:
          - networking/config
`,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewThreeWayMerger(100) // High threshold for real-world scenarios
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs, tt.fileName)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err == nil {
				got := strings.TrimSpace(result.Content)
				want := strings.TrimSpace(tt.want)

				if got != want {
					t.Errorf("Merge result mismatch\nGot:\n%s\n\nWant:\n%s", got, want)
				}
			}
		})
	}
}

func TestThreeWayMerger_ConflictHandling(t *testing.T) {
	tests := []struct {
		name      string
		fileName  string
		base      string
		ours      string
		theirs    string
		threshold int
		wantErr   bool
		wantConflicts bool
		wantConflictCount int
	}{
		{
			name:     "YAML conflict: both modify same value",
			fileName: "config.yaml",
			base: `timeout: 30
`,
			ours: `timeout: 60
`,
			theirs: `timeout: 45
`,
			threshold: 100, // High threshold to allow conflict
			wantErr:   false,
			wantConflicts: true,
			wantConflictCount: 1,
		},
		{
			name:      "text conflict: both modify same line",
			fileName:  "file.txt",
			base:      "original line\n",
			ours:      "user version\n",
			theirs:    "template version\n",
			threshold: 100,
			wantErr:   false, // High threshold allows conflict
			wantConflicts: true,
			wantConflictCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			merger := NewThreeWayMerger(tt.threshold)
			result, err := merger.Merge(tt.base, tt.ours, tt.theirs, tt.fileName)

			if tt.wantErr && err == nil {
				t.Errorf("Expected error, got nil")
				return
			}

			if !tt.wantErr && err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if err == nil {
				if result.HasConflicts != tt.wantConflicts {
					t.Errorf("HasConflicts = %v, want %v", result.HasConflicts, tt.wantConflicts)
				}

				if tt.wantConflicts && result.ConflictCount < tt.wantConflictCount {
					t.Errorf("ConflictCount = %d, want at least %d", result.ConflictCount, tt.wantConflictCount)
				}

				if !tt.wantConflicts && result.ConflictCount != 0 {
					t.Errorf("ConflictCount = %d, want 0 for non-conflict scenario", result.ConflictCount)
				}

				if result.HasConflicts {
					t.Logf("Merge completed with %d conflicts (within threshold)", result.ConflictCount)
				}
			}
		})
	}
}
