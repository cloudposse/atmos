package config

import (
	"fmt"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/ui"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// valueType holds the --type flag for `config set`.
var valueType string

var configGetCmd = &cobra.Command{
	Use:     "get <path>",
	Short:   "Read a value from atmos.yaml by dot-notation path",
	Long:    "Read a value from atmos.yaml using a dot-notation path (e.g. logs.level).",
	Example: "atmos config get logs.level",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.getRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}

		value, err := atmosyaml.GetFile(file, args[0])
		if err != nil {
			return errUtils.Build(err).
				WithHintf("Not found in `%s`. If it may come from a merged source (e.g. atmos.d/), check `atmos describe config -q .%s` instead.", atmosyaml.DisplayPath(file), args[0]).
				Err()
		}
		return data.Writeln(value)
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <path> <value>",
	Short: "Set a value in atmos.yaml by dot-notation path",
	Long: `Set a value in atmos.yaml using a dot-notation path, preserving comments,
anchors, YAML functions, and templates. By default (--type=auto) the value's
type is inferred: from the Atmos config schema when the path matches a known
field (e.g. mcp.enabled infers bool), otherwise from the type of the value
already at that path, otherwise from the new value's own shape (e.g. "5"
infers int, "true" infers bool), otherwise string. Pass --type explicitly to
override.`,
	Example: "atmos config set logs.level debug\natmos config set mcp.enabled true\natmos config set --type=yaml logs.exclude '[\"a\", \"b\"]'",
	Args:    cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.setRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}

		effectiveType, resolved, err := effectiveValueType(file, args[0], args[1])
		if err != nil {
			return err
		}
		if !resolved {
			warnIfSilentlyStoredAsString(args[1])
		}

		created, err := atmosyaml.SetFileWithType(file, args[0], args[1], effectiveType)
		if err != nil {
			return err
		}
		if created {
			ui.Successf("Created `%s` = `%s` in `%s`", args[0], args[1], atmosyaml.DisplayPath(file))
			return nil
		}
		ui.Successf("Updated `%s` to `%s` in `%s`", args[0], args[1], atmosyaml.DisplayPath(file))
		return nil
	},
}

// effectiveValueType resolves --type=auto (the default) to a concrete type:
// first from the Atmos config schema for dotPath (e.g. a known bool field),
// then from the type of the value already at dotPath in file (if any).
// Resolved is false only when neither source had an answer and TypeString was
// used as a bare default (as opposed to a genuinely inferred string) -- most
// commonly a free-form path like vars holding a value for the first time. An
// explicit (non-auto) --type is always returned unchanged, with resolved true.
//
// An existing value of TypeNull is deliberately not treated as a resolved
// inference: buildRHS's TypeNull case always writes the literal `null`,
// ignoring the value argument, which makes sense for an explicit
// `--type=null` but would silently discard the new value the caller is
// setting if auto-inference forced it here. So a null-typed existing value
// falls through to the same unresolved/string-fallback path as "nothing to
// infer from".
//
// Either inference source can also land on TypeYAML -- the schema modeling a
// map/slice field, or GetFileType finding an existing list/map -- which
// means "there's a typed answer, but it isn't a scalar auto can coerce a
// plain CLI string argument into." That returns a non-nil error instead of
// silently falling through to TypeString, which would otherwise replace the
// list/map with a plain string with no indication anything destructive
// happened.
//
// When neither source resolves anything, rawValue's own shape is tried last
// (atmosyaml.GuessScalarType) -- e.g. "true"/"5"/"3.14" are inferred as
// bool/int/float directly instead of falling back to string. Only a value
// that doesn't even look like a bool/int/float reaches the final
// TypeString/resolved=false fallback.
func effectiveValueType(file, dotPath, rawValue string) (valType string, resolved bool, err error) {
	if valueType != atmosyaml.TypeAuto {
		return valueType, true, nil
	}
	if inferred, ok := cfg.InferValueType(dotPath); ok {
		if inferred == atmosyaml.TypeYAML {
			return "", false, newNonScalarInferenceError(dotPath)
		}
		return inferred, true, nil
	}
	if inferred, ok := atmosyaml.GetFileType(file, dotPath); ok && inferred != atmosyaml.TypeNull {
		if inferred == atmosyaml.TypeYAML {
			return "", false, newNonScalarInferenceError(dotPath)
		}
		return inferred, true, nil
	}
	if guessed, ok := atmosyaml.GuessScalarType(rawValue); ok {
		return guessed, true, nil
	}
	return atmosyaml.TypeString, false, nil
}

// newNonScalarInferenceError builds the actionable error returned when
// --type=auto resolves to a path whose existing (or schema-modeled) value is
// a list or map.
func newNonScalarInferenceError(dotPath string) error {
	return errUtils.Build(fmt.Errorf("%w: %q", atmosyaml.ErrTypeInferenceNonScalar, dotPath)).
		WithHint("Pass --type=yaml with a full replacement literal (e.g. --type=yaml '[\"a\",\"b\"]'), " +
			"or --type=string to intentionally overwrite it with a string.").
		Err()
}

// warnIfSilentlyStoredAsString warns when --type=auto couldn't infer anything
// (effectiveValueType's resolved was false) and a value that looks like a
// bool or number is about to be written as a literal string anyway. Since
// GuessScalarType now handles the ordinary case (e.g. "true" on an
// unmodeled path infers TypeBool directly, never reaching here), this fires
// only for the one case GuessScalarType itself deliberately fails closed on:
// a bare "nan" literal, which looks numeric but can't be safely written as a
// float (see pkg/yaml.GuessNumericType) -- otherwise it would silently store
// the string "nan" with no indication a numeric interpretation was
// attempted and rejected.
func warnIfSilentlyStoredAsString(value string) {
	if !atmosyaml.LooksNonString(value) {
		return
	}
	ui.Warningf("%q looks like it could be a bool/int/float, but it's being stored as a literal string because the path isn't recognized by the Atmos config schema and has no existing typed value to infer from. Pass --type to store it as bool, int, float, or yaml.", value)
}

var configDeleteCmd = &cobra.Command{
	Use:     "delete <path>",
	Aliases: []string{"del", "unset"},
	Short:   "Delete a value from atmos.yaml by dot-notation path",
	Long:    "Delete a value from atmos.yaml using a dot-notation path, preserving the rest of the file.",
	Example: "atmos config delete components.terraform.append_user_agent",
	Args:    cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.deleteRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}

		existed, err := atmosyaml.DeleteFile(file, args[0])
		if err != nil {
			return err
		}
		if !existed {
			ui.Successf("Nothing to delete — `%s` is not set in `%s`", args[0], atmosyaml.DisplayPath(file))
			return nil
		}
		ui.Successf("Deleted `%s` from `%s`", args[0], atmosyaml.DisplayPath(file))
		return nil
	},
}

var configFormatCmd = &cobra.Command{
	Use:     "format",
	Aliases: []string{"fmt"},
	Short:   "Format the active atmos.yaml file",
	Long: `Format the active atmos.yaml file in place, preserving comments, anchors,
Atmos YAML functions, and Go templates.`,
	Example: "atmos config format\natmos --config ./config/atmos.yaml config format",
	Args:    cobra.NoArgs,
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "config.formatRunE")()

		file, err := resolveConfigFile(cmd)
		if err != nil {
			return err
		}
		if err := atmosyaml.FormatFile(file); err != nil {
			return err
		}
		ui.Successf("Formatted `%s`", atmosyaml.DisplayPath(file))
		return nil
	},
}

func init() {
	configSetCmd.Flags().StringVar(&valueType, "type", atmosyaml.TypeAuto,
		"Value type: auto, string, int, bool, float, null, or yaml (raw literal). "+
			"auto infers from the Atmos config schema, then from the existing value at the path, "+
			"then from the new value's own shape, falling back to string.")
}

// resolveConfigFile picks the atmos.yaml to edit. The inherited persistent
// --config flag (first entry) acts as an explicit override; otherwise the file
// is discovered in the current directory or git root. Unlike every other
// Atmos command, edits can only ever target one physical file, so a
// multi-value --config (which normally means "merge all of these") silently
// narrows to the first entry here -- worth a warning since it's a real
// behavior inconsistency, not just an unused extra flag value.
func resolveConfigFile(cmd *cobra.Command) (string, error) {
	override := ""
	if cfgFiles, _ := cmd.Flags().GetStringSlice("config"); len(cfgFiles) > 0 {
		override = cfgFiles[0]
		if len(cfgFiles) > 1 {
			ui.Warningf("Multiple --config files given; config edit commands only target the first one (%s) -- the rest are ignored.", cfgFiles[0])
		}
	}

	file, err := cfg.ResolveEditableConfigFile(atmosConfigPtr, override)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrInvalidArgumentError).
			WithExplanation(err.Error()).
			WithHint("Run from a directory containing atmos.yaml, or pass --config <file>.").
			Err()
	}
	return file, nil
}
