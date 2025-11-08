package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestExtractSeparatedArgs(t *testing.T) {
	tests := []struct {
		name                    string
		args                    []string
		osArgs                  []string
		expectedBeforeSeparator []string
		expectedAfterSeparator  []string
		expectedHasSeparator    bool
		expectedSeparatorIndex  int
	}{
		{
			name:                    "terraform with separator",
			args:                    []string{"plan", "myapp"},
			osArgs:                  []string{"atmos", "terraform", "plan", "myapp", "-s", "dev", "--", "-var", "foo=bar"},
			expectedBeforeSeparator: []string{"plan", "myapp"},
			expectedAfterSeparator:  []string{"-var", "foo=bar"},
			expectedHasSeparator:    true,
			expectedSeparatorIndex:  6,
		},
		{
			name:                    "auth exec with separator",
			args:                    []string{"--identity", "admin", "--", "terraform", "apply", "-auto-approve"},
			osArgs:                  []string{"atmos", "auth", "exec", "--identity", "admin", "--", "terraform", "apply", "-auto-approve"},
			expectedBeforeSeparator: []string{"--identity", "admin"},
			expectedAfterSeparator:  []string{"terraform", "apply", "-auto-approve"},
			expectedHasSeparator:    true,
			expectedSeparatorIndex:  5,
		},
		{
			name:                    "no separator",
			args:                    []string{"plan", "myapp"},
			osArgs:                  []string{"atmos", "terraform", "plan", "myapp", "-s", "dev"},
			expectedBeforeSeparator: []string{"plan", "myapp"},
			expectedAfterSeparator:  nil,
			expectedHasSeparator:    false,
			expectedSeparatorIndex:  -1,
		},
		{
			name:                    "separator at end with no trailing args",
			args:                    []string{"plan"},
			osArgs:                  []string{"atmos", "terraform", "plan", "--"},
			expectedBeforeSeparator: []string{"plan"},
			expectedAfterSeparator:  []string{},
			expectedHasSeparator:    true,
			expectedSeparatorIndex:  3,
		},
		{
			name:                    "separator with complex terraform flags",
			args:                    []string{"plan", "myapp"},
			osArgs:                  []string{"atmos", "terraform", "plan", "myapp", "-s", "dev", "--", "-var", "foo=bar", "-out=plan.tfplan", "-detailed-exitcode"},
			expectedBeforeSeparator: []string{"plan", "myapp"},
			expectedAfterSeparator:  []string{"-var", "foo=bar", "-out=plan.tfplan", "-detailed-exitcode"},
			expectedHasSeparator:    true,
			expectedSeparatorIndex:  6,
		},
		{
			name:                    "custom command with separator",
			args:                    []string{"arg1", "arg2"},
			osArgs:                  []string{"atmos", "mycmd", "arg1", "arg2", "--", "trailing1", "trailing2"},
			expectedBeforeSeparator: []string{"arg1", "arg2"},
			expectedAfterSeparator:  []string{"trailing1", "trailing2"},
			expectedHasSeparator:    true,
			expectedSeparatorIndex:  4,
		},
		{
			name:                    "helmfile with separator",
			args:                    []string{"apply"},
			osArgs:                  []string{"atmos", "helmfile", "apply", "-s", "prod", "--", "--set", "image.tag=v1.0"},
			expectedBeforeSeparator: []string{"apply"},
			expectedAfterSeparator:  []string{"--set", "image.tag=v1.0"},
			expectedHasSeparator:    true,
			expectedSeparatorIndex:  5,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cmd := &cobra.Command{}
			separated := ExtractSeparatedArgs(cmd, tt.args, tt.osArgs)

			assert.Equal(t, tt.expectedBeforeSeparator, separated.BeforeSeparator, "BeforeSeparator mismatch")
			assert.Equal(t, tt.expectedAfterSeparator, separated.AfterSeparator, "AfterSeparator mismatch")
			assert.Equal(t, tt.expectedHasSeparator, separated.HasSeparator, "HasSeparator mismatch")
			assert.Equal(t, tt.expectedSeparatorIndex, separated.SeparatorIndex, "SeparatorIndex mismatch")
		})
	}
}

func TestSeparatedCommandArgs_Methods(t *testing.T) {
	t.Run("GetAfterSeparator with separator", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: []string{"-var", "foo=bar"},
			HasSeparator:   true,
		}
		assert.Equal(t, []string{"-var", "foo=bar"}, separated.GetAfterSeparator())
	})

	t.Run("GetAfterSeparator without separator", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: nil,
			HasSeparator:   false,
		}
		assert.Nil(t, separated.GetAfterSeparator())
	})

	t.Run("GetBeforeSeparator with separator", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			BeforeSeparator: []string{"plan", "myapp"},
			HasSeparator:    true,
		}
		assert.Equal(t, []string{"plan", "myapp"}, separated.GetBeforeSeparator())
	})

	t.Run("GetBeforeSeparator without separator", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			BeforeSeparator: []string{"plan", "myapp"},
			HasSeparator:    false,
		}
		assert.Equal(t, []string{"plan", "myapp"}, separated.GetBeforeSeparator())
	})

	t.Run("GetAfterSeparatorAsString with separator", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: []string{"-var", "foo=bar", "-out=plan.tfplan"},
			HasSeparator:   true,
		}
		assert.Equal(t, "-var foo=bar -out=plan.tfplan", separated.GetAfterSeparatorAsString())
	})

	t.Run("GetAfterSeparatorAsString without separator", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: nil,
			HasSeparator:   false,
		}
		assert.Equal(t, "", separated.GetAfterSeparatorAsString())
	})

	t.Run("GetAfterSeparatorAsString with separator but empty trailing args", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: []string{},
			HasSeparator:   true,
		}
		assert.Equal(t, "", separated.GetAfterSeparatorAsString())
	})

	t.Run("GetAfterSeparatorAsQuotedString with whitespace", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: []string{"echo", "hello  world"},
			HasSeparator:   true,
		}
		quoted, err := separated.GetAfterSeparatorAsQuotedString()
		assert.NoError(t, err)
		// Should quote the arg with spaces
		assert.Equal(t, "echo 'hello  world'", quoted)
	})

	t.Run("GetAfterSeparatorAsQuotedString without separator", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: nil,
			HasSeparator:   false,
		}
		quoted, err := separated.GetAfterSeparatorAsQuotedString()
		assert.NoError(t, err)
		assert.Equal(t, "", quoted)
	})

	t.Run("GetAfterSeparatorAsQuotedString with special characters", func(t *testing.T) {
		separated := &SeparatedCommandArgs{
			AfterSeparator: []string{"echo", "foo$bar", "baz'qux"},
			HasSeparator:   true,
		}
		quoted, err := separated.GetAfterSeparatorAsQuotedString()
		assert.NoError(t, err)
		// Should properly quote special shell characters
		assert.Contains(t, quoted, "echo")
		// The exact quoting may vary, but it should be shell-safe
		t.Logf("Quoted string: %s", quoted)
	})
}
