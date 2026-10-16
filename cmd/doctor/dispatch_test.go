package doctor

import (
	"testing"

	"github.com/spf13/pflag"
	"github.com/stretchr/testify/assert"
)

// rootFlagSet mimics the arities of the real root persistent flags.
func rootFlagSet() *pflag.FlagSet {
	fs := pflag.NewFlagSet("root", pflag.ContinueOnError)
	fs.Bool("no-color", false, "")
	fs.BoolP("verbose", "v", false, "")
	fs.StringP("chdir", "C", "", "")
	fs.String("logs-level", "", "")
	fs.String("identity", "", "")
	fs.Lookup("identity").NoOptDefVal = "__SELECT__"
	fs.Bool("brand-new-flag", false, "")
	return fs
}

func TestIsInvocation(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		want bool
	}{
		{"bare", []string{"doctor"}, true},
		{"trailing command flags", []string{"doctor", "--fix", "--dry-run"}, true},
		{"empty", nil, false},
		{"other command", []string{"terraform", "plan", "doctor"}, false},
		{"flag not in any legacy allowlist", []string{"--brand-new-flag", "doctor"}, true},
		{"bool flag with explicit literal", []string{"--no-color", "false", "doctor"}, true},
		{"bool flag with equals value", []string{"--no-color=false", "doctor"}, true},
		{"value flag with separate value", []string{"--logs-level", "Debug", "doctor"}, true},
		{"value flag with inline value", []string{"--logs-level=Debug", "doctor"}, true},
		{"value flag consumes a command-looking value", []string{"--logs-level", "doctor", "version"}, false},
		{"missing value", []string{"--logs-level"}, false},
		{"shorthand value", []string{"-C", "dir", "doctor"}, true},
		{"shorthand attached value", []string{"-Cdir", "doctor"}, true},
		{"shorthand bool", []string{"-v", "doctor"}, true},
		{"clustered shorthand bools", []string{"-vh", "doctor"}, true},
		{"help shorthand", []string{"-h", "doctor"}, true},
		{"optional value flag takes the value", []string{"--identity", "prod", "doctor"}, true},
		{"optional value flag stays bare before the command", []string{"--identity", "--no-color", "doctor"}, true},
		{"unknown flag is treated as value-less", []string{"--skip-hooks", "doctor"}, true},
		{"unknown flag with inline value", []string{"--skip-hooks=a,b", "doctor"}, true},
		{"many leading flags", []string{"--no-color", "-C", "dir", "--logs-level=Info", "--skip-hooks", "doctor", "--format=json"}, true},
		{"terminator stops the scan", []string{"--", "doctor"}, false},
		{"lone dash is a positional", []string{"-", "doctor"}, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, IsInvocation(tt.args, rootFlagSet()))
		})
	}
}

func TestIsInvocationWithoutFlagSetTreatsFlagsAsValueLess(t *testing.T) {
	assert.True(t, IsInvocation([]string{"--anything", "doctor"}, nil))
	assert.True(t, IsInvocation([]string{"-x", "doctor"}, nil))
	assert.False(t, IsInvocation([]string{"version"}, nil))
}
