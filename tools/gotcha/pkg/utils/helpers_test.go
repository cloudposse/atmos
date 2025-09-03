package utils

import (
	"testing"
)

func TestFilterPackages(t *testing.T) {
	tests := []struct {
		name            string
		packages        []string
		includePatterns string
		excludePatterns string
		want            []string
		wantErr         bool
	}{
		{
			name:            "include all packages",
			packages:        []string{"pkg1", "pkg2", "pkg3"},
			includePatterns: ".*",
			excludePatterns: "",
			want:            []string{"pkg1", "pkg2", "pkg3"},
			wantErr:         false,
		},
		{
			name:            "include specific pattern",
			packages:        []string{"api/v1", "api/v2", "internal/config", "cmd/main"},
			includePatterns: "api/.*",
			excludePatterns: "",
			want:            []string{"api/v1", "api/v2"},
			wantErr:         false,
		},
		{
			name:            "exclude pattern",
			packages:        []string{"pkg/main", "pkg/mock", "pkg/test"},
			includePatterns: ".*",
			excludePatterns: "mock",
			want:            []string{"pkg/main", "pkg/test"},
			wantErr:         false,
		},
		{
			name:            "include and exclude patterns",
			packages:        []string{"api/v1", "api/v2", "api/mock", "internal/config"},
			includePatterns: "api/.*",
			excludePatterns: "mock",
			want:            []string{"api/v1", "api/v2"},
			wantErr:         false,
		},
		{
			name:            "multiple include patterns",
			packages:        []string{"api/v1", "api/v2", "cmd/main", "internal/config"},
			includePatterns: "api/.*,cmd/.*",
			excludePatterns: "",
			want:            []string{"api/v1", "api/v2", "cmd/main"},
			wantErr:         false,
		},
		{
			name:            "multiple exclude patterns",
			packages:        []string{"pkg/main", "pkg/mock", "pkg/test", "pkg/fake"},
			includePatterns: ".*",
			excludePatterns: "mock,fake",
			want:            []string{"pkg/main", "pkg/test"},
			wantErr:         false,
		},
		{
			name:            "empty packages",
			packages:        []string{},
			includePatterns: ".*",
			excludePatterns: "",
			want:            []string{},
			wantErr:         false,
		},
		{
			name:            "no patterns",
			packages:        []string{"pkg1", "pkg2"},
			includePatterns: "",
			excludePatterns: "",
			want:            []string{"pkg1", "pkg2"},
			wantErr:         false,
		},
		{
			name:            "invalid include regex",
			packages:        []string{"pkg1"},
			includePatterns: "[",
			excludePatterns: "",
			want:            nil,
			wantErr:         true,
		},
		{
			name:            "invalid exclude regex",
			packages:        []string{"pkg1"},
			includePatterns: ".*",
			excludePatterns: "[",
			want:            nil,
			wantErr:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := FilterPackages(tt.packages, tt.includePatterns, tt.excludePatterns)

			if (err != nil) != tt.wantErr {
				t.Errorf("FilterPackages() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("FilterPackages() got %d packages, want %d", len(got), len(tt.want))
					return
				}

				for i, pkg := range got {
					if pkg != tt.want[i] {
						t.Errorf("FilterPackages() got[%d] = %v, want %v", i, pkg, tt.want[i])
					}
				}
			}
		})
	}
}

func TestIsValidShowFilter(t *testing.T) {
	tests := []struct {
		name  string
		show  string
		valid bool
	}{
		{"valid all", "all", true},
		{"valid failed", "failed", true},
		{"valid passed", "passed", true},
		{"valid skipped", "skipped", true},
		{"valid collapsed", "collapsed", true},
		{"valid none", "none", true},
		{"invalid filter", "invalid", false},
		{"empty string", "", false},
		{"case sensitive", "ALL", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsValidShowFilter(tt.show)
			if got != tt.valid {
				t.Errorf("IsValidShowFilter(%q) = %v, want %v", tt.show, got, tt.valid)
			}
		})
	}
}