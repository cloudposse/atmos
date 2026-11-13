// Package provideroptions declares, registers, and resolves the typed options a configuration
// provider adds to `atmos terraform` commands.
//
// A provider declares its options as static package metadata, so the host can register flags and
// render help without starting the plugin. Values follow the repo's flag precedence (command line,
// then environment); the provider's own configuration layer sits below both and is read by the
// provider. Only options the user set explicitly are reported, so a provider can tell an absent
// option from an explicit false, zero, or empty value.
package provideroptions

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Option value types.
const (
	TypeString = "string"
	TypeBool   = "bool"
	TypeInt    = "int"
	TypeList   = "list"
	TypeMap    = "map"
)

// Modes a configuration provider section can run in.
const (
	ModeNative     = "native"
	ModeExecutable = "executable"
)

// Option is one statically declared provider option.
type Option struct {
	// Name is the key under which the value reaches the provider, in snake_case.
	Name string
	// Flag is the long flag name without dashes. It defaults to `<provider>-<name with dashes>`.
	Flag string
	// Aliases are additional hidden flag names for the same option.
	Aliases []string
	Type    string
	// Default is documented in help. The provider applies it when the option is absent; the host never sends it.
	Default any
	// Config is the key under the provider's configuration section that supplies the lowest-precedence value.
	Config string
	// Env lists environment variables in addition to ATMOS_<PROVIDER>_<NAME>.
	Env []string
	// Modes lists the section modes the option applies to. Empty means every mode.
	Modes []string
	// Commands lists the `atmos terraform` subcommands that accept the option.
	Commands []string
	// Sensitive values are masked, hashed into saved-plan identities, and never logged or written to configuration.
	Sensitive bool
	// Deprecated is the message shown when the option is used. Empty means the option is current.
	Deprecated  string
	Description string
}

// Provider groups the options of one enabled configuration section.
type Provider struct {
	// Section is the key under `components.terraform` that enabled the provider.
	Section string
	// Name is the provider name; it prefixes flag and environment variable names.
	Name string
	Mode string
	// Options are the declared options. Registration filters them by mode.
	Options []Option
}

var (
	namePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	flagPattern = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)
	envPattern  = regexp.MustCompile(`^[A-Z][A-Z0-9_]*$`)
	nonWord     = regexp.MustCompile(`[^a-z0-9]+`)
)

var optionKeys = []string{"name", "flag", "aliases", "type", "default", "config", "env", "modes", "commands", "sensitive", "deprecated", "description"}

// FlagName returns the primary flag name of an option for the provider.
//
//nolint:lintroller // Pure name derivation; no I/O.
func (o *Option) FlagName(provider string) string {
	if o.Flag != "" {
		return o.Flag
	}
	return slug(provider, "-") + "-" + strings.ReplaceAll(o.Name, "_", "-")
}

// EnvNames returns the environment variables that supply the option, Atmos-prefixed name first.
//
//nolint:lintroller // Pure name derivation; no I/O.
func (o *Option) EnvNames(provider string) []string {
	primary := "ATMOS_" + strings.ToUpper(slug(provider, "_")) + "_" + strings.ToUpper(o.Name)
	return append([]string{primary}, o.Env...)
}

// slug makes a provider name safe for flag and environment variable names: `vendor/tool` becomes `vendor-tool`.
func slug(provider, separator string) string {
	return nonWord.ReplaceAllString(strings.ToLower(provider), separator)
}

// AppliesTo reports whether the option is valid in a section mode.
//
//nolint:lintroller // Pure lookup; no I/O.
func (o *Option) AppliesTo(mode string) bool {
	return len(o.Modes) == 0 || slices.Contains(o.Modes, mode)
}

// Parse validates the `options` metadata of a provider export. The value comes from the package
// manifest, so every problem names the offending option and how to repair it.
func Parse(provider string, raw any) ([]Option, error) {
	defer perf.Track(nil, "provideroptions.Parse")()

	if raw == nil {
		return nil, nil
	}
	entries, ok := raw.([]any)
	if !ok {
		return nil, declarationError("", "`options` must be a list of option declarations")
	}
	options := make([]Option, 0, len(entries))
	flagsSeen := make(map[string]string)
	for index, entry := range entries {
		fields, ok := entry.(map[string]any)
		if !ok {
			return nil, declarationError(fmt.Sprintf("#%d", index), "an option declaration must be a mapping")
		}
		option, err := parseOption(provider, fields)
		if err != nil {
			return nil, err
		}
		if err := recordFlags(provider, &option, flagsSeen); err != nil {
			return nil, err
		}
		if err := rejectOverlappingName(options, &option); err != nil {
			return nil, err
		}
		options = append(options, option)
	}
	return options, nil
}

func recordFlags(provider string, option *Option, seen map[string]string) error {
	for _, name := range append([]string{option.FlagName(provider)}, option.Aliases...) {
		if previous, exists := seen[name]; exists {
			return declarationError(option.Name, fmt.Sprintf("flag `--%s` is already used by option `%s`", name, previous))
		}
		seen[name] = option.Name
	}
	return nil
}

// rejectOverlappingName rejects a second declaration of an option name that can apply to the same command
// and mode: values reach the provider keyed by name, so only one of the two flags would take effect.
func rejectOverlappingName(declared []Option, option *Option) error {
	for index := range declared {
		previous := &declared[index]
		if previous.Name == option.Name && overlaps(previous.Commands, option.Commands) && overlaps(previous.Modes, option.Modes) {
			return declarationError(option.Name, "the option is declared more than once for the same command; give each declaration distinct `commands` or `modes`")
		}
	}
	return nil
}

// overlaps reports whether two scopes share an entry; an empty scope applies everywhere.
func overlaps(left, right []string) bool {
	if len(left) == 0 || len(right) == 0 {
		return true
	}
	for _, entry := range left {
		if slices.Contains(right, entry) {
			return true
		}
	}
	return false
}

func parseOption(provider string, fields map[string]any) (Option, error) {
	if err := validateOptionFields(fields); err != nil {
		return Option{}, err
	}
	option := Option{
		Name: stringField(fields, "name"), Flag: stringField(fields, "flag"), Type: stringField(fields, "type"),
		Config: stringField(fields, "config"), Deprecated: stringField(fields, "deprecated"), Description: stringField(fields, "description"),
	}
	var err error
	option.Sensitive, _ = fields["sensitive"].(bool)
	for target, key := range map[*[]string]string{&option.Aliases: "aliases", &option.Env: "env", &option.Modes: "modes", &option.Commands: "commands"} {
		if *target, err = stringList(fields[key]); err != nil {
			return Option{}, declarationError(option.Name, fmt.Sprintf("`%s` must be a list of strings", key))
		}
	}
	if option.Default, err = typedDefault(option.Type, fields["default"]); err != nil {
		return Option{}, declarationError(option.Name, err.Error())
	}
	if err := validateOption(provider, &option); err != nil {
		return Option{}, err
	}
	return option, nil
}

func validateOptionFields(fields map[string]any) error {
	name := stringField(fields, "name")
	for key := range fields {
		if !slices.Contains(optionKeys, key) {
			return declarationError(name, fmt.Sprintf("unsupported key `%s`; use one of %s", key, strings.Join(optionKeys, ", ")))
		}
	}
	for _, key := range []string{"flag", "config", "deprecated"} {
		if raw := fields[key]; raw != nil {
			if _, ok := raw.(string); !ok {
				return declarationError(name, fmt.Sprintf("`%s` must be a string, not %T", key, raw))
			}
		}
	}
	_, isBool := fields["sensitive"].(bool)
	if raw, present := fields["sensitive"]; present && !isBool {
		return declarationError(name, fmt.Sprintf("`sensitive` must be true or false, not %T", raw))
	}
	return nil
}

func validateOption(provider string, option *Option) error {
	if err := validateShape(option); err != nil {
		return err
	}
	for _, name := range append([]string{option.FlagName(provider)}, option.Aliases...) {
		if !flagPattern.MatchString(name) {
			return declarationError(option.Name, fmt.Sprintf("flag name %q must be lowercase words separated by dashes", name))
		}
	}
	for _, name := range option.Env {
		if !envPattern.MatchString(name) {
			return declarationError(option.Name, fmt.Sprintf("environment variable %q must be upper case with underscores", name))
		}
	}
	return validateModes(option)
}

func validateShape(option *Option) error {
	switch {
	case !namePattern.MatchString(option.Name):
		return declarationError(option.Name, "`name` must be snake_case, for example `iam_assume_role`")
	case !slices.Contains([]string{TypeString, TypeBool, TypeInt, TypeList, TypeMap}, option.Type):
		return declarationError(option.Name, "`type` must be one of string, bool, int, list, or map")
	case option.Description == "":
		return declarationError(option.Name, "`description` is required because it is the help text")
	case len(option.Commands) == 0:
		return declarationError(option.Name, "`commands` must list the terraform subcommands that accept the option")
	case option.Sensitive && (option.Config != "" || option.Default != nil):
		return declarationError(option.Name, "a sensitive option cannot have a `config` key or a `default`, because secrets must not be written to configuration")
	}
	return nil
}

func validateModes(option *Option) error {
	for _, mode := range option.Modes {
		if mode != ModeNative && mode != ModeExecutable {
			return declarationError(option.Name, fmt.Sprintf("mode %q is not one of native or executable", mode))
		}
	}
	return nil
}

func stringField(fields map[string]any, key string) string {
	value, _ := fields[key].(string)
	return value
}

func stringList(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, errUtils.ErrProviderOption
	}
	result := make([]string, 0, len(items))
	for _, item := range items {
		text, ok := item.(string)
		if !ok || text == "" {
			return nil, errUtils.ErrProviderOption
		}
		result = append(result, text)
	}
	return result, nil
}

// defaultCheckers accept a declared default of each option type.
var defaultCheckers = map[string]func(any) (any, bool){
	TypeString: func(raw any) (any, bool) { text, ok := raw.(string); return text, ok },
	TypeBool:   func(raw any) (any, bool) { flag, ok := raw.(bool); return flag, ok },
	TypeInt:    integer,
	TypeList: func(raw any) (any, bool) {
		items, err := stringList(raw)
		return items, err == nil
	},
}

// typedDefault checks that a declared default matches the option type.
func typedDefault(optionType string, raw any) (any, error) {
	if raw == nil {
		return nil, nil
	}
	if optionType == TypeMap {
		return nil, fmt.Errorf("%w: a map option cannot declare a default; set it in configuration instead", errUtils.ErrProviderOption)
	}
	if check := defaultCheckers[optionType]; check != nil {
		if value, ok := check(raw); ok {
			return value, nil
		}
	}
	return nil, fmt.Errorf("%w: `default` does not match type %s", errUtils.ErrProviderOption, optionType)
}

func integer(raw any) (any, bool) {
	switch number := raw.(type) {
	case int:
		return number, true
	case int64:
		return int(number), true
	case float64:
		whole := int(number)
		return whole, float64(whole) == number
	}
	return nil, false
}

func declarationError(option, problem string) error {
	label := "an option"
	if option != "" {
		label = "option `" + option + "`"
	}
	return errUtils.Build(errUtils.ErrProviderOption).
		WithExplanationf("The package metadata declares %s incorrectly: %s.", label, problem).
		WithHint("Fix the `options` list of the configuration export in the package's `plugin.yaml`.").
		Err()
}
