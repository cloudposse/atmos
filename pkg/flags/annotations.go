package flags

import (
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/perf"
)

// annotationValueFromEnv is the pflag annotation key that records which environment variable
// supplied a flag's current value, for flags whose value was copied from the environment rather
// than typed on the command line.
const annotationValueFromEnv = "atmos_value_from_env"

// MarkFlagValueFromEnv records that the flag's current value came from the named environment
// variable. Callers use it after copying an environment value onto a flag that the command line
// did not set, so later validation can tell an explicit `--flag` apart from an ambient
// `ATMOS_*` value. An empty variable name clears a previous mark, so a long-lived flag object
// never reports a stale environment source. A nil flag is a no-op.
func MarkFlagValueFromEnv(f *pflag.Flag, envVar string) {
	defer perf.Track(nil, "flags.MarkFlagValueFromEnv")()

	if f == nil {
		return
	}
	if envVar == "" {
		delete(f.Annotations, annotationValueFromEnv)
		return
	}
	if f.Annotations == nil {
		f.Annotations = map[string][]string{}
	}
	f.Annotations[annotationValueFromEnv] = []string{envVar}
}

// FlagValueFromEnv returns the environment variable that supplied the flag's current value, as
// recorded by MarkFlagValueFromEnv. The boolean result is false when the flag is nil or its value
// did not come from the environment.
func FlagValueFromEnv(f *pflag.Flag) (envVar string, ok bool) {
	defer perf.Track(nil, "flags.FlagValueFromEnv")()

	if f == nil {
		return "", false
	}
	values := f.Annotations[annotationValueFromEnv]
	if len(values) == 0 || values[0] == "" {
		return "", false
	}
	return values[0], true
}
