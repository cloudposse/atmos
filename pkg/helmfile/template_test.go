package helmfile

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExtractTargetFlag(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantTarget string
		wantArgs   []string
	}{
		{
			name:       "no target",
			args:       []string{"--state-values-file", "vars.yaml", "template"},
			wantTarget: "",
			wantArgs:   []string{"--state-values-file", "vars.yaml", "template"},
		},
		{
			name:       "space form",
			args:       []string{"template", "--target", "deployment-repo", "--skip-deps"},
			wantTarget: "deployment-repo",
			wantArgs:   []string{"template", "--skip-deps"},
		},
		{
			name:       "equals form",
			args:       []string{"--target=git-repo", "template"},
			wantTarget: "git-repo",
			wantArgs:   []string{"template"},
		},
		{
			name:       "trailing target without value",
			args:       []string{"template", "--target"},
			wantTarget: "",
			wantArgs:   []string{"template"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, args := ExtractTargetFlag(tt.args)
			assert.Equal(t, tt.wantTarget, target)
			assert.Equal(t, tt.wantArgs, args)
		})
	}
}
