package flags

import (
	"slices"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandPath returns the canonical registered path selected by argv, without
// positionals or flags. Aliases resolve to the same path as the original command.
func (c *CommandCatalog) CommandPath(argv []string) []string {
	defer perf.Track(nil, "flags.CommandCatalog.CommandPath")()

	if c == nil || len(argv) == 0 || c.commands[argv[0]] == nil {
		return nil
	}
	return slices.Clone(c.commands[argv[0]].resolve(argv[1:]).path)
}

// WithDefaultFlag adds a flag only when the selected command supports it and
// argv does not already supply it. The caller's argument slice is unchanged.
func (c *CommandCatalog) WithDefaultFlag(argv, commandPath []string, name, value string) []string {
	defer perf.Track(nil, "flags.CommandCatalog.WithDefaultFlag")()

	if c == nil || len(argv) == 0 || len(commandPath) == 0 || c.commands[argv[0]] == nil {
		return argv
	}
	entry := c.commands[argv[0]].resolve(commandPath[1:])
	spelling := entry.flags[name]
	present, index := entry.flagPosition(argv[1:], spelling)
	if spelling == "" || present {
		return argv
	}
	return slices.Insert(slices.Clone(argv), index+1, spelling+"="+value)
}

func (entry *commandEntry) flagPosition(args []string, spelling string) (bool, int) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == endOfOptionsArg {
			return false, i
		}
		if !strings.HasPrefix(arg, flagPrefix) {
			continue
		}
		name, _, assigned := strings.Cut(arg, flagAssignmentSeparator)
		key := strings.TrimLeft(name, flagPrefix)
		if entry.flags[key] == spelling {
			return true, i
		}
		// pflag permits an attached value on a short flag, such as -fyaml.
		if entry.attachedShortFlag(name, spelling) {
			return true, i
		}
		if entry.takesValue[name] && !assigned {
			i++
		}
	}
	return false, len(args)
}

func (entry *commandEntry) attachedShortFlag(name, spelling string) bool {
	return strings.HasPrefix(name, "-") && !strings.HasPrefix(name, "--") && len(name) > 2 && entry.flags[name[1:2]] == spelling
}
