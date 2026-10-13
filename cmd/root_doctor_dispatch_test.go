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
