package cmd

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	terminalenv "github.com/cloudposse/atmos/pkg/terminal/env"
)

type discoveryRequest struct {
	command         *cobra.Command
	docs            bool
	example         bool
	help            bool
	noColor         bool
	forceColor      bool
	interactive     *bool
	globals         []string
	positionals     []string
	discoveryValues []discoveryFlagValue
}

type discoveryFlagValue struct {
	name, value string
}

const discoveryDash = "-"

// Cheap lexical gate: ordinary commands never need the discovery parser.
// The parser still verifies arity, values, and the separator before dispatch.
func hasDiscoveryFlag(args []string) bool {
	for _, arg := range args {
		if arg == "--" {
			break
		}
		name, _, _ := strings.Cut(arg, "=")
		if name == "--docs" || name == "--example" {
			return true
		}
	}
	return false
}

// parseDiscoveryRequest reads flags without changing Cobra/Viper state or running
// validators. In particular, a flag value containing "--docs" is not a request.
func parseDiscoveryRequest(root *cobra.Command, args []string) (discoveryRequest, error) {
	request := discoveryRequest{command: root}
	var firstErr error
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if !strings.HasPrefix(arg, discoveryDash) || arg == discoveryDash {
			request.addPositional(root, arg)
			continue
		}
		consumed, err := request.readFlag(root, args[i:])
		if err != nil && firstErr == nil {
			firstErr = err
		}
		i += consumed - 1
	}
	if err := request.setDiscoveryFlags(root); firstErr == nil {
		firstErr = err
	}
	return request, firstErr
}

// Cobra parses flags on the selected command, even when they precede its name.
// Wait until that command is known before deciding who owns discovery flags.
func (request *discoveryRequest) setDiscoveryFlags(root *cobra.Command) error {
	var firstErr error
	for _, value := range request.discoveryValues {
		if discoveryFlag(request.command, value.name, false) != root.PersistentFlags().Lookup(value.name) {
			continue
		}
		if err := request.setFlag(value.name, value.value); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (request *discoveryRequest) addPositional(root *cobra.Command, arg string) {
	if arg == helpFlagName && request.command == root && len(request.positionals) == 0 {
		request.help = true
		return
	}
	if child := discoveryChild(request.command, arg); child != nil && len(request.positionals) == 0 {
		request.command = child
	} else {
		request.positionals = append(request.positionals, arg)
	}
}

func (request *discoveryRequest) readFlag(root *cobra.Command, args []string) (int, error) {
	flag, value, inline := request.flagToken(args[0])
	if flag == nil {
		if !request.command.FParseErrWhitelist.UnknownFlags && !request.command.DisableFlagParsing {
			return 1, fmt.Errorf("%w: unknown flag %s", errUtils.ErrInvalidFlag, args[0])
		}
		return 1, nil
	}
	consumed := 1
	if !inline {
		value = flag.NoOptDefVal
		if discoveryConsumesValue(request.command, flag, args[1:]) {
			if len(args) == 1 {
				return 1, fmt.Errorf("%w: flag needs an argument: %s", errUtils.ErrInvalidFlagValue, args[0])
			}
			consumed++
			value = args[1]
		}
	}
	var err error
	if flag.Name == "docs" || flag.Name == "example" {
		request.discoveryValues = append(request.discoveryValues, discoveryFlagValue{flag.Name, value})
	} else {
		err = request.setFlag(flag.Name, value)
	}
	if forwardDiscoveryGlobal(root, flag.Name) {
		request.globals = append(request.globals, args[:consumed]...)
	}
	return consumed, err
}

func forwardDiscoveryGlobal(root *cobra.Command, name string) bool {
	return root.PersistentFlags().Lookup(name) != nil && !slices.Contains([]string{"docs", "example", "chdir"}, name)
}

// A value-taking shorthand owns the remainder of a short-flag cluster.
func (request *discoveryRequest) flagToken(arg string) (*pflag.Flag, string, bool) {
	name, value, inline := strings.Cut(strings.TrimLeft(arg, discoveryDash), "=")
	if strings.HasPrefix(arg, "--") {
		return discoveryFlag(request.command, name, false), value, inline
	}
	var flag *pflag.Flag
	for j := 0; j < len(name); j++ {
		flag = discoveryFlag(request.command, name[j:j+1], true)
		if flag == nil {
			break
		}
		if flag.NoOptDefVal == "" {
			if j+1 < len(name) {
				value, inline = name[j+1:], true
			}
			break
		}
		if flag.Name == helpFlagName {
			request.help = true
		}
	}
	return flag, value, inline
}

func discoveryConsumesValue(cmd *cobra.Command, flag *pflag.Flag, rest []string) bool {
	if flag.NoOptDefVal == "" {
		return true
	}
	if len(rest) == 0 {
		return false
	}
	if flag.Value.Type() == "bool" {
		return terminalenv.IsBoolLiteral(rest[0])
	}
	registered := flags.GlobalFlagsRegistry().Get(flag.Name)
	return registered != nil && registered.GetNoOptDefValConsumesNextArg() &&
		!strings.HasPrefix(rest[0], discoveryDash) && discoveryChild(cmd, rest[0]) == nil
}

func discoveryChild(parent *cobra.Command, name string) *cobra.Command {
	for _, child := range parent.Commands() {
		if child.Name() == name || slices.Contains(child.Aliases, name) {
			return child
		}
	}
	return nil
}

func discoveryFlag(cmd *cobra.Command, name string, shorthand bool) *pflag.Flag {
	if (!shorthand && name == helpFlagName) || (shorthand && name == "h") {
		set := pflag.NewFlagSet(helpFlagName, pflag.ContinueOnError)
		set.BoolP(helpFlagName, "h", false, "")
		return set.Lookup(helpFlagName)
	}
	for current := cmd; current != nil; current = current.Parent() {
		sets := []*pflag.FlagSet{current.PersistentFlags()}
		if current == cmd {
			sets = []*pflag.FlagSet{current.Flags(), current.PersistentFlags()}
		}
		for _, set := range sets {
			var flag *pflag.Flag
			if shorthand {
				flag = set.ShorthandLookup(name)
			} else {
				flag = set.Lookup(name)
			}
			if flag != nil {
				return flag
			}
		}
	}
	return nil
}

func (request *discoveryRequest) setFlag(name, value string) error {
	targets := map[string]*bool{
		"docs":        &request.docs,
		"example":     &request.example,
		helpFlagName:  &request.help,
		"no-color":    &request.noColor,
		"force-color": &request.forceColor,
	}
	target := targets[name]
	if name == "interactive" {
		request.interactive = new(bool)
		target = request.interactive
	}
	if target == nil {
		return nil
	}
	if name == helpFlagName && !isBoolHelpValue(value) {
		request.help = true // The normal help path reports invalid help topics.
		return nil
	}
	enabled, err := strconv.ParseBool(value)
	if err != nil {
		// Invalid discovery values must fail before normal execution starts.
		request.docs = true
		return fmt.Errorf("invalid value %q for --%s: %w", value, name, err)
	}
	*target = enabled
	return nil
}
