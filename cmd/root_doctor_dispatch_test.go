package cmd

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"

	doctorcmd "github.com/cloudposse/atmos/cmd/doctor"
)

// TestDoctorDispatchHonorsEveryRootPersistentFlag guards against root flags missing from a hand-maintained
// allowlist: doctor must be dispatched before configuration loading no matter which root flag precedes it.
func TestDoctorDispatchHonorsEveryRootPersistentFlag(t *testing.T) {
	_ = NewTestKit(t)

	flags := RootCmd.PersistentFlags()
	count := 0
	flags.VisitAll(func(f *pflag.Flag) {
		count++
		t.Run(f.Name, func(t *testing.T) {
			switch {
			case f.Value.Type() == "bool":
				assert.True(t, doctorcmd.IsInvocation([]string{"--" + f.Name, "doctor"}, flags))
				assert.True(t, doctorcmd.IsInvocation([]string{"--" + f.Name + "=true", "doctor"}, flags))
			case f.NoOptDefVal != "":
				assert.True(t, doctorcmd.IsInvocation([]string{"--" + f.Name + "=value", "doctor"}, flags))
				assert.True(t, doctorcmd.IsInvocation([]string{"--" + f.Name, "value", "doctor"}, flags))
				assert.False(t, doctorcmd.IsInvocation([]string{"--" + f.Name, "doctor"}, flags))
			default:
				assert.True(t, doctorcmd.IsInvocation([]string{"--" + f.Name + "=value", "doctor"}, flags))
				assert.True(t, doctorcmd.IsInvocation([]string{"--" + f.Name, "value", "doctor"}, flags))
				// The value belongs to the flag, so a command-looking value must not dispatch doctor.
				assert.False(t, doctorcmd.IsInvocation([]string{"--" + f.Name, "doctor"}, flags))
			}
			if f.Shorthand != "" && f.Value.Type() != "bool" && f.NoOptDefVal == "" {
				assert.True(t, doctorcmd.IsInvocation([]string{"-" + f.Shorthand, "value", "doctor"}, flags))
			}
		})
	})
	assert.Positive(t, count, "root must register persistent flags")
}

func TestDoctorDispatchLeavesOtherCommandsAlone(t *testing.T) {
	_ = NewTestKit(t)

	flags := RootCmd.PersistentFlags()
	for _, args := range [][]string{
		{"terraform", "plan"},
		{"--no-color", "validate", "stacks"},
		{"describe", "doctor"},
		{"--", "doctor"},
	} {
		assert.False(t, doctorcmd.IsInvocation(args, flags), "%v", args)
	}
}

// TestDoctorDispatchFoldsSpaceSeparatedFlagValues guards the early doctor dispatch, which runs before
// preprocessArgs: without the same folding, Cobra reads the value in "--no-color false doctor" or
// "--identity prod doctor" as the command name and doctor never runs.
func TestDoctorDispatchFoldsSpaceSeparatedFlagValues(t *testing.T) {
	_ = NewTestKit(t)

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "global bool flag value", args: []string{"--no-color", "false", "doctor", "--list-checks"}, want: []string{"--no-color=false", "doctor", "--list-checks"}},
		{name: "global optional-value flag", args: []string{"--identity", "prod", "doctor"}, want: []string{"--identity=prod", "doctor"}},
		{name: "doctor's own bool flag value", args: []string{"doctor", "--offline", "true"}, want: []string{"doctor", "--offline=true"}},
		{name: "equals forms are left alone", args: []string{"--no-color=false", "doctor", "--offline=true"}, want: []string{"--no-color=false", "doctor", "--offline=true"}},
		{name: "a bare bool flag does not consume the command", args: []string{"--no-color", "doctor"}, want: []string{"--no-color", "doctor"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.True(t, doctorcmd.IsInvocation(tt.args, RootCmd.PersistentFlags()), "the raw arguments must dispatch doctor")
			assert.Equal(t, tt.want, preprocessNativeFlags(tt.args))
		})
	}
}
