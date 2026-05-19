package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestSanitizeTerraformWorkspaceEnv exercises the blocklist filter at every
// edge it can hit: blocked keys, preserved per-subcommand variants, malformed
// entries, and empty inputs. Each row names a representative concrete value
// rather than just "x=y" so a regression that miscategorizes a real variable
// fails with a recognizable message.
func TestSanitizeTerraformWorkspaceEnv(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want []string
	}{
		{
			name: "nil input returns nil",
			in:   nil,
			want: nil,
		},
		{
			name: "empty input returns empty",
			in:   []string{},
			want: []string{},
		},
		{
			name: "TF_CLI_ARGS dropped — the user-reported bug",
			in:   []string{"TF_CLI_ARGS=-lock-timeout=10m"},
			want: []string{},
		},
		{
			name: "TF_CLI_ARGS with empty value still dropped",
			in:   []string{"TF_CLI_ARGS="},
			want: []string{},
		},
		{
			name: "TF_CLI_ARGS_workspace dropped — same hazard, narrower scope",
			in:   []string{"TF_CLI_ARGS_workspace=-no-color"},
			want: []string{},
		},
		{
			name: "TF_CLI_ARGS_plan preserved — only applies to `tofu plan`",
			in:   []string{"TF_CLI_ARGS_plan=-lock-timeout=10m"},
			want: []string{"TF_CLI_ARGS_plan=-lock-timeout=10m"},
		},
		{
			name: "TF_CLI_ARGS_apply preserved — only applies to `tofu apply`",
			in:   []string{"TF_CLI_ARGS_apply=-parallelism=4"},
			want: []string{"TF_CLI_ARGS_apply=-parallelism=4"},
		},
		{
			name: "TF_CLI_ARGS_init preserved — only applies to `tofu init`",
			in:   []string{"TF_CLI_ARGS_init=-upgrade"},
			want: []string{"TF_CLI_ARGS_init=-upgrade"},
		},
		{
			name: "TF_VAR_* preserved — not a CLI args injector",
			in:   []string{"TF_VAR_region=us-east-1"},
			want: []string{"TF_VAR_region=us-east-1"},
		},
		{
			name: "TF_LOG preserved — not a CLI args injector",
			in:   []string{"TF_LOG=DEBUG"},
			want: []string{"TF_LOG=DEBUG"},
		},
		{
			name: "non-TF env preserved",
			in:   []string{"PATH=/usr/bin", "HOME=/home/user"},
			want: []string{"PATH=/usr/bin", "HOME=/home/user"},
		},
		{
			name: "entries without '=' preserved (cannot classify)",
			in:   []string{"NO_EQUALS_SIGN", "PATH=/usr/bin"},
			want: []string{"NO_EQUALS_SIGN", "PATH=/usr/bin"},
		},
		{
			name: "value containing '=' preserved correctly (only first '=' splits)",
			in:   []string{"AWS_PROFILE=foo=bar"},
			want: []string{"AWS_PROFILE=foo=bar"},
		},
		{
			name: "mixed: blocked entries removed, others preserved, order kept",
			in: []string{
				"PATH=/usr/bin",
				"TF_CLI_ARGS=-lock-timeout=10m",
				"TF_CLI_ARGS_plan=-lock-timeout=10m",
				"TF_VAR_region=us-east-1",
				"TF_CLI_ARGS_workspace=-no-color",
				"TF_LOG=DEBUG",
			},
			want: []string{
				"PATH=/usr/bin",
				"TF_CLI_ARGS_plan=-lock-timeout=10m",
				"TF_VAR_region=us-east-1",
				"TF_LOG=DEBUG",
			},
		},
		{
			name: "key prefix match must be exact (TF_CLI_ARGS_PLAN_X kept)",
			// Defensive: TF_CLI_ARGS_PLAN_X is not a real env var, but if a user
			// sets some custom TF_CLI_ARGS_<weird-suffix> it should pass through.
			// Only the exact blocklist keys are removed.
			in:   []string{"TF_CLI_ARGS_PLAN_X=value"},
			want: []string{"TF_CLI_ARGS_PLAN_X=value"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeTerraformWorkspaceEnv(tt.in)
			if tt.want == nil {
				assert.Nil(t, got)
				return
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestSanitizeTerraformWorkspaceEnv_DoesNotMutateInput verifies the helper
// does not mutate the caller's slice — important because callers typically
// pass os.Environ() or a struct field they intend to use elsewhere.
func TestSanitizeTerraformWorkspaceEnv_DoesNotMutateInput(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"TF_CLI_ARGS=-lock-timeout=10m",
		"TF_CLI_ARGS_plan=-lock-timeout=10m",
	}
	original := make([]string, len(in))
	copy(original, in)

	_ = sanitizeTerraformWorkspaceEnv(in)

	assert.Equal(t, original, in,
		"sanitizeTerraformWorkspaceEnv must not mutate the caller's slice")
}
