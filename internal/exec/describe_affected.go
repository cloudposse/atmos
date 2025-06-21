package exec

import (
	log "github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	l "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/selector"
	"github.com/go-git/go-git/v5/plumbing"
	giturl "github.com/kubescape/go-git-url"

	"github.com/cloudposse/atmos/pkg/pro"
	"github.com/cloudposse/atmos/pkg/pro/dtos"

	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

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
	Selector                    string
}

//go:generate mockgen -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeAffectedExec interface {
	Execute(*DescribeAffectedCmdArgs) error
}

type describeAffectedExec struct {
	atmosConfig                               *schema.AtmosConfiguration
	executeDescribeAffectedWithTargetRepoPath func(
		atmosConfig *schema.AtmosConfiguration,
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
		atmosConfig *schema.AtmosConfiguration,
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
		atmosConfig *schema.AtmosConfiguration,
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
		atmosConfig *schema.AtmosConfiguration,
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
	atmosConfig *schema.AtmosConfiguration,
) DescribeAffectedExec {
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

func (d *describeAffectedExec) Execute(a *DescribeAffectedCmdArgs) error {
	var affected []schema.Affected
	var headHead, baseHead *plumbing.Reference
	var repoUrl string
	var err error
	switch {
	case a.RepoPath != "":
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
	case a.CloneTargetRef:
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
	default:
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

	// Apply selector filtering if provided
	if a.Selector != "" {
		affected, err = d.applyAffectedSelectorFilter(a.CLIConfig, affected, a.Selector)
		if err != nil {
			return err
		}
	}

	// Add dependent components and stacks for each affected component
	if len(affected) > 0 && a.IncludeDependents {
		err = d.addDependentsToAffected(a.CLIConfig, &affected, a.IncludeSettings)
		if err != nil {
			return err
		}
	}

	return d.view(a, repoUrl, headHead, baseHead, affected)
}

func (d *describeAffectedExec) view(a *DescribeAffectedCmdArgs, repoUrl string, headHead, baseHead *plumbing.Reference, affected []schema.Affected) error {
	if a.Query == "" {
		return d.uploadableQuery(a, repoUrl, headHead, baseHead, affected)
	}

	res, err := u.EvaluateYqExpression(d.atmosConfig, affected, a.Query)
	if err != nil {
		return err
	}

	return viewWithScroll(&viewWithScrollProps{d.pageCreator, term.IsTTYSupportForStdout, d.printOrWriteToFile, d.atmosConfig, "Affected components and stacks", a.Format, a.OutputFile, res})
}

func (d *describeAffectedExec) uploadableQuery(args *DescribeAffectedCmdArgs, repoUrl string, headHead, baseHead *plumbing.Reference, affected []schema.Affected) error {
	log.Debug("\nAffected components and stacks: \n")

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
	logger, err := l.NewLoggerFromCliConfig(*d.atmosConfig)
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

	log.Debug("Creating API client")
	apiClient, err := pro.NewAtmosProAPIClientFromEnv(logger, d.atmosConfig)
	if err != nil {
		return err
	}

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
		switch err.(type) {
		case DescribeConfigFormatError:
			return err
		case nil:
			return nil
		default:
			log.Debug("Failed to use pager")
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

func (d *describeAffectedExec) applyAffectedSelectorFilter(atmosConfig *schema.AtmosConfiguration, affected []schema.Affected, selectorStr string) ([]schema.Affected, error) {
	if selectorStr == "" {
		return affected, nil
	}

	// Parse the selector
	reqs, err := selector.Parse(selectorStr)
	if err != nil {
		return nil, err
	}

	// Get all stacks to extract labels from
	stacksMap, _, err := FindStacksMap(*atmosConfig, false)
	if err != nil {
		return nil, err
	}

	var filtered []schema.Affected

	for _, affectedItem := range affected {
		// Extract labels for this affected component/stack
		stackData, ok := stacksMap[affectedItem.Stack]
		if !ok {
			continue
		}

		// Get merged labels (stack + component)
		merged := selector.MergedLabels(stackData.(map[string]any), affectedItem.Component)

		// Check if the affected item matches the selector
		if selector.Matches(merged, reqs) {
			filtered = append(filtered, affectedItem)
		}
	}

	// If no matches, print the standard message
	if len(filtered) == 0 {
		log.Info("No affected components matched selector.")
	}

	return filtered, nil
}
