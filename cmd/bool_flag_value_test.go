package cmd

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsVersionCommandWithArgs_BoolFlagValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want bool
	}{
		{"logs-color false before version", []string{"--logs-color", "false", "version"}, true},
		{"no-color TRUE before version", []string{"--no-color", "TRUE", "version"}, true},
		{"bare no-color before version", []string{"--no-color", "version"}, true},
		{"bool literal then --version", []string{"--logs-color", "false", "--version"}, true},
		{"bool literal then terraform", []string{"--logs-color", "false", "terraform", "plan", "--version"}, false},
		{"literal is not a command", []string{"--no-color", "false"}, false},
		{"after separator", []string{"--", "--logs-color", "false", "version"}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, isVersionCommandWithArgs(append([]string{"atmos"}, tt.args...)))
		})
	}
}

func TestSkipLeadingRootFlags_BoolFlagValues(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{"literal consumed", []string{"--logs-color", "false", "validate", "config"}, []string{"validate", "config"}},
		{"command kept", []string{"--logs-color", "validate", "config"}, []string{"validate", "config"}},
		{"equals form", []string{"--logs-color=false", "validate"}, []string{"validate"}},
		{"numeric is a command token", []string{"--logs-color", "0", "validate"}, []string{"0", "validate"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := skipLeadingRootFlags(tt.args)
			assert.True(t, ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestPreprocessCommandBoolFlags(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "subcommand flag false literal",
			args: []string{"terraform", "plan", "vpc", "-s", "dev", "--dry-run", "false"},
			want: []string{"terraform", "plan", "vpc", "-s", "dev", "--dry-run=false"},
		},
		{
			name: "flag before subcommand",
			args: []string{"terraform", "--dry-run", "TRUE", "plan", "vpc"},
			want: []string{"terraform", "--dry-run=TRUE", "plan", "vpc"},
		},
		{
			name: "bare flag keeps component",
			args: []string{"terraform", "plan", "--dry-run", "vpc", "-s", "dev"},
			want: []string{"terraform", "plan", "--dry-run", "vpc", "-s", "dev"},
		},
		{
			name: "non-literal value untouched",
			args: []string{"terraform", "plan", "vpc", "--dry-run", "1"},
			want: []string{"terraform", "plan", "vpc", "--dry-run", "1"},
		},
		{
			name: "after separator untouched",
			args: []string{"terraform", "plan", "vpc", "--", "--dry-run", "false"},
			want: []string{"terraform", "plan", "vpc", "--", "--dry-run", "false"},
		},
		{
			name: "string flag untouched",
			args: []string{"terraform", "plan", "vpc", "--stack", "false"},
			want: []string{"terraform", "plan", "vpc", "--stack", "false"},
		},
		{
			name: "unknown flag untouched",
			args: []string{"terraform", "plan", "vpc", "--not-a-flag", "false"},
			want: []string{"terraform", "plan", "vpc", "--not-a-flag", "false"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, preprocessCommandBoolFlags(tt.args))
		})
	}
}
