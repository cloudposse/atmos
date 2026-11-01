package provideroptions

import (
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	atmosio "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// sectionAnnotation marks a flag as belonging to a provider, so help can group it.
const sectionAnnotation = "atmos_provider_section"

// registration records the flags one provider added to one command.
type registration struct {
	provider Provider
	options  []Option
	parser   *flags.StandardParser
}

var registered = struct {
	sync.Mutex
	byCommand map[*cobra.Command][]*registration
}{byCommand: make(map[*cobra.Command][]*registration)}

// Register adds the flags of every option to the subcommands that accept it. Registration needs no
// running plugin: it uses only the static declarations. A flag name that an Atmos flag or another
// provider already uses is rejected, so a provider can never shadow selection, scheduling, or UI flags.
func Register(root *cobra.Command, providers []Provider) error {
	defer perf.Track(nil, "provideroptions.Register")()

	for index := range providers {
		provider := &providers[index]
		commands := make(map[string][]Option)
		for i := range provider.Options {
			option := &provider.Options[i]
			if !option.AppliesTo(provider.Mode) {
				continue
			}
			for _, name := range option.Commands {
				commands[name] = append(commands[name], *option)
			}
		}
		for _, name := range slices.Sorted(maps.Keys(commands)) {
			sub := findSubcommand(root, name)
			if sub == nil {
				continue
			}
			if err := registerCommand(sub, provider, commands[name]); err != nil {
				return err
			}
		}
	}
	return nil
}

func findSubcommand(root *cobra.Command, name string) *cobra.Command {
	for _, sub := range root.Commands() {
		if sub.Name() == name {
			return sub
		}
	}
	return nil
}

func registerCommand(sub *cobra.Command, provider *Provider, options []Option) error {
	if alreadyRegistered(sub, provider) {
		return nil
	}
	var parserOptions []flags.Option
	for index := range options {
		option := &options[index]
		primary := option.FlagName(provider.Name)
		for _, name := range append([]string{primary}, option.Aliases...) {
			if err := checkCollision(sub, provider, option, name); err != nil {
				return err
			}
		}
		parserOptions = append(parserOptions, flagOptions(provider.Name, option, primary)...)
	}
	parser := flags.NewStandardParser(parserOptions...)
	parser.RegisterFlags(sub)
	if err := parser.BindToViper(viper.GetViper()); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrProviderOption, err)
	}
	for index := range options {
		decorate(sub, provider.Name, &options[index])
	}
	registered.Lock()
	registered.byCommand[sub] = append(registered.byCommand[sub], &registration{provider: *provider, options: options, parser: parser})
	registered.Unlock()
	return nil
}

func alreadyRegistered(sub *cobra.Command, provider *Provider) bool {
	registered.Lock()
	defer registered.Unlock()
	for _, existing := range registered.byCommand[sub] {
		if existing.provider.Name == provider.Name && existing.provider.Section == provider.Section {
			return true
		}
	}
	return false
}

func checkCollision(sub *cobra.Command, provider *Provider, option *Option, name string) error {
	if sub.Flags().Lookup(name) == nil && sub.InheritedFlags().Lookup(name) == nil && sub.PersistentFlags().Lookup(name) == nil {
		return nil
	}
	return errUtils.Build(errUtils.ErrProviderOptionCollision).
		WithExplanationf("The `%s` configuration provider declares option `%s` with flag `--%s`, but `atmos terraform %s` already has a flag with that name.", provider.Name, option.Name, name, sub.Name()).
		WithContext("provider", provider.Name).
		WithContext("option", option.Name).
		WithContext("command", sub.Name()).
		WithHint("Atmos owns stack selection, `--affected`, `--all`, `--ui`, and scheduling flags; a provider option cannot reuse them.").
		WithHint("Ask the package author to set a different `flag` for the option in `plugin.yaml`, or remove the conflicting package.").
		Err()
}

func flagOptions(provider string, option *Option, name string) []flags.Option {
	description := option.Description
	if option.Default != nil {
		description += fmt.Sprintf(" (default: %v)", option.Default)
	}
	var result []flags.Option
	switch option.Type {
	case TypeBool:
		value, _ := option.Default.(bool)
		result = append(result, flags.WithBoolFlag(name, "", value, description))
	case TypeInt:
		value, _ := option.Default.(int)
		result = append(result, flags.WithIntFlag(name, "", value, description))
	case TypeList:
		value, _ := option.Default.([]string)
		result = append(result, flags.WithStringArrayFlag(name, "", value, description))
	case TypeMap:
		result = append(result, flags.WithStringMapFlag(name, "", nil, description))
	default:
		value, _ := option.Default.(string)
		result = append(result, flags.WithStringFlag(name, "", value, description))
	}
	return append(result, flags.WithEnvVars(name, option.EnvNames(provider)...))
}

// decorate groups the flags under their provider in help and adds aliases and deprecation.
func decorate(sub *cobra.Command, provider string, option *Option) {
	primary := sub.Flags().Lookup(option.FlagName(provider))
	if primary == nil {
		return
	}
	mark(primary, provider)
	for _, alias := range option.Aliases {
		shared := &pflag.Flag{Name: alias, Usage: "Alias of --" + primary.Name, Value: primary.Value, DefValue: primary.DefValue, NoOptDefVal: primary.NoOptDefVal, Hidden: true}
		mark(shared, provider)
		sub.Flags().AddFlag(shared)
	}
	if option.Deprecated != "" {
		_ = sub.Flags().MarkDeprecated(primary.Name, option.Deprecated)
	}
}

func mark(flag *pflag.Flag, provider string) {
	if flag.Annotations == nil {
		flag.Annotations = make(map[string][]string)
	}
	flag.Annotations[sectionAnnotation] = []string{provider}
}

// Resolve reads the options the user set for the command, from the command line or the
// environment, and validates them against their declared types. Options the user did not set are
// absent from the result. Values of sensitive options are registered with the output masker and
// never appear in an error.
func Resolve(cmd *cobra.Command, v *viper.Viper) (map[string]map[string]schema.ComponentProviderOption, error) {
	defer perf.Track(nil, "provideroptions.Resolve")()

	registered.Lock()
	entries := slices.Clone(registered.byCommand[cmd])
	registered.Unlock()
	result := make(map[string]map[string]schema.ComponentProviderOption)
	for _, entry := range entries {
		if err := entry.parser.BindFlagsToViper(cmd, v); err != nil {
			return nil, fmt.Errorf("%w: %w", errUtils.ErrProviderOption, err)
		}
		for index := range entry.options {
			option := &entry.options[index]
			value, set, err := entry.read(cmd, v, option)
			if err != nil {
				return nil, err
			}
			if !set {
				continue
			}
			if result[entry.provider.Section] == nil {
				result[entry.provider.Section] = make(map[string]schema.ComponentProviderOption)
			}
			result[entry.provider.Section][option.Name] = schema.ComponentProviderOption{Value: value, Sensitive: option.Sensitive}
			if option.Sensitive {
				atmosio.RegisterSecretValue(value)
			}
		}
	}
	return result, nil
}

func (r *registration) read(cmd *cobra.Command, v *viper.Viper, option *Option) (any, bool, error) {
	primary := option.FlagName(r.provider.Name)
	if !r.explicit(cmd, option, primary) {
		return nil, false, nil
	}
	value, err := convert(option, primary, r.raw(cmd, v, option, primary))
	if err != nil {
		return nil, false, err
	}
	return value, true, nil
}

func (r *registration) explicit(cmd *cobra.Command, option *Option, primary string) bool {
	for _, name := range append([]string{primary}, option.Aliases...) {
		if flag := cmd.Flags().Lookup(name); flag != nil && flag.Changed {
			return true
		}
	}
	return r.parser.IsFlagExplicitlySet(cmd, primary)
}

// convert turns a flag or environment value into the declared type.
func convert(option *Option, flag string, raw any) (any, error) {
	switch option.Type {
	case TypeBool:
		parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(raw)))
		if err != nil {
			return nil, invalidValue(option, flag, "expected true or false")
		}
		return parsed, nil
	case TypeInt:
		parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(raw)))
		if err != nil {
			return nil, invalidValue(option, flag, "expected a whole number")
		}
		return parsed, nil
	case TypeList:
		return listValues(raw), nil
	case TypeMap:
		return mapValues(option, flag, raw)
	}
	return fmt.Sprint(raw), nil
}

// listValues reads a repeatable flag or a comma-separated environment value. An explicit empty
// value is an empty list, not an absent one.
func listValues(raw any) []string {
	switch typed := raw.(type) {
	case []string:
		return append([]string{}, typed...)
	case []any:
		items := make([]string, 0, len(typed))
		for _, item := range typed {
			items = append(items, fmt.Sprint(item))
		}
		return items
	case string:
		items := []string{}
		for _, item := range strings.Split(typed, ",") {
			if item = strings.TrimSpace(item); item != "" {
				items = append(items, item)
			}
		}
		return items
	}
	return []string{}
}

// mapValues parses name=value entries and rejects malformed ones instead of dropping them.
func mapValues(option *Option, flag string, raw any) (map[string]any, error) {
	result := make(map[string]any)
	for _, entry := range listValues(raw) {
		key, value, found := strings.Cut(entry, "=")
		if !found || strings.TrimSpace(key) == "" {
			return nil, invalidValue(option, flag, "each entry must look like name=value")
		}
		result[strings.TrimSpace(key)] = value
	}
	return result, nil
}

// invalidValue never echoes the value, which may be a secret.
func invalidValue(option *Option, flag, problem string) error {
	return errUtils.Build(errUtils.ErrProviderOption).
		WithExplanationf("The value for `--%s` (option `%s`) is invalid: %s.", flag, option.Name, problem).
		WithContext("option", option.Name).
		WithHintf("Check `--%s` or the environment variable that supplies it. Run the command with `--help` to see the accepted values.", flag).
		Err()
}

// Section is a titled group of provider flags, for help output.
type Section struct {
	Title string
	Flags *pflag.FlagSet
}

// SplitFlags separates provider flags from the rest, so help can show them under a provider heading.
func SplitFlags(set *pflag.FlagSet) (*pflag.FlagSet, []Section) {
	defer perf.Track(nil, "provideroptions.SplitFlags")()

	rest := pflag.NewFlagSet("flags", pflag.ContinueOnError)
	groups := make(map[string]*pflag.FlagSet)
	set.VisitAll(func(flag *pflag.Flag) {
		provider := flag.Annotations[sectionAnnotation]
		if len(provider) == 0 {
			rest.AddFlag(flag)
			return
		}
		if groups[provider[0]] == nil {
			groups[provider[0]] = pflag.NewFlagSet(provider[0], pflag.ContinueOnError)
		}
		groups[provider[0]].AddFlag(flag)
	})
	sections := make([]Section, 0, len(groups))
	for _, name := range slices.Sorted(maps.Keys(groups)) {
		sections = append(sections, Section{Title: strings.ToUpper(name) + " FLAGS", Flags: groups[name]})
	}
	return rest, sections
}

// raw returns the value of an alias the user typed, which Viper does not see because only the
// primary flag is bound, and otherwise the layered value of the primary flag.
func (r *registration) raw(cmd *cobra.Command, v *viper.Viper, option *Option, primary string) any {
	for _, name := range option.Aliases {
		flag := cmd.Flags().Lookup(name)
		if flag == nil || !flag.Changed {
			continue
		}
		if slice, ok := flag.Value.(pflag.SliceValue); ok {
			return slice.GetSlice()
		}
		return flag.Value.String()
	}
	return v.Get(primary)
}
