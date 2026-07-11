package client

import (
	_ "embed"
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/mcp/mcpcmd"
	errUtils "github.com/cloudposse/atmos/errors"
	term "github.com/cloudposse/atmos/internal/tui/templates/term"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	mcpconfig "github.com/cloudposse/atmos/pkg/mcp/config"
	mcpinstall "github.com/cloudposse/atmos/pkg/mcp/install"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/telemetry"
	"github.com/cloudposse/atmos/pkg/ui"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

//go:embed markdown/atmos_mcp_add.md
var addLongMarkdown string

var addCmd = &cobra.Command{
	Use:   "add [preset-name|url|command] [flags]",
	Short: "Add an MCP server to mcp.servers in atmos.yaml",
	Long:  addLongMarkdown,
	Args:  cobra.MaximumNArgs(1),
	RunE:  executeMCPAdd,
}

var addParser *flags.StandardParser

func init() {
	addParser = flags.NewStandardParser(
		flags.WithStringFlag("name", "n", "", "Server name (auto-inferred if not provided)"),
		flags.WithStringFlag("transport", "t", "", "Transport for a URL target: http (default: inferred)"),
		flags.WithValidValues("transport", schema.MCPTransportHTTP, schema.MCPTransportStdio),
		flags.WithStringSliceFlag("env", "", nil, "Environment variable for a stdio server (repeatable, KEY=VALUE)"),
		flags.WithStringSliceFlag("header", "H", nil, `HTTP header for a remote server (repeatable, "Key: Value")`),
		flags.WithStringFlag("description", "", "", "Human-readable description"),
		flags.WithStringFlag("identity", "", "", "Atmos Auth identity for credential injection"),
		flags.WithStringFlag("timeout", "", "", "Connection timeout (e.g. 30s)"),
		flags.WithBoolFlag("auto-start", "", false, "Start the server automatically when Atmos starts"),
		flags.WithBoolFlag("install", "", false, "Also install into detected AI clients immediately after adding"),
		flags.WithBoolFlag(yesFlag, "y", false, "Skip confirmation prompts"),
		flags.WithEnvVars(yesFlag, "ATMOS_YES"),
		flags.WithBoolFlag("force", "", false, "Overwrite an existing entry without prompting"),
	)
	addParser.RegisterFlags(addCmd)
	if err := addParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
	mcpcmd.McpCmd.AddCommand(addCmd)
}

func executeMCPAdd(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "cmd.mcpAdd")()
	v := viper.GetViper()
	if err := addParser.BindFlagsToViper(cmd, v); err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return err
	}

	inputs, err := resolveAddInputs(cmd, args, v, &atmosConfig)
	if err != nil {
		return err
	}

	yes := v.GetBool(yesFlag)
	force := v.GetBool("force")

	proceed, err := confirmAddOverwrite(inputs.File, inputs.Name, yes, force)
	if err != nil {
		return err
	}
	if !proceed {
		ui.Warningf("Skipped `%s` — already configured in %s", inputs.Name, inputs.File)
		return nil
	}

	if err := ensureMCPEnabledForPreset(cmd, &atmosConfig, inputs.Target, yes); err != nil {
		return err
	}

	if err := mcpconfig.Write(inputs.File, inputs.Name, inputs.ServerCfg); err != nil {
		return err
	}
	ui.Successf("Added `%s` to `mcp.servers` in %s", inputs.Name, inputs.File)

	if v.GetBool("install") {
		return installAfterAdd(&atmosConfig, inputs.Name, &inputs.ServerCfg, yes, force)
	}
	ui.Infof("Run `atmos mcp install %s` to push it to your AI client, or pass --install next time to do both at once.", inputs.Name)
	return nil
}

// addInputs bundles the parsed positional target, resolved server name and
// config, and the atmos.yaml file to write -- returned as a single struct
// (rather than five return values) per Atmos's function-result-limit
// convention.
type addInputs struct {
	Target    string
	Name      string
	File      string
	ServerCfg schema.MCPServerConfig
}

// resolveAddInputs parses the positional target and flags into a server
// name+config and resolves the atmos.yaml file to write, collapsing the
// up-front parsing steps into one call so executeMCPAdd reads as a flat
// pipeline. The returned Target (not just Name/ServerCfg) is needed by
// ensureMCPEnabledForPreset to look up the preset again.
func resolveAddInputs(
	cmd *cobra.Command, args []string, v *viper.Viper, atmosConfig *schema.AtmosConfiguration,
) (addInputs, error) {
	target, defaulted := resolveAddTarget(args)
	if defaulted {
		ui.Info("No target specified — adding the built-in `self` preset. Run `atmos mcp add --help` to see other options.")
	}

	name, serverCfg, err := mcpconfig.ParseServerSpec(atmosConfig, mcpconfig.ServerSpec{
		Target:      target,
		Name:        v.GetString("name"),
		Transport:   v.GetString("transport"),
		Description: v.GetString("description"),
		Identity:    v.GetString("identity"),
		Timeout:     v.GetString("timeout"),
		Env:         v.GetStringSlice("env"),
		Headers:     v.GetStringSlice("header"),
		AutoStart:   v.GetBool("auto-start"),
	})
	if err != nil {
		return addInputs{}, err
	}

	file, err := mcpconfig.ResolveFile(cmd, atmosConfig)
	if err != nil {
		return addInputs{}, err
	}
	return addInputs{Target: target, Name: name, File: file, ServerCfg: serverCfg}, nil
}

// resolveAddTarget returns the positional target argument, defaulting to the
// built-in self preset when none was given (`atmos mcp add` == `atmos mcp add
// self`). The second return value reports whether the default was applied,
// so the caller can print a one-line note -- collapsing "no target" down to
// self shouldn't feel silently magical.
func resolveAddTarget(args []string) (target string, defaulted bool) {
	if len(args) > 0 {
		return args[0], false
	}
	return mcpconfig.PresetSelf, true
}

// confirmAddOverwrite reports whether it's OK to write name to file --
// prompting first if the entry already exists, unless --force/--yes bypass
// the prompt. Mirrors installCmd's mcpConflictHandler; declining leaves the
// entry untouched and is not an error, matching Install's SkippedServers
// handling for the same situation.
func confirmAddOverwrite(file, name string, yes, force bool) (bool, error) {
	if force || yes {
		return true, nil
	}
	exists, err := mcpconfig.Exists(file, name)
	if err != nil {
		return false, err
	}
	if !exists {
		return true, nil
	}
	return flags.PromptForConfirmation(
		fmt.Sprintf("Overwrite existing MCP server %q in %s?", name, file),
		false,
	)
}

// ensureMCPEnabledForPreset checks (and, interactively, offers to fix)
// mcp.enabled before writing a preset that requires Atmos to be able to run
// as an MCP server itself -- otherwise the entry would fail at runtime the
// moment a client tries to launch `atmos mcp start`.
func ensureMCPEnabledForPreset(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, target string, yes bool) error {
	preset, ok := mcpconfig.ResolvePreset(target)
	if !ok || !preset.RequiresMCPEnabled || atmosConfig.MCP.Enabled {
		return nil
	}

	notEnabledErr := errUtils.Build(errUtils.ErrMCPNotEnabled).
		WithHint("Run `atmos config set mcp.enabled true`, or add to atmos.yaml:\n\n```yaml\nmcp:\n  enabled: true\n```").
		Err()

	if yes || !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		return notEnabledErr
	}
	return enableMCPInteractively(cmd, atmosConfig, notEnabledErr)
}

// enableMCPInteractively prompts to flip mcp.enabled on and writes it to
// atmos.yaml on confirm. Returns notEnabledErr unchanged if the user declines.
func enableMCPInteractively(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, notEnabledErr error) error {
	confirmed, err := flags.PromptForConfirmation("mcp.enabled is false in atmos.yaml. Enable it now?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return notEnabledErr
	}

	file, err := mcpconfig.ResolveFile(cmd, atmosConfig)
	if err != nil {
		return err
	}
	if err := atmosyaml.SetFileWithType(file, "mcp.enabled", "true", atmosyaml.TypeBool); err != nil {
		return err
	}
	atmosConfig.MCP.Enabled = true
	ui.Successf("Enabled `mcp.enabled` in %s", file)
	return nil
}

// installAfterAdd pushes the newly added server to detected AI clients
// immediately (--install). The add/remove and install/uninstall command
// pairs each own a separate flag surface, so add has no
// --client/--scope/--all-clients flags of its own and this deliberately
// doesn't reuse installCmd's own flag/viper state. If no client is detected,
// it skips with a hint rather than falling into an interactive
// client-selection prompt, since --install is meant to be a low-friction
// convenience cascade, not a wizard.
func installAfterAdd(atmosConfig *schema.AtmosConfiguration, name string, serverCfg *schema.MCPServerConfig, yes, force bool) error {
	basePath := installBasePath(atmosConfig)
	clients := mcpinstall.DetectClients(basePath, "", mcpinstall.ScopeProject)
	if len(clients) == 0 {
		ui.Warningf("No AI clients detected to install into — run `atmos mcp install %s --client <client>` to install manually.", name)
		return nil
	}
	return installServers(atmosConfig, map[string]schema.MCPServerConfig{name: *serverCfg}, installCommandOptions{
		clients: clients,
		scope:   mcpinstall.ScopeProject,
		yes:     yes,
		force:   force,
	})
}

// offerSelfInstall prompts, when running interactively, to add and install
// the built-in self preset when `atmos mcp install` (with no explicit server
// names) finds nothing configured at all -- turning a dead end into a
// one-keystroke path to using Atmos's own tools via MCP, instead of only
// printing a static hint. Returns false (no error) whenever the offer
// doesn't apply or is declined, so the caller falls through to that hint.
func offerSelfInstall(cmd *cobra.Command, atmosConfig *schema.AtmosConfiguration, yes bool) (bool, error) {
	if yes || !term.IsTTYSupportForStdin() || telemetry.IsCI() {
		return false, nil
	}
	confirmed, err := flags.PromptForConfirmation(
		"No MCP servers configured. Install Atmos's own MCP server (self) now?", false,
	)
	if err != nil || !confirmed {
		return false, err
	}

	// Call ParseServerSpec directly with a bare self-preset spec, rather than
	// going through resolveAddInputs/the global viper: this runs from inside
	// installCmd's RunE, so addCmd's flags were never bound to viper for this
	// invocation (only zero-valued at init()) -- reading them here would be
	// an implicit coincidence, not a real contract.
	name, serverCfg, err := mcpconfig.ParseServerSpec(atmosConfig, mcpconfig.ServerSpec{Target: mcpconfig.PresetSelf})
	if err != nil {
		return false, err
	}
	file, err := mcpconfig.ResolveFile(cmd, atmosConfig)
	if err != nil {
		return false, err
	}
	if err := ensureMCPEnabledForPreset(cmd, atmosConfig, mcpconfig.PresetSelf, yes); err != nil {
		return false, err
	}
	if err := mcpconfig.Write(file, name, serverCfg); err != nil {
		return false, err
	}
	ui.Successf("Added `%s` to `mcp.servers` in %s", name, file)

	return true, installAfterAdd(atmosConfig, name, &serverCfg, yes, false)
}
