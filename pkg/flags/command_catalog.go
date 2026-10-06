package flags

import (
	"maps"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
)

// CommandCatalog is an immutable view of registered commands and their flag
// spellings. Embedded clients use it without accessing mutable Cobra state.
type CommandCatalog struct {
	commands map[string]*commandEntry
}

type commandEntry struct {
	flags      map[string]string
	takesValue map[string]bool
	children   map[string]*commandEntry
}

// CompatibilityLookup supplies the registry's compatibility flags for a command path.
type CompatibilityLookup func(path []string) map[string]compat.CompatibilityFlag

// NewCommandCatalog snapshots the command tree after flag and command registration.
// Parsing and validation remain the responsibility of the invoked CLI command.
func NewCommandCatalog(root *cobra.Command, lookup CompatibilityLookup) *CommandCatalog {
	defer perf.Track(nil, "flags.NewCommandCatalog")()

	catalog := &CommandCatalog{commands: make(map[string]*commandEntry)}
	for _, command := range root.Commands() {
		entry := snapshotCommand(command, []string{command.Name()}, lookup)
		catalog.commands[command.Name()] = entry
		for _, alias := range command.Aliases {
			catalog.commands[alias] = entry
		}
	}
	return catalog
}

func snapshotCommand(command *cobra.Command, path []string, lookup CompatibilityLookup) *commandEntry {
	entry := &commandEntry{flags: make(map[string]string), takesValue: make(map[string]bool), children: make(map[string]*commandEntry)}
	if lookup != nil {
		for name, flag := range lookup(path) {
			spelling := name
			if flag.Behavior == compat.MapToAtmosFlag {
				spelling = flag.Target
			}
			entry.flags[strings.TrimLeft(name, flagPrefix)] = spelling
		}
	}
	add := func(flag *pflag.Flag) {
		entry.flags[flag.Name] = longFlagPrefix + flag.Name
		entry.takesValue["--"+flag.Name] = flag.NoOptDefVal == ""
		if flag.Shorthand != "" {
			entry.flags[flag.Shorthand] = longFlagPrefix + flag.Name
			entry.takesValue["-"+flag.Shorthand] = flag.NoOptDefVal == ""
		}
	}
	command.InheritedFlags().VisitAll(add)
	command.PersistentFlags().VisitAll(add)
	command.Flags().VisitAll(add)
	for _, child := range command.Commands() {
		childEntry := snapshotCommand(child, append(slices.Clone(path), child.Name()), lookup)
		entry.children[child.Name()] = childEntry
		for _, alias := range child.Aliases {
			entry.children[alias] = childEntry
		}
	}
	return entry
}

// Names returns a sorted copy of the available top-level command names and aliases.
func (c *CommandCatalog) Names() []string {
	defer perf.Track(nil, "flags.CommandCatalog.Names")()

	if c == nil {
		return nil
	}
	names := slices.Collect(maps.Keys(c.commands))
	slices.Sort(names)
	return names
}

// FlagName resolves a bare flag key using registered native and compatibility
// flags. Explicit dash prefixes and unknown flags are passed through to the CLI.
func (c *CommandCatalog) FlagName(argv []string, key string) string {
	defer perf.Track(nil, "flags.CommandCatalog.FlagName")()

	if strings.HasPrefix(key, flagPrefix) {
		return key
	}
	if c == nil || len(argv) == 0 {
		return longFlagPrefix + key
	}
	entry := c.commands[argv[0]]
	if entry == nil {
		return longFlagPrefix + key
	}
	entry = entry.resolve(argv[1:])
	if name := entry.flags[key]; name != "" {
		return name
	}
	return longFlagPrefix + key
}

// resolve follows subcommands while skipping registered flags and their values.
// Unknown tokens end discovery; the actual command parser remains authoritative.
func (entry *commandEntry) resolve(args []string) *commandEntry {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == endOfOptionsArg {
			break
		}
		if strings.HasPrefix(arg, flagPrefix) {
			name, _, hasValue := strings.Cut(arg, flagAssignmentSeparator)
			takesValue, known := entry.takesValue[name]
			if !known {
				break
			}
			if takesValue && !hasValue {
				i++
			}
			continue
		}
		child := entry.children[arg]
		if child == nil {
			break
		}
		entry = child
	}
	return entry
}
