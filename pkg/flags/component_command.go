package flags

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// RegisterComponentCommandFlags adds an executable override to a subprocess-backed component family.
// SDK-backed component families do not have an executable to override.
func RegisterComponentCommandFlags(cmd *cobra.Command, componentType string) {
	defer perf.Track(nil, "flags.RegisterComponentCommandFlags")()

	alias := componentType + "-command"
	NewStandardParser(
		WithStringFlag("command", "", "", "Executable name or path for this component invocation"),
		WithStringFlag(alias, "", "", "Compatibility alias for --command"),
	).RegisterPersistentFlags(cmd)
	cmd.MarkFlagsMutuallyExclusive("command", alias)
}

// ComponentCommand reads an explicitly selected executable without changing global configuration.
func ComponentCommand(cmd *cobra.Command, componentType string) (string, error) {
	defer perf.Track(nil, "flags.ComponentCommand")()

	var value string
	for _, name := range []string{"command", componentType + "-command"} {
		flag := cmd.Flag(name)
		if flag == nil || !flag.Changed {
			continue
		}
		if value != "" {
			return "", fmt.Errorf("%w: use either --command or --%s-command", errUtils.ErrInvalidFlag, componentType)
		}
		value = flag.Value.String()
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("%w: --%s requires an executable name or path", errUtils.ErrInvalidFlag, name)
		}
	}
	return value, nil
}
