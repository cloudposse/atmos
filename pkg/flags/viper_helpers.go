package flags

import (
	"fmt"

	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/perf"
)

// bindFlagToViper binds a single flag to Viper with environment variable support.
// This is shared helper code used by flag parsers.
func bindFlagToViper(v *viper.Viper, viperKey string, flag Flag) error {
	defer perf.Track(nil, "flags.bindFlagToViper")()

	// Set default value in Viper so it's returned when flag is not explicitly set.
	// This ensures defaults work correctly for CLI flags, ENV vars, and config files.
	v.SetDefault(viperKey, flag.GetDefault())

	// Special handling for flags with NoOptDefVal (identity pattern)
	if flag.GetNoOptDefVal() != "" {
		envVars := flag.GetEnvVars()
		if len(envVars) > 0 {
			args := make([]string, 0, len(envVars)+1)
			args = append(args, viperKey)
			args = append(args, envVars...)
			if err := v.BindEnv(args...); err != nil {
				return fmt.Errorf("failed to bind env vars for flag %s: %w", flag.GetName(), err)
			}
		}
		return nil
	}

	// Bind environment variables
	envVars := flag.GetEnvVars()
	if len(envVars) > 0 {
		args := make([]string, 0, len(envVars)+1)
		args = append(args, viperKey)
		args = append(args, envVars...)
		if err := v.BindEnv(args...); err != nil {
			return fmt.Errorf("failed to bind env vars for flag %s: %w", flag.GetName(), err)
		}
	}

	return nil
}
