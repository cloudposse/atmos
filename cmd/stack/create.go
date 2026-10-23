package stack

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/generator/setflag"
	"github.com/cloudposse/atmos/pkg/generator/setup"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	stackcreate "github.com/cloudposse/atmos/pkg/stack/create"
	"github.com/cloudposse/atmos/pkg/ui"
)

const flagSet = "set"

func init() { stackCmd.AddCommand(newStackCreateCommand()) }

func newStackCreateCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("file", "", "", "New stack manifest path (mutually exclusive with --target; never overwrites)"),
		flags.WithStringFlag("target", "", "", "Directory for generated stack files (mutually exclusive with --file; never overwrites)"),
		flags.WithStringFlag("name", "", "", "Explicit stack name (otherwise preserve the project's naming convention)"),
		flags.WithStringFlag("template", "", "stack", "Scaffold name or source (from scaffold.templates or the built-in catalog)"),
		flags.WithStringFlag("ref", "", "", "Git revision for a Git scaffold source"),
		// Repeatable flags are arrays, not slices, so a value such as catalog/{a,b} keeps its comma.
		flags.WithStringArrayFlag("component", "", nil, "Add disabled component stubs as kind:name (repeatable)"),
		flags.WithStringArrayFlag("import", "", nil, "Add stack imports (repeatable; commas and braces are kept as written)"),
		flags.WithStringMapFlag(flagSet, "", nil, "Set scaffold answers as key=value (repeatable)"),
		flags.WithBoolFlag("interactive", "i", true, "Prompt for scaffold answers when connected to a terminal"),
		flags.WithBoolFlag("defaults", "", false, "Use defaults and --set values without prompting"),
		flags.WithStringFlag(flagSkipHooks, "", "", "Skip all scaffold hooks or a comma-separated list of hook names"),
		flags.WithNoOptDefVal(flagSkipHooks, "*"),
		flags.WithEnvVars(flagSkipHooks, "ATMOS_SKIP_HOOKS"),
		flags.WithDryRunFlag(),
	)
	cmd := &cobra.Command{Use: "create", Short: "Create a new stack from a scaffold", Args: cobra.NoArgs}
	parser.RegisterFlags(cmd)
	cmd.MarkFlagsMutuallyExclusive("file", "target")
	cmd.RunE = func(cmd *cobra.Command, _ []string) error {
		v := viper.New()
		if err := parser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		return runStackCreate(cmd, v)
	}
	return cmd
}

func runStackCreate(cmd *cobra.Command, v *viper.Viper) error {
	config, err := stackCreationConfig()
	if err != nil {
		return err
	}
	file, target, err := creationDestination(cmd.Context(), v)
	if err != nil {
		return err
	}
	gen, err := setup.NewGeneratorContext()
	if err != nil {
		return err
	}
	values, err := scaffoldAnswers(cmd, v)
	if err != nil {
		return err
	}
	imports, components, err := repeatedFlags(cmd, v)
	if err != nil {
		return err
	}
	opts := &stackcreate.Options{
		File: file, Target: target, Name: v.GetString("name"), Template: v.GetString("template"), Ref: v.GetString("ref"),
		Imports: imports, Components: components, Values: values,
		Defaults: creationDefaults(v),
	}
	plan, err := stackcreate.Prepare(config, opts, gen.UI)
	if err != nil {
		return err
	}
	skip := hooks.NewSkipPredicate(v.GetString(flagSkipHooks))
	if v.GetBool("dry-run") {
		return previewStackCreation(plan, skip)
	}
	if err := plan.Apply(cmd.Context(), skip); err != nil {
		return err
	}
	reportCreated(plan)
	return nil
}

// repeatedFlags reads --import and --component verbatim; each occurrence is one value.
func repeatedFlags(cmd *cobra.Command, v *viper.Viper) (imports, components []string, err error) {
	if imports, err = flagValues(cmd, v, "import"); err != nil {
		return nil, nil, err
	}
	if components, err = flagValues(cmd, v, "component"); err != nil {
		return nil, nil, err
	}
	return imports, components, nil
}

// flagValues returns the raw occurrences of a repeatable flag, falling back to the bound value when unset on the command line.
func flagValues(cmd *cobra.Command, v *viper.Viper, name string) ([]string, error) {
	if cmd.Flags().Changed(name) {
		return cmd.Flags().GetStringArray(name)
	}
	return v.GetStringSlice(name), nil
}

// scaffoldAnswers collects --set values. A malformed entry is an error, as it is for scaffold generate,
// so an answer is never dropped without the user knowing.
func scaffoldAnswers(cmd *cobra.Command, v *viper.Viper) (map[string]any, error) {
	if cmd.Flags().Changed(flagSet) {
		raw, err := cmd.Flags().GetStringArray(flagSet)
		if err != nil {
			return nil, err
		}
		if err := setflag.Validate(raw); err != nil {
			return nil, err
		}
	}
	values := make(map[string]any)
	for key, value := range flags.ParseStringMap(v, flagSet) {
		values[key] = value
	}
	return values, nil
}

// reportCreated lists every file that was written, not only the manifest.
func reportCreated(plan *stackcreate.Plan) {
	for _, output := range plan.Outputs() {
		switch output.Kind {
		case stackcreate.OutputManifest:
			ui.Successf("Created stack manifest %s", output.Path)
		case stackcreate.OutputRecord:
			ui.Successf("Recorded the generation in %s", output.Path)
		default:
			ui.Successf("Created %s", output.Path)
		}
	}
}

func stackCreationConfig() (*schema.AtmosConfiguration, error) {
	if atmosConfigPtr != nil {
		return atmosConfigPtr, nil
	}
	loaded, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	return &loaded, err
}

func creationDefaults(v *viper.Viper) bool {
	return v.GetBool("defaults") || !v.GetBool("interactive") || !term.IsTTYSupportForStdout() || !term.IsTTYSupportForStdin()
}

// previewStackCreation prints the files and scaffold hooks a creation would produce.
func previewStackCreation(plan *stackcreate.Plan, skip func(string) bool) error {
	return plan.Preview(skip)
}
