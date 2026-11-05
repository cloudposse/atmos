package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProcessArgsAndFlags_BoolFlagSpaceValues(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantLogsColor string
		wantDryRun    bool
		wantSkipInit  bool
		wantAdditions []string
	}{
		{
			name:          "logs-color false is consumed and stripped",
			args:          []string{"plan", "vpc", "--logs-color", "false", "-s", "dev"},
			wantLogsColor: "false",
			wantAdditions: nil,
		},
		{
			name:          "logs-color TRUE is consumed and stripped",
			args:          []string{"plan", "vpc", "--logs-color", "TRUE"},
			wantLogsColor: "true",
			wantAdditions: nil,
		},
		{
			name:          "bare logs-color keeps following native flag",
			args:          []string{"plan", "vpc", "--logs-color", "-lock=false"},
			wantLogsColor: "true",
			wantAdditions: []string{"-lock=false"},
		},
		{
			name:          "bare logs-color keeps following non-literal",
			args:          []string{"plan", "vpc", "--logs-color", "1"},
			wantLogsColor: "true",
			wantAdditions: []string{"1"},
		},
		{
			name:          "logs-color equals false stripped",
			args:          []string{"plan", "vpc", "--logs-color=false"},
			wantLogsColor: "false",
			wantAdditions: nil,
		},
		{
			name:          "dry-run false",
			args:          []string{"plan", "vpc", "--dry-run", "false"},
			wantDryRun:    false,
			wantAdditions: nil,
		},
		{
			name:          "dry-run true",
			args:          []string{"plan", "vpc", "--dry-run", "true"},
			wantDryRun:    true,
			wantAdditions: nil,
		},
		{
			name:          "bare dry-run does not eat component",
			args:          []string{"plan", "--dry-run", "vpc"},
			wantDryRun:    true,
			wantAdditions: nil,
		},
		{
			name:          "skip-init false",
			args:          []string{"plan", "vpc", "--skip-init", "false"},
			wantSkipInit:  false,
			wantAdditions: nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := processArgsAndFlags("terraform", tt.args)
			require.NoError(t, err)
			assert.Equal(t, "plan", info.SubCommand)
			assert.Equal(t, tt.wantLogsColor, info.LogsColor)
			assert.Equal(t, tt.wantDryRun, info.DryRun)
			assert.Equal(t, tt.wantSkipInit, info.SkipInit)
			assert.Equal(t, "vpc", info.ComponentFromArg)
			assert.Equal(t, tt.wantAdditions, info.AdditionalArgsAndFlags)
			assert.NotContains(t, info.AdditionalArgsAndFlags, "false")
			assert.NotContains(t, info.AdditionalArgsAndFlags, "--logs-color")
			assert.NotContains(t, info.AdditionalArgsAndFlags, "--dry-run")
			assert.NotContains(t, info.AdditionalArgsAndFlags, "--skip-init")
		})
	}
}
