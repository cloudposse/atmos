package stack

import (
	"fmt"
	"sort"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/generator/setup"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/schema"
	stackcreate "github.com/cloudposse/atmos/pkg/stack/create"
	"github.com/cloudposse/atmos/pkg/ui"
)

func init() { stackCmd.AddCommand(newStackCreateCommand()) }

func newStackCreateCommand() *cobra.Command {
	parser := flags.NewStandardParser(
		flags.WithStringFlag("file", "", "", "New stack manifest path (required; never overwrites a file)"),
		flags.WithStringFlag("name", "", "", "Explicit stack name (otherwise preserve the project's naming convention)"),
		flags.WithStringFlag("template", "", "stack", "Scaffold name or source (from scaffold.templates or the built-in catalog)"),
		flags.WithStringFlag("ref", "", "", "Git revision for the scaffold source"),
		flags.WithStringSliceFlag("component", "", nil, "Add disabled component stubs as kind:name (repeatable)"),
		flags.WithStringSliceFlag("import", "", nil, "Add stack imports (repeatable)"),
		flags.WithStringMapFlag("set", "", nil, "Set scaffold answers as key=value (repeatable)"),
		flags.WithBoolFlag("interactive", "i", true, "Prompt for scaffold answers when connected to a terminal"),
		flags.WithBoolFlag("defaults", "", false, "Use defaults and --set values without prompting"),
		flags.WithStringFlag(flagSkipHooks, "", "", "Skip all scaffold hooks or a comma-separated list of hook names"),
		flags.WithNoOptDefVal(flagSkipHooks, "*"),
		flags.WithEnvVars(flagSkipHooks, "ATMOS_SKIP_HOOKS"),
		flags.WithDryRunFlag(),
	)
	cmd := &cobra.Command{Use: "create", Short: "Create a new stack from a scaffold", Args: cobra.NoArgs}
	parser.RegisterFlags(cmd)
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
	if v.GetString("file") == "" {
		return fmt.Errorf("%w: --file is required", errUtils.ErrInvalidFlag)
	}
	gen, err := setup.NewGeneratorContext()
	if err != nil {
		return err
	}
	values := make(map[string]any)
	for key, value := range flags.ParseStringMap(v, "set") {
		values[key] = value
	}
	opts := &stackcreate.Options{
		File: v.GetString("file"), Name: v.GetString("name"), Template: v.GetString("template"), Ref: v.GetString("ref"),
		Imports: v.GetStringSlice("import"), Components: v.GetStringSlice("component"), Values: values,
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
	ui.Successf("Created stack manifest %s", opts.File)
	return nil
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

func previewStackCreation(plan *stackcreate.Plan, skip func(string) bool) error {
	for _, file := range plan.Generation.Files {
		ui.Infof("Would create %s", file.Path)
		if err := data.Writeln(file.Content); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(plan.Hooks))
	for name := range plan.Hooks {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		hook := plan.Hooks[name]
		if err := previewCreationHook(name, &hook, skip); err != nil {
			return err
		}
	}
	return nil
}

func previewCreationHook(name string, hook *hooks.Hook, skip func(string) bool) error {
	if !hook.MatchesEvent(hooks.BeforeScaffoldGenerate) && !hook.MatchesEvent(hooks.AfterScaffoldGenerate) {
		return nil
	}
	if skip != nil && skip(name) {
		ui.Infof("Would skip scaffold hook %s (--skip-hooks)", name)
		return nil
	}
	ui.Infof("Scaffold hook %s would be evaluated at generation time", name)
	content, err := yaml.Marshal(hook)
	if err != nil {
		return err
	}
	if err := data.Writeln(string(content)); err != nil {
		return err
	}
	return nil
}
