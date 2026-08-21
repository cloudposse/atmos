package schema

import (
	"fmt"
	"reflect"
	"sort"

	"github.com/go-viper/mapstructure/v2"
)

// Custom CLI commands.

// FindCommandByName searches commands (and their nested Commands subcommands, recursively)
// for a Command with the given top-level Name. Used to resolve dependencies.commands entries,
// which reference a command by its own name regardless of nesting depth, since
// atmosConfig.Commands is already a flat-merged list of every atmos.d-imported command
// definition by the time dependency resolution runs.
//
// Ambiguous reports whether more than one command in the tree shares name -- e.g. two unrelated
// parent commands each declaring a nested child named "build" -- in which case cmd is nil and
// found is false regardless of how many matches exist: callers MUST treat this as unresolvable,
// never silently pick "whichever the tree-walk found first," since that order is an
// implementation detail (declaration order across possibly-merged atmos.d imports), not a
// meaningful disambiguation rule a config author actually chose.
func FindCommandByName(commands []Command, name string) (cmd *Command, found bool, ambiguous bool) {
	matches := collectCommandsByName(commands, name)
	switch len(matches) {
	case 0:
		return nil, false, false
	case 1:
		return matches[0], true, false
	default:
		return nil, false, true
	}
}

// collectCommandsByName returns every Command in the tree (commands and their nested Commands
// subcommands, recursively) whose Name matches name.
func collectCommandsByName(commands []Command, name string) []*Command {
	var matches []*Command
	for i := range commands {
		if commands[i].Name == name {
			matches = append(matches, &commands[i])
		}
		matches = append(matches, collectCommandsByName(commands[i].Commands, name)...)
	}
	return matches
}

// Command defines a custom CLI command.
type Command struct {
	Name             string `yaml:"name" json:"name" mapstructure:"name"`
	Description      string `yaml:"description" json:"description" mapstructure:"description"`
	Default          string `yaml:"default,omitempty" json:"default,omitempty" mapstructure:"default"`
	WorkingDirectory string `yaml:"working_directory,omitempty" json:"working_directory,omitempty" mapstructure:"working_directory"`
	// Dependencies specifies external tool dependencies that must be installed before running this command.
	Dependencies    *Dependencies          `yaml:"dependencies,omitempty" json:"dependencies,omitempty" mapstructure:"dependencies"`
	Env             []CommandEnv           `yaml:"env" json:"env" mapstructure:"env"`
	Arguments       []CommandArgument      `yaml:"arguments" json:"arguments" mapstructure:"arguments"`
	Flags           []CommandFlag          `yaml:"flags" json:"flags" mapstructure:"flags"`
	Component       *CommandComponent      `yaml:"component,omitempty" json:"component,omitempty" mapstructure:"component"`
	ComponentConfig CommandComponentConfig `yaml:"component_config" json:"component_config" mapstructure:"component_config"`
	// Steps supports both simple string syntax and structured syntax.
	// Simple: ["echo hello", "echo world"]
	// Structured: [{name: step1, command: echo hello, timeout: 30s}]
	// Mixed: Both formats can be used in the same list.
	Steps    Tasks     `yaml:"steps" json:"steps" mapstructure:"steps"`
	Commands []Command `yaml:"commands" json:"commands" mapstructure:"commands"`
	Verbose  bool      `yaml:"verbose" json:"verbose" mapstructure:"verbose"`
	Identity string    `yaml:"identity,omitempty" json:"identity,omitempty" mapstructure:"identity"`
	// Aliases lists alternative names this command is also invocable under. Native, in-process
	// Cobra aliases (the same *cobra.Command registered under extra names) -- distinct from the
	// top-level `command_aliases:` map (CommandAliases), which redirects to a possibly-unrelated
	// command via a subprocess re-exec.
	Aliases []string `yaml:"aliases,omitempty" json:"aliases,omitempty" mapstructure:"aliases"`
	// Internal excludes the command from `atmos --help` / `atmos <group> --help` subcommand
	// listings, shell-completion suggestions, and the AI `atmos_list_commands` tool, while
	// leaving it fully runnable: `atmos <name> ...` still executes it directly, and
	// `atmos <name> --help` still renders its own help when invoked explicitly. Use for
	// helper commands meant to be called by other commands or run manually for debugging,
	// analogous to Just's `[private]` recipes or Task's `internal: true` tasks (maps to Cobra's
	// Command.Hidden).
	Internal bool `yaml:"internal,omitempty" json:"internal,omitempty" mapstructure:"internal"`
}

// CommandArgument defines a positional argument for a custom command.
type CommandArgument struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     string `yaml:"default" json:"default" mapstructure:"default"`
	// Type specifies the semantic type of this argument: "component" or "stack".
	// When set, the argument value is used to resolve component configuration.
	Type string `yaml:"type,omitempty" json:"type,omitempty" mapstructure:"type"`
	// Values restricts this argument to a fixed set of allowed strings, validated the same way
	// pkg/flags' built-in-command `valid_values:` already is (flags.ValidateValue). When
	// Required and missing in an interactive terminal, the user is prompted to pick one instead
	// of erroring, reusing pkg/flags/interactive.go's PromptForPositionalArg machinery.
	Values []string `yaml:"values,omitempty" json:"values,omitempty" mapstructure:"values"`
}

// CommandFlag defines a flag for a custom command.
type CommandFlag struct {
	Name        string `yaml:"name" json:"name" mapstructure:"name"`
	Shorthand   string `yaml:"shorthand" json:"shorthand" mapstructure:"shorthand"`
	Type        string `yaml:"type" json:"type" mapstructure:"type"`
	Description string `yaml:"description" json:"description" mapstructure:"description"`
	Usage       string `yaml:"usage" json:"usage" mapstructure:"usage"`
	Required    bool   `yaml:"required" json:"required" mapstructure:"required"`
	Default     any    `yaml:"default" json:"default" mapstructure:"default"`
	// SemanticType specifies the semantic type of this flag: "component" or "stack".
	// When set, the flag value is used to resolve component configuration.
	SemanticType string `yaml:"semantic_type,omitempty" json:"semantic_type,omitempty" mapstructure:"semantic_type"`
	// Values restricts this flag to a fixed set of allowed strings, validated the same way
	// pkg/flags' built-in-command `valid_values:` already is (flags.ValidateValue). When
	// Required and missing in an interactive terminal, the user is prompted to pick one instead
	// of erroring, reusing pkg/flags/interactive.go's PromptForMissingRequired machinery.
	Values []string `yaml:"values,omitempty" json:"values,omitempty" mapstructure:"values"`
}

// CommandEnv defines an environment variable for a custom command.
type CommandEnv struct {
	Key          string `yaml:"key" json:"key" mapstructure:"key"`
	Value        string `yaml:"value" json:"value" mapstructure:"value"`
	ValueCommand string `yaml:"valueCommand" json:"valueCommand" mapstructure:"valueCommand"`
}

const commandEnvDecodeFailedMessage = "failed to decode command env"

// ErrCommandEnvDecodeFailed is returned when command env map decoding fails.
var ErrCommandEnvDecodeFailed error = commandEnvDecodeError{}

type commandEnvDecodeError struct{}

func (commandEnvDecodeError) Error() string {
	return commandEnvDecodeFailedMessage
}

func (commandEnvDecodeError) Is(target error) bool {
	return target != nil && target.Error() == commandEnvDecodeFailedMessage
}

// CommandEnvDecodeHook lets command-level env accept both the legacy list form:
//
//	env:
//	  - key: AWS_PROFILE
//	    value: dev
//
// and the map form used by workflow steps:
//
//	env:
//	  AWS_PROFILE: dev
func CommandEnvDecodeHook() mapstructure.DecodeHookFunc {
	return func(f reflect.Type, t reflect.Type, data any) (any, error) {
		if t != reflect.TypeOf([]CommandEnv{}) {
			return data, nil
		}
		if f.Kind() != reflect.Map {
			return data, nil
		}
		envMap, ok := data.(map[string]any)
		if !ok {
			return data, nil
		}
		return decodeCommandEnvMap(envMap)
	}
}

func decodeCommandEnvMap(envMap map[string]any) ([]CommandEnv, error) {
	keys := make([]string, 0, len(envMap))
	for key := range envMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	env := make([]CommandEnv, 0, len(keys))
	for _, key := range keys {
		item, err := decodeCommandEnvMapValue(key, envMap[key])
		if err != nil {
			return nil, err
		}
		env = append(env, item)
	}
	return env, nil
}

func decodeCommandEnvMapValue(key string, value any) (CommandEnv, error) {
	switch v := value.(type) {
	case string:
		return CommandEnv{Key: key, Value: v}, nil
	case map[string]any:
		item := CommandEnv{Key: key}
		decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
			Result:           &item,
			TagName:          "mapstructure",
			WeaklyTypedInput: true,
		})
		if err != nil {
			return CommandEnv{}, fmt.Errorf("%w for %q: %w", ErrCommandEnvDecodeFailed, key, err)
		}
		if err := decoder.Decode(v); err != nil {
			return CommandEnv{}, fmt.Errorf("%w for %q: %w", ErrCommandEnvDecodeFailed, key, err)
		}
		item.Key = key
		return item, nil
	default:
		return CommandEnv{}, fmt.Errorf("%w for command env %q: got %T (expected string or map)", ErrTaskUnexpectedNodeKind, key, value)
	}
}

// CommandComponent defines a custom component type for a command.
// When specified, the command can access component configuration via {{ .Component.* }} templates.
type CommandComponent struct {
	// Type is the component type name (e.g., "script", "ansible", "manifest").
	Type string `yaml:"type" json:"type" mapstructure:"type"`
	// BasePath is the base directory for components of this type.
	// Defaults to "components/<type>" if not specified.
	BasePath string `yaml:"base_path,omitempty" json:"base_path,omitempty" mapstructure:"base_path"`
}

// CommandComponentConfig defines component configuration for a custom command (legacy).
type CommandComponentConfig struct {
	Component string `yaml:"component" json:"component" mapstructure:"component"`
	Stack     string `yaml:"stack" json:"stack" mapstructure:"stack"`
}

// CLI command aliases

type CommandAliases map[string]string
