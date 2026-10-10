package skill

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	ai "github.com/cloudposse/atmos/cmd/ai"
	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/ai/skills/source"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/config/homedir"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const (
	sourceAddOperation       = "add"
	sourceUninstallOperation = "uninstall"
	sourceSetOperation       = "set"
	sourceDryRunFlag         = "dry-run"
)

func init() {
	for _, operation := range []string{sourceAddOperation, sourceSetOperation, "remove"} {
		ai.SkillCmd.AddCommand(newSourceEditCommand(operation))
	}
	ai.SkillCmd.AddCommand(newSyncCommand())
}

func newSourceEditCommand(operation string) *cobra.Command {
	uses := map[string]string{sourceAddOperation: "add <source> --name <label>", sourceSetOperation: "set <label> <field> <value>", "remove": "remove <label>"}
	cmd := &cobra.Command{Use: uses[operation], Short: operation + " a skill source declaration (configuration only)", Args: cobra.ExactArgs(1)}
	if operation == sourceSetOperation {
		cmd.Args = cobra.ExactArgs(3)
	}
	options := []flags.Option{flags.WithBoolFlag(sourceDryRunFlag, "", false, "Preview without writing configuration")}
	if operation == sourceAddOperation {
		options = append(options, flags.WithStringFlag("name", "", "", "Source label"))
	}
	parser := flags.NewStandardParser(options...)
	parser.RegisterFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		v := viper.New()
		if err := parser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}
		files, _ := cmd.Flags().GetStringSlice("config")
		file, err := cfg.ResolveConfigOverride(files)
		if err != nil {
			return err
		}
		o := source.EditOptions{Operation: operation, Label: args[0], File: file, DryRun: v.GetBool(sourceDryRunFlag)}
		if operation == sourceAddOperation {
			o.Label = v.GetString("name")
			o.Value = args[0]
		}
		if operation == sourceSetOperation {
			o.Field = args[1]
			o.Value = args[2]
		}
		result, err := source.Edit(&config, o)
		if err != nil {
			return err
		}
		return data.Writeln(result)
	}
	return cmd
}

func sourceFlags() *flags.StandardParser {
	return flags.NewStandardParser(
		flags.WithStringFlag("source", "", "", "Select a declared source label"),
		flags.WithStringFlag("track", "", "", "Version track"),
		flags.WithBoolFlag(sourceDryRunFlag, "", false, "Preview without persistent changes"),
		flags.WithBoolFlag("frozen", "", false, "Require matching resolution locks"),
		flags.WithBoolFlag("check", "", false, "Check offline without writing"),
		flags.WithBoolFlag("prune", "", false, "Remove obsolete owned copies"),
	)
}

func newSyncCommand() *cobra.Command {
	cmd := &cobra.Command{Use: "sync [label]", Short: "Reconcile declared skill sources", Args: cobra.MaximumNArgs(1)}
	parser := sourceFlags()
	parser.RegisterFlags(cmd)
	common := flags.NewStandardParser(
		flags.WithStringFlag(scopeFlag, "", "", "Override declaration scope"),
		flags.WithEnvVars(scopeFlag, "ATMOS_AI_SKILL_SCOPE"),
		flags.WithStringSliceFlag(clientFlag, "c", nil, "Limit target clients"),
		flags.WithEnvVars(clientFlag, "ATMOS_AI_SKILL_CLIENT"),
		flags.WithBoolFlag("force", "", false, "Replace modified owned copies"),
		flags.WithEnvVars("force", "ATMOS_AI_SKILL_FORCE"),
		flags.WithBoolFlag("recover", "", false, "Recover an interrupted skill transaction"),
	)
	common.RegisterFlags(cmd)
	cmd.RunE = func(cmd *cobra.Command, args []string) error {
		v := viper.New()
		if err := parser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		if err := common.BindFlagsToViper(cmd, v); err != nil {
			return err
		}
		config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
		if err != nil {
			return err
		}
		engine, err := source.New(&config)
		if err != nil {
			return err
		}
		if v.GetBool("recover") {
			return recoverSources(engine, v)
		}
		opts := sourceOptions(cmd, v)
		if len(args) > 0 {
			if opts.Source != "" && opts.Source != args[0] {
				return source.ErrInvalid
			}
			opts.Source = args[0]
		}
		statuses, err := engine.Run(cmd.Context(), opts)
		printSourceOutcome(statuses, &opts, err)
		return err
	}
	return cmd
}

func sourceOptions(cmd *cobra.Command, v *viper.Viper) source.Options {
	o := source.Options{Path: v.GetString("path"), Source: v.GetString("source"), Track: v.GetString("track"), DryRun: v.GetBool(sourceDryRunFlag), Frozen: v.GetBool("frozen"), Check: v.GetBool("check"), Prune: v.GetBool("prune"), Force: v.GetBool("force")}
	if cmd.Flags().Changed(scopeFlag) || sourceEnvironment("ATMOS_AI_SKILL_SCOPE") {
		o.Scope = v.GetString(scopeFlag)
	} else if v.GetBool("global") {
		o.Scope = "user"
	}
	if cmd.Flags().Changed(clientFlag) || sourceEnvironment("ATMOS_AI_SKILL_CLIENT") {
		o.Clients = v.GetStringSlice(clientFlag)
	}
	if v.GetBool("all-clients") {
		o.Clients = marketplace.SupportedClients
	}
	if o.Path != "" {
		// Manual destinations do not retain ignored distribution overrides.
		o.Scope, o.Clients = "", nil
	}
	return o
}

func printSourceOutcome(statuses []source.Status, opts *source.Options, err error) {
	if err != nil && len(statuses) == 0 {
		return
	}
	if err == nil && !opts.DryRun && !opts.Check {
		for i := range statuses {
			switch statuses[i].Status {
			case "missing", "stale", "drifted":
				statuses[i].Status = "installed"
			case "remove":
				statuses[i].Status = "removed"
			}
		}
	}
	printSourceStatuses(statuses)
}

func printSourceStatuses(statuses []source.Status) {
	home, _ := homedir.Dir()
	changed := map[string]bool{}
	for _, s := range statuses {
		if s.Status == "current" {
			continue
		}
		path := s.Path
		if home != "" {
			path = strings.Replace(path, home, "~", 1)
		}
		if path != "" {
			// Preserve Windows separators when the status is rendered as Markdown.
			path = "`" + path + "`"
		}
		ui.Infof("%s: %s / %s [%s, %s, %s] %s", s.Status, s.Source, s.Name, s.Scope, s.Track, s.Client, path)
		changed[s.Source+"/"+s.Name] = true
	}
	if len(changed) == 0 {
		ui.Info("0 skills installed; already up to date")
	}
}

// sourceInvocation keeps CLI orchestration separate from reconciliation.
type sourceInvocation struct {
	cmd       *cobra.Command
	args      []string
	v         *viper.Viper
	engine    *source.Engine
	opts      source.Options
	confirmed bool
}

// runDeclared dispatches owned installations through the reconciliation engine.
func runDeclared(cmd *cobra.Command, args []string, v *viper.Viper) (bool, error) {
	parser := sourceFlags()
	if err := parser.BindFlagsToViper(cmd, v); err != nil {
		return true, err
	}
	opts := sourceOptions(cmd, v)
	if opts.Path != "" {
		warnIgnoredDistributionFlags(cmd)
	}
	config, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return true, err
	}
	engine, err := source.New(&config)
	if err != nil {
		return true, err
	}
	call := &sourceInvocation{cmd: cmd, args: args, v: v, engine: engine, opts: opts}
	if err = call.resolveScope(); err != nil {
		return true, err
	}
	if err = call.selectSource(); err != nil {
		return true, err
	}
	return call.dispatch()
}

func (call *sourceInvocation) dispatch() (bool, error) {
	cmd := call.cmd
	call.opts.Update = cmd.Name() == "update"
	call.opts.Uninstall = cmd.Name() == sourceUninstallOperation
	if call.opts.Source != "" {
		return true, call.runSelected()
	}
	switch cmd.Name() {
	case "install":
		return true, call.installAdHoc()
	case sourceUninstallOperation:
		return call.uninstallAll()
	default:
		return call.updateAll()
	}
}

func (c *sourceInvocation) selectSource() error {
	if len(c.args) == 0 || c.opts.Source != "" {
		return nil
	}
	name := c.args[0]
	if d := c.engine.Config.AI.Skills[name]; d != nil && d.Source != "" {
		if err := c.rejectAmbiguousLabel(name); err != nil {
			return err
		}
		c.opts.Source = name
		return nil
	}
	if c.cmd.Name() == "install" {
		return nil
	}
	owner, err := c.engine.ResolveInstalled(name, c.opts.Scope)
	c.opts.Source = owner
	c.opts.Name = name
	return err
}

func (c *sourceInvocation) runSelected() error {
	if len(c.args) > 0 && c.args[0] != c.opts.Source {
		c.opts.Name = c.args[0]
	}
	if strings.HasPrefix(c.opts.Source, "ad-hoc:") {
		config, err := c.engine.AdHocConfig(c.opts.Source)
		if err != nil {
			return err
		}
		c.engine.Config = config
		c.engine.AdHoc = true
	}
	return c.run()
}

func (c *sourceInvocation) run() error {
	if c.opts.Update && c.opts.Frozen {
		return fmt.Errorf("%w: frozen cannot update", source.ErrInvalid)
	}
	if err := c.confirm(); err != nil {
		return err
	}
	statuses, err := c.engine.Run(c.cmd.Context(), c.opts)
	printSourceOutcome(statuses, &c.opts, err)
	return err
}

// resolveScope keeps destructive defaults in the project and honors declared install scopes.
func (c *sourceInvocation) resolveScope() error {
	if c.opts.Scope != "" || c.opts.Path != "" || c.cmd.Name() == "update" {
		return nil
	}
	if c.hasDeclaredInstallScope() {
		return nil
	}
	skip := c.opts.Check || c.opts.DryRun || c.v.GetBool("yes")
	if c.cmd.Name() == sourceUninstallOperation {
		skip = skip || c.opts.Force
	}
	scope, err := resolveSkillScope(c.cmd, c.v, skip)
	c.opts.Scope = scope
	return err
}

func (c *sourceInvocation) hasDeclaredInstallScope() bool {
	label := c.opts.Source
	if label == "" && len(c.args) > 0 {
		label = c.args[0]
	}
	d := c.engine.Config.AI.Skills[label]
	return c.cmd.Name() == "install" && d != nil && d.Scope != ""
}

func (c *sourceInvocation) confirm() error {
	if c.confirmed || c.opts.DryRun || c.opts.Check {
		return nil
	}
	skip, title, cancelled := c.v.GetBool("yes"), "Install selected skills?", marketplace.ErrInstallationCancelled
	switch c.cmd.Name() {
	case sourceUninstallOperation:
		skip, title, cancelled = c.opts.Force, "Uninstall selected skills?", marketplace.ErrUninstallationCancelled
	case "update":
		title, cancelled = "Update selected skills?", marketplace.ErrUpdateCancelled
	}
	confirmed, err := flags.PromptForConfirmation(title, skip)
	if err != nil {
		return err
	}
	if !confirmed {
		return cancelled
	}
	c.confirmed = true
	return nil
}

func (c *sourceInvocation) installAdHoc() error {
	config := *c.engine.Config
	config.AI.Skills = map[string]*schema.AISkillConfig{}
	sources, err := adHocSources(c.args)
	if err != nil {
		return err
	}
	ui.Infof("Discovered %d skill sources", len(sources))
	for _, raw := range sources {
		declaration, err := adHocDeclaration(raw)
		if err != nil {
			return err
		}
		if c.opts.Scope != "" {
			declaration.Scope = c.opts.Scope
		}
		if c.opts.Clients != nil {
			declaration.Clients = c.opts.Clients
		}
		config.AI.Skills["ad-hoc:"+raw] = declaration
	}
	c.engine.Config = &config
	c.engine.AdHoc = true
	return c.run()
}

func adHocSources(args []string) ([]string, error) {
	if len(args) > 0 {
		return args, nil
	}
	catalog, err := marketplace.Catalog()
	if err != nil {
		return nil, err
	}
	sources := []string{}
	for _, entry := range catalog {
		sources = append(sources, entry.Name)
	}
	return sources, nil
}

func adHocDeclaration(raw string) (*schema.AISkillConfig, error) {
	ui.Info("Resolving skill source...")
	return source.ParseAdHoc(raw)
}

func (c *sourceInvocation) uninstallAll() (bool, error) {
	if len(c.args) > 0 {
		if c.opts.DryRun || c.opts.Check || c.opts.Frozen {
			return true, fmt.Errorf("%w: reinstall legacy skills to establish ownership before reconciliation", source.ErrInvalid)
		}
		return false, nil
	}
	if err := c.run(); err != nil {
		return true, err
	}
	c.engine.AdHoc = true
	if err := c.run(); err != nil {
		return true, err
	}
	ui.Success("Owned skills uninstalled successfully")
	return c.opts.DryRun || c.opts.Check || c.opts.Frozen, nil
}

//nolint:revive // Reconcile declarations and local ad-hoc records before the legacy compatibility adapter.
func (c *sourceInvocation) updateAll() (bool, error) {
	if len(c.args) > 0 {
		if c.opts.DryRun || c.opts.Check || c.opts.Frozen {
			return true, fmt.Errorf("%w: reinstall legacy skills to establish ownership before reconciliation", source.ErrInvalid)
		}
		return false, nil
	}
	if c.opts.Scope == "" {
		c.opts.Scope = marketplace.ScopeProject
	}
	installed, err := c.engine.InstalledConfig(c.opts.Scope)
	if err != nil {
		return true, err
	}
	c.engine.Config = installed
	if len(installed.AI.Skills) > 0 {
		if err = c.run(); err != nil {
			return true, err
		}
	}
	adHoc, err := c.engine.AdHocConfig("")
	if err != nil {
		return true, err
	}
	if err = c.filterInstalledScope(adHoc); err != nil {
		return true, err
	}
	c.engine.Config = adHoc
	c.engine.AdHoc = true
	if len(adHoc.AI.Skills) > 0 {
		if err = c.run(); err != nil {
			return true, err
		}
	}
	// Legacy bundled installations retain their existing update adapter.
	return c.opts.DryRun || c.opts.Check || c.opts.Frozen, nil
}

func recoverSources(engine *source.Engine, v *viper.Viper) error {
	if v.GetBool(sourceDryRunFlag) || v.GetBool("check") || v.GetBool("frozen") {
		return fmt.Errorf("%w: --recover cannot be combined with --dry-run, --check, or --frozen", source.ErrInvalid)
	}
	return engine.Recover()
}

func sourceEnvironment(name string) bool {
	env := viper.New()
	_ = env.BindEnv("value", name)
	return env.IsSet("value")
}

func (c *sourceInvocation) filterInstalledScope(config *schema.AtmosConfiguration) error {
	labels, err := c.engine.InstalledSources("", c.opts.Scope)
	if err != nil {
		return err
	}
	selected := map[string]bool{}
	for _, label := range labels {
		selected[label] = true
	}
	for label := range config.AI.Skills {
		if !selected[label] {
			delete(config.AI.Skills, label)
		}
	}
	return nil
}

func (c *sourceInvocation) rejectAmbiguousLabel(name string) error {
	owners, err := c.engine.InstalledSources(name, c.opts.Scope)
	if err != nil {
		return err
	}
	_, bundled := marketplace.LookupBundledSkill(name)
	local, statErr := os.Stat(filepath.Join(c.engine.Project, name))
	foreignOwner := false
	for _, owner := range owners {
		foreignOwner = foreignOwner || owner != name
	}
	if bundled || foreignOwner || (statErr == nil && local.IsDir()) {
		return fmt.Errorf("%w: ambiguous name %s; use --source", source.ErrInvalid, name)
	}
	return nil
}
