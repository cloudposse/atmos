package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestFilterPlanDiffFlags(t *testing.T) {
	tests := []struct {
		name     string
		args     []string
		expected []string
	}{
		{
			name:     "no flags to filter",
			args:     []string{"--var=foo=bar", "--auto-approve"},
			expected: []string{"--var=foo=bar", "--auto-approve"},
		},
		{
			name:     "with --orig= and --new= flags",
			args:     []string{"--orig=orig.plan", "--new=new.plan", "--var=foo=bar"},
			expected: []string{"--var=foo=bar"},
		},
		{
			name:     "with --orig and --new flags with space",
			args:     []string{"--orig", "orig.plan", "--var=foo=bar", "--new", "new.plan"},
			expected: []string{"--var=foo=bar"},
		},
		{
			name:     "with --orig flag but missing value",
			args:     []string{"--orig", "--var=foo=bar", "--new=new.plan"},
			expected: []string{"--var=foo=bar"},
		},
		{
			name:     "with mixed flags",
			args:     []string{"--var=foo=bar", "--orig=orig.plan", "--auto-approve", "--new=new.plan"},
			expected: []string{"--var=foo=bar", "--auto-approve"},
		},
		{
			name:     "with -var flag using space (integration test format)",
			args:     []string{"--orig=orig.plan", "-var", "foo=new-value"},
			expected: []string{"-var", "foo=new-value"},
		},
		{
			name:     "preserves multiple -var flags",
			args:     []string{"--orig", "orig.plan", "-var", "foo=bar", "-var", "baz=qux", "--new", "new.plan"},
			expected: []string{"-var", "foo=bar", "-var", "baz=qux"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := filterPlanDiffFlags(tc.args)
			assert.Equal(t, tc.expected, result)
		})
	}
}
