package exec

import (
	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	l "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/go-git/go-git/v5/plumbing"
	giturl "github.com/kubescape/go-git-url"

	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type DescribeAffectedCmdArgs struct {
	CLIConfig                   schema.AtmosConfiguration
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
}

type DescribeAffectedExec struct {
	atmosConfig                               *schema.AtmosConfiguration
	executeDescribeAffectedWithTargetRepoPath func(
		atmosConfig schema.AtmosConfiguration,
		targetRefPath string,
		verbose bool,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	executeDescribeAffectedWithTargetRefClone func(
		atmosConfig schema.AtmosConfiguration,
		ref string,
		sha string,
		sshKeyPath string,
		sshKeyPassword string,
		verbose bool,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	executeDescribeAffectedWithTargetRefCheckout func(
		atmosConfig schema.AtmosConfiguration,
		ref string,
		sha string,
		verbose bool,
		includeSpaceliftAdminStacks bool,
		includeSettings bool,
		stack string,
		processTemplates bool,
		processYamlFunctions bool,
		skip []string,
	) ([]schema.Affected, *plumbing.Reference, *plumbing.Reference, string, error)
	addDependentsToAffected func(
		atmosConfig schema.AtmosConfiguration,
		affected *[]schema.Affected,
		includeSettings bool,
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

func NewDescribeAffectedExec(
	atmosConfig *schema.AtmosConfiguration) *DescribeAffectedExec {
	return &DescribeAffectedExec{
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
func (d *DescribeAffectedExec) Execute(a DescribeAffectedCmdArgs) error {
	var affected []schema.Affected
	var headHead, baseHead *plumbing.Reference
	var repoUrl string
	var err error
	if a.RepoPath != "" {
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRepoPath(
			a.CLIConfig,
			a.RepoPath,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	} else if a.CloneTargetRef {
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRefClone(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.SSHKeyPath,
			a.SSHKeyPassword,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	} else {
		affected, headHead, baseHead, repoUrl, err = d.executeDescribeAffectedWithTargetRefCheckout(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	}

	if err != nil {
		return err
	}

	// Add dependent components and stacks for each affected component
	if len(affected) > 0 && a.IncludeDependents {
		err = addDependentsToAffected(a.CLIConfig, &affected, a.IncludeSettings)
		if err != nil {
			return err
		}
	}

	if a.Query == "" {
		log.Debug("\nAffected components and stacks: \n")
		err = viewWithScroll(d.pageCreator, d.IsTTYSupportForStdout, d.printOrWriteToFile, d.atmosConfig, "Affected components and stacks", a.Format, a.OutputFile, affected)
		if err != nil {
			return err
		}

		if a.Upload {
			// Parse the repo URL
			gitURL, err := giturl.NewGitURL(repoUrl)
			if err != nil {
				return err
			}
			logger, err := l.NewLoggerFromCliConfig(*d.atmosConfig)
			if err != nil {
				return err
			}
			apiClient, err := pro.NewAtmosProAPIClientFromEnv(logger)
			if err != nil {
				return err
			}

			req := pro.AffectedStacksUploadRequest{
				HeadSHA:   headHead.Hash().String(),
				BaseSHA:   baseHead.Hash().String(),
				RepoURL:   repoUrl,
				RepoName:  gitURL.GetRepoName(),
				RepoOwner: gitURL.GetOwnerName(),
				RepoHost:  gitURL.GetHostName(),
				Stacks:    affected,
			}

			err = apiClient.UploadAffectedStacks(req)
			if err != nil {
				return err
			}
		}
	} else {
		res, err := u.EvaluateYqExpression(d.atmosConfig, affected, a.Query)
		if err != nil {
			return err
		}

		err = viewWithScroll(pager.New(), term.IsTTYSupportForStdout, printOrWriteToFile, d.atmosConfig, "Affected components and stacks", a.Format, a.OutputFile, res)
		if err != nil {
			return err
		}
	}
	return nil

}

// ExecuteDescribeAffectedCmd executes `describe affected` command
func ExecuteDescribeAffectedCmd(atmosConfig *schema.AtmosConfiguration, a DescribeAffectedCmdArgs) error {
	var affected []schema.Affected
	var headHead, baseHead *plumbing.Reference
	var repoUrl string
	var err error
	if a.RepoPath != "" {
		affected, headHead, baseHead, repoUrl, err = ExecuteDescribeAffectedWithTargetRepoPath(
			a.CLIConfig,
			a.RepoPath,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	} else if a.CloneTargetRef {
		affected, headHead, baseHead, repoUrl, err = ExecuteDescribeAffectedWithTargetRefClone(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.SSHKeyPath,
			a.SSHKeyPassword,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	} else {
		affected, headHead, baseHead, repoUrl, err = ExecuteDescribeAffectedWithTargetRefCheckout(
			a.CLIConfig,
			a.Ref,
			a.SHA,
			a.Verbose,
			a.IncludeSpaceliftAdminStacks,
			a.IncludeSettings,
			a.Stack,
			a.ProcessTemplates,
			a.ProcessYamlFunctions,
			a.Skip,
		)
	}

	if err != nil {
		return err
	}

	// Add dependent components and stacks for each affected component
	if len(affected) > 0 && a.IncludeDependents {
		err = addDependentsToAffected(a.CLIConfig, &affected, a.IncludeSettings)
		if err != nil {
			return err
		}
	}

	if a.Query == "" {
		log.Debug("\nAffected components and stacks: \n")

		err = printOrWriteToFile(atmosConfig, a.Format, a.OutputFile, affected)
		if err != nil {
			return err
		}

		if a.Upload {
			// Parse the repo URL
			gitURL, err := giturl.NewGitURL(repoUrl)
			if err != nil {
				return err
			}
			logger, err := l.NewLoggerFromCliConfig(*atmosConfig)
			if err != nil {
				return err
			}
			apiClient, err := pro.NewAtmosProAPIClientFromEnv(logger)
			if err != nil {
				return err
			}

			req := pro.AffectedStacksUploadRequest{
				HeadSHA:   headHead.Hash().String(),
				BaseSHA:   baseHead.Hash().String(),
				RepoURL:   repoUrl,
				RepoName:  gitURL.GetRepoName(),
				RepoOwner: gitURL.GetOwnerName(),
				RepoHost:  gitURL.GetHostName(),
				Stacks:    affected,
			}

			err = apiClient.UploadAffectedStacks(req)
			if err != nil {
				return err
			}
		}
	} else {
		res, err := u.EvaluateYqExpression(atmosConfig, affected, a.Query)
		if err != nil {
			return err
		}

		err = viewWithScroll(pager.New(), term.IsTTYSupportForStdout, printOrWriteToFile, atmosConfig, "Affected components and stacks", a.Format, a.OutputFile, res)
		if err != nil {
			return err
		}
	}

	return nil
}

func viewWithScroll(pageCreator pager.PageCreator,
	isTTYSupportForStdout func() bool,
	printOrWriteToFile func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error,
	atmosConfig *schema.AtmosConfiguration, displayName string, format string, file string, res any) error {
	if atmosConfig.Settings.Terminal.IsPagerEnabled() && file == "" {
		err := viewConfig(pageCreator, isTTYSupportForStdout, atmosConfig, displayName, format, res)
		switch err.(type) {
		case DescribeConfigFormatError:
			return err
		case nil:
			return nil
		default:
			log.Debug("Failed to use pager")
		}
	}

	err := printOrWriteToFile(atmosConfig, format, file, res)
	if err != nil {
		return err
	}
	return nil
}

func viewConfig(pageCreator pager.PageCreator, isTTYSupportForStdout func() bool, atmosConfig *schema.AtmosConfiguration, displayName string, format string, data any) error {
	if !isTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	var content string
	var err error
	switch format {
	case "yaml":
		content, err = u.GetHighlightedYAML(atmosConfig, data)
		if err != nil {
			return err
		}
	case "json":
		content, err = u.GetHighlightedJSON(atmosConfig, data)
		if err != nil {
			return err
		}
	default:
		return DescribeConfigFormatError{
			format,
		}
	}
	if err := pageCreator.Run(displayName, content); err != nil {
		return err
	}
	return nil
}
