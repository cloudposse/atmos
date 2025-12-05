package exec

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
	giturl "github.com/kubescape/go-git-url"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/auth"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

var ErrRepoPathConflict = errors.New("if the '--repo-path' flag is specified, the '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")

type DescribeAffectedExecCreator func(atmosConfig *schema.AtmosConfiguration) DescribeAffectedExec

type DescribeAffectedCmdArgs struct {
	CLIConfig                   *schema.AtmosConfiguration
	CloneTargetRef              bool
	Format                      string
	IncludeDependents           bool
	IncludeSettings             bool
	IncludeSpaceliftAdminStacks bool
	OutputFile                  string
	Ref                         string
	RepoPath                    string
	SHA                         string
	SSHKeyPath                  string
	SSHKeyPassword              string
	Verbose                     bool
	Upload                      bool
	Stack                       string
	Query                       string
	ProcessTemplates            bool
	ProcessYamlFunctions        bool
	Skip                        []string
	ExcludeLocked               bool
	AuthManager                 auth.AuthManager // Optional: Auth manager for credential management (from --identity flag).
}

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeAffectedExec interface {
	Execute(*DescribeAffectedCmdArgs) error
}

type describeAffectedExec struct {
	atmosConfig                               *schema.AtmosConfiguration
	executeDescribeAffectedWithTargetRepoPath func(
		atmosConfig *schema.AtmosConfiguration,
		targetRefPath string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		excludeLocked bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	executeDescribeAffectedWithTargetRefClone func(
		atmosConfig *schema.AtmosConfiguration,
		ref string,
		sha string,
		sshKeyPath string,
		sshKeyPassword string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		excludeLocked bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	executeDescribeAffectedWithTargetRefCheckout func(
		atmosConfig *schema.AtmosConfiguration,
		ref string,
		sha string,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		excludeLocked bool,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	addDependentsToAffected func(
		atmosConfig *schema.AtmosConfiguration,
		affected *[]schema.Affected,
		includeSettings bool,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
		onlyInStack string,
	) error
	printOrWriteToFile func(
		atmosConfig *schema.AtmosConfiguration,
		format string,
		file string,
		data any,
	) error
	IsTTYSupportForStdout func() bool
	pageCreator           pager.PageCreator
}

// NewDescribeAffectedExec creates a new `describe affected` executor.
func NewDescribeAffectedExec(
	atmosConfig *schema.AtmosConfiguration,
) DescribeAffectedExec {
	defer perf.Track(atmosConfig, "exec.NewDescribeAffectedExec")()

	return &describeAffectedExec{
		atmosConfig: atmosConfig,
		executeDescribeAffectedWithTargetRepoPath:    ExecuteDescribeAffectedWithTargetRepoPath,
		executeDescribeAffectedWithTargetRefClone:    ExecuteDescribeAffectedWithTargetRefClone,
		executeDescribeAffectedWithTargetRefCheckout: ExecuteDescribeAffectedWithTargetRefCheckout,
		addDependentsToAffected:                      addDependentsToAffected,
		printOrWriteToFile:                           printOrWriteToFile,
		IsTTYSupportForStdout:                        term.IsTTYSupportForStdout,
		pageCreator:                                  pager.New(),
	}
}

// ParseDescribeAffectedCliArgs parses the command-line arguments of the `atmos describe affected` command.
func ParseDescribeAffectedCliArgs(cmd *cobra.Command, args []string) (DescribeAffectedCmdArgs, error) {
	defer perf.Track(nil, "exec.ParseDescribeAffectedCliArgs")()

	var atmosConfig schema.AtmosConfiguration
	if info, err := ProcessCommandLineArgs("", cmd, args, nil); err != nil {
		return DescribeAffectedCmdArgs{}, err
	} else if atmosConfig, err = cfg.InitCliConfig(info, true); err != nil {
		return DescribeAffectedCmdArgs{}, err
	}
	if err := ValidateStacks(&atmosConfig); err != nil {
		return DescribeAffectedCmdArgs{}, err
	}
	// Process flags
	flags := cmd.Flags()

	result := DescribeAffectedCmdArgs{
		CLIConfig: &atmosConfig,
	}
	SetDescribeAffectedFlagValueInCliArgs(flags, &result)

	if result.Format != "yaml" && result.Format != "json" {
		return DescribeAffectedCmdArgs{}, ErrInvalidFormat
	}
	if result.RepoPath != "" && (result.Ref != "" || result.SHA != "" || result.SSHKeyPath != "" || result.SSHKeyPassword != "") {
		return DescribeAffectedCmdArgs{}, ErrRepoPathConflict
	}

	return result, nil
}

// SetDescribeAffectedFlagValueInCliArgs sets the flag values in CLI arguments.
func SetDescribeAffectedFlagValueInCliArgs(flags *pflag.FlagSet, describe *DescribeAffectedCmdArgs) {
	defer perf.Track(nil, "exec.SetDescribeAffectedFlagValueInCliArgs")()

	flagsKeyValue := map[string]any{
		"ref":                            &describe.Ref,
		"sha":                            &describe.SHA,
		"repo-path":                      &describe.RepoPath,
		"ssh-key":                        &describe.SSHKeyPath,
		"ssh-key-password":               &describe.SSHKeyPassword,
		"include-spacelift-admin-stacks": &describe.IncludeSpaceliftAdminStacks,
		"include-dependents":             &describe.IncludeDependents,
		"include-settings":               &describe.IncludeSettings,
		"upload":                         &describe.Upload,
		"clone-target-ref":               &describe.CloneTargetRef,
		"process-templates":              &describe.ProcessTemplates,
		"process-functions":              &describe.ProcessYamlFunctions,
		"skip":                           &describe.Skip,
		"pager":                          &describe.CLIConfig.Settings.Terminal.Pager,
		"stack":                          &describe.Stack,
		"format":                         &describe.Format,
		"file":                           &describe.OutputFile,
		"query":                          &describe.Query,
		"verbose":                        &describe.Verbose,
		"exclude-locked":                 &describe.ExcludeLocked,
	}

	// By default, process templates and YAML functions
	describe.ProcessTemplates = true
	describe.ProcessYamlFunctions = true

	var err error
	for k := range flagsKeyValue {
		if !flags.Changed(k) {
			continue
		}
		switch v := flagsKeyValue[k].(type) {
		case *string:
			*v, err = flags.GetString(k)
		case *bool:
			*v, err = flags.GetBool(k)
		case *[]string:
			*v, err = flags.GetStringSlice(k)
		default:
			er := fmt.Errorf("unsupported type %T for flag %s", v, k)
			errUtils.CheckErrorPrintAndExit(er, "", "")
		}
		errUtils.CheckErrorPrintAndExit(err, "", "")
	}
	// When uploading, always include dependents and settings for all affected components
	if describe.Upload {
		describe.IncludeDependents = true
		describe.IncludeSettings = true
	}
	if describe.Format == "" {
		describe.Format = "json"
	}
}

// Execute executes `describe affected` command.
func (d *describeAffectedExec) Execute(a *DescribeAffectedCmdArgs) error {
	defer perf.Track(nil, "exec.Execute")()

	var affected []schema.Affected
	var headHead, baseHead *plumbing.Reference
	var repoUrl string
	var err error

	switch {
	case a.RepoPath != "":
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRepoPath(
			a.CLIConfig,
			a.RepoPath,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.ExcludeLocked,
		)
	case a.CloneTargetRef:
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRefClone(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.SSHKeyPath,
			a.SSHKeyPassword,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.ExcludeLocked,
		)
	default:
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRefCheckout(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
			a.ExcludeLocked,
		)
	}
	if err != nil {
		return err
	}

	// Add dependent components and stacks for each affected component.
	if len(affected) > 0 && a.IncludeDependents {
		err = d.addDependentsToAffected(a.CLIConfig, &affected, a.IncludeSettings, a.ProcessTemplates, a.ProcessYamlFunctions, a.Skip, a.Stack)
		if err != nil {
			return err
		}
	}

	return d.view(a, repoUrl, headHead, baseHead, affected)
}

func (d *describeAffectedExec) view(a *DescribeAffectedCmdArgs, repoUrl string, headHead, baseHead *plumbing.Reference, affected []schema.Affected) error {
	if a.Query == "" {
		if err := d.uploadableQuery(a, repoUrl, headHead, baseHead, affected); err != nil {
			return err
		}
	} else {
		res, err := u.EvaluateYqExpression(d.atmosConfig, affected, a.Query)
		if err != nil {
			return err
		}

		err = viewWithScroll(&viewWithScrollProps{d.pageCreator, term.IsTTYSupportForStdout, d.printOrWriteToFile, d.atmosConfig, "Affected components and stacks", a.Format, a.OutputFile, res})
		if err != nil {
			return err
		}
	}
	return nil
}

func (d *describeAffectedExec) uploadableQuery(args *DescribeAffectedCmdArgs, repoUrl string, headHead, baseHead *plumbing.Reference, affected []schema.Affected) error {
	log.Debug("Affected components and stacks:")

	err := viewWithScroll(&viewWithScrollProps{d.pageCreator, d.IsTTYSupportForStdout, d.printOrWriteToFile, d.atmosConfig, "Affected components and stacks", args.Format, args.OutputFile, affected})
	if err != nil {
		return err
	}

	if !args.Upload {
		return nil
	}
	// Parse the repo URL
	gitURL, err := giturl.NewGitURL(repoUrl)
	if err != nil {
		return err
	}

	log.Debug("Creating API client")
	apiClient, err := pro.NewAtmosProAPIClientFromEnv(d.atmosConfig)
	if err != nil {
		return err
	}

	req := dtos.UploadAffectedStacksRequest{
		HeadSHA:   headHead.Hash().String(),
		BaseSHA:   baseHead.Hash().String(),
		RepoURL:   repoUrl,
		RepoName:  gitURL.GetRepoName(),
		RepoOwner: gitURL.GetOwnerName(),
		RepoHost:  gitURL.GetHostName(),
		Stacks:    affected,
	}

	log.Debug("Preparing upload affected stacks request", "req", req)

	return apiClient.UploadAffectedStacks(&req)
}

type viewWithScrollProps struct {
	pageCreator           pager.PageCreator
	isTTYSupportForStdout func() bool
	printOrWriteToFile    func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
	atmosConfig           *schema.AtmosConfiguration
	displayName           string
	format                string
	file                  string
	res                   any
}

func viewWithScroll(v *viewWithScrollProps) error {
	if v.atmosConfig.Settings.Terminal.IsPagerEnabled() && v.file == "" {
		err := viewConfig(&viewConfigProps{v.pageCreator, v.isTTYSupportForStdout, v.atmosConfig, v.displayName, v.format, v.res})
		{
			var errCase0 DescribeConfigFormatError
			var errCase1 error
			switch {
			case errors.As(err, &errCase0):
				return err
			case errors.As(err, &errCase1):
				return nil
			default:
				log.Debug("Failed to use pager")
			}
		}
	}

	err := v.printOrWriteToFile(v.atmosConfig, v.format, v.file, v.res)
	if err != nil {
		return err
	}
	return nil
}

type viewConfigProps struct {
	pageCreator           pager.PageCreator
	isTTYSupportForStdout func() bool
	atmosConfig           *schema.AtmosConfiguration
	displayName           string
	format                string
	data                  any
}

func viewConfig(v *viewConfigProps) error {
	if !v.isTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	var content string
	var err error
	switch v.format {
	case "yaml":
		content, err = u.GetHighlightedYAML(v.atmosConfig, v.data)
		if err != nil {
			return err
		}
	case "json":
		content, err = u.GetHighlightedJSON(v.atmosConfig, v.data)
		if err != nil {
			return err
		}
	default:
		return DescribeConfigFormatError{
			v.format,
		}
	}
	if err := v.pageCreator.Run(v.displayName, content); err != nil {
		return err
	}
	return nil
}
