package rain

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/component"
	raincomponent "github.com/cloudposse/atmos/pkg/component/rain"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var rainParser *flags.StandardParser

var rainCmd = &cobra.Command{
	Use:                "rain",
	Aliases:            []string{"rn"},
	Short:              "Manage AWS CloudFormation stacks with Rain",
	Long:               "Run Rain commands using Atmos stack configurations for AWS CloudFormation management.",
	FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
	RunE: func(cmd *cobra.Command, args []string) error {
		return cmd.Usage()
	},
}

var rainCommands = []string{
	"bootstrap", "build", "cat", "cc", "console", "deploy", "diff", "fmt",
	"forecast", "info", "logs", "ls", "merge", "module", "pkg", "rm",
	"stackset", "tree", "watch",
}

func init() {
	rainParser = flags.NewStandardParser(WithRainFlags())
	rainParser.RegisterPersistentFlags(rainCmd)
	if err := rainParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}

	internal.RegisterCommandFlagRegistry("rain", rainParser.Registry())

	for _, commandName := range rainCommands {
		rainCmd.AddCommand(newRainSubcommand(commandName))
	}

	internal.Register(&RainCommandProvider{})
}

type RainCommandProvider struct{}

func (p *RainCommandProvider) GetCommand() *cobra.Command {
	return rainCmd
}

func (p *RainCommandProvider) GetName() string {
	return "rain"
}

func (p *RainCommandProvider) GetGroup() string {
	return "Core Stack Commands"
}

func (p *RainCommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

func (p *RainCommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (p *RainCommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (p *RainCommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

func (p *RainCommandProvider) IsExperimental() bool {
	return false
}

func newRainSubcommand(commandName string) *cobra.Command {
	commandNameCopy := commandName
	return &cobra.Command{
		Use:                commandNameCopy + " [component]",
		Short:              fmt.Sprintf("Run rain %s", commandNameCopy),
		FParseErrWhitelist: struct{ UnknownFlags bool }{UnknownFlags: true},
		RunE: func(cmd *cobra.Command, args []string) error {
			return rainRun(rainCmd, cmd, commandNameCopy, args)
		},
	}
}

type rainRunOptions struct {
	ProcessTemplates  bool
	ProcessFunctions  bool
	Skip              []string
	DryRun            bool
	Query             string
	Components        []string
	All               bool
	Affected          bool
	RepoPath          string
	Ref               string
	SHA               string
	CloneTargetRef    bool
	SSHKey            string
	SSHKeyPassword    string
	IncludeDependents bool
}

func parseRainRunOptions(v *viper.Viper) *rainRunOptions {
	defer perf.Track(nil, "rain.parseRainRunOptions")()

	return &rainRunOptions{
		ProcessTemplates:  v.GetBool("process-templates"),
		ProcessFunctions:  v.GetBool("process-functions"),
		Skip:              v.GetStringSlice("skip"),
		DryRun:            v.GetBool("dry-run"),
		Query:             v.GetString("query"),
		Components:        v.GetStringSlice("components"),
		All:               v.GetBool("all"),
		Affected:          v.GetBool("affected"),
		RepoPath:          v.GetString("repo-path"),
		Ref:               v.GetString("ref"),
		SHA:               v.GetString("sha"),
		CloneTargetRef:    v.GetBool("clone-target-ref"),
		SSHKey:            v.GetString("ssh-key"),
		SSHKeyPassword:    v.GetString("ssh-key-password"),
		IncludeDependents: v.GetBool("include-dependents"),
	}
}

func rainRun(parentCmd, actualCmd *cobra.Command, subCommand string, args []string) error {
	if err := internal.ValidateAtmosConfig(); err != nil {
		return err
	}

	v := viper.GetViper()
	if err := rainParser.BindFlagsToViper(actualCmd, v); err != nil {
		return err
	}
	opts := parseRainRunOptions(v)

	argsWithSubCommand := append([]string{subCommand}, args...)
	info, err := e.ProcessCommandLineArgs(cfg.RainComponentType, parentCmd, argsWithSubCommand, nil)
	if err != nil {
		return err
	}
	applyOptionsToInfo(&info, opts)

	return dispatchRainRun(&info, opts, subCommand)
}

func dispatchRainRun(info *schema.ConfigAndStacksInfo, opts *rainRunOptions, subCommand string) error {
	switch {
	case info.Affected:
		return executeAffected(info, opts)
	case info.All || len(info.Components) > 0 || info.Query != "" || (info.Stack != "" && info.ComponentFromArg == ""):
		return raincomponent.ExecuteAll(info)
	case info.ComponentFromArg != "":
		return executeSingleComponent(info)
	case requiresComponent(subCommand):
		return fmt.Errorf("%w: rain %s requires a component argument unless --all, --affected, --components, --query, or --stack is used",
			errUtils.ErrMissingComponent, subCommand)
	default:
		return executeRawRain(info)
	}
}

func applyOptionsToInfo(info *schema.ConfigAndStacksInfo, opts *rainRunOptions) {
	info.ProcessTemplates = opts.ProcessTemplates
	info.ProcessFunctions = opts.ProcessFunctions
	info.Skip = opts.Skip
	info.Components = opts.Components
	info.DryRun = opts.DryRun
	info.All = opts.All
	info.Affected = opts.Affected
	info.Query = opts.Query
}

func executeSingleComponent(info *schema.ConfigAndStacksInfo) error {
	provider := component.MustGetProvider(cfg.RainComponentType)
	return provider.Execute(&component.ExecutionContext{
		ComponentType:       cfg.RainComponentType,
		Component:           info.ComponentFromArg,
		Stack:               info.Stack,
		Command:             cfg.RainComponentType,
		SubCommand:          info.SubCommand,
		ConfigAndStacksInfo: *info,
		Args:                info.AdditionalArgsAndFlags,
	})
}

func executeAffected(info *schema.ConfigAndStacksInfo, opts *rainRunOptions) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}
	args := &e.DescribeAffectedCmdArgs{
		CLIConfig:            &atmosConfig,
		CloneTargetRef:       opts.CloneTargetRef,
		IncludeDependents:    opts.IncludeDependents,
		IncludeSettings:      false,
		Ref:                  opts.Ref,
		RepoPath:             opts.RepoPath,
		SHA:                  opts.SHA,
		SSHKeyPath:           opts.SSHKey,
		SSHKeyPassword:       opts.SSHKeyPassword,
		Stack:                info.Stack,
		Query:                info.Query,
		ProcessTemplates:     info.ProcessTemplates,
		ProcessYamlFunctions: info.ProcessFunctions,
		Skip:                 info.Skip,
	}
	return raincomponent.ExecuteAffected(args, info)
}

func executeRawRain(info *schema.ConfigAndStacksInfo) error {
	atmosConfig, err := cfg.InitCliConfig(*info, true)
	if err != nil {
		return err
	}
	command := atmosConfig.Components.Rain.Command
	if command == "" {
		command = cfg.RainComponentType
	}
	return e.ExecuteShellCommand(
		atmosConfig,
		command,
		append([]string{info.SubCommand}, info.AdditionalArgsAndFlags...),
		"",
		nil,
		info.DryRun,
		info.RedirectStdErr,
	)
}

func requiresComponent(subCommand string) bool {
	switch subCommand {
	case "deploy", "rm", "logs", "watch", "cat", "console", "forecast", "diff", "pkg", "fmt", "tree":
		return true
	default:
		return false
	}
}
