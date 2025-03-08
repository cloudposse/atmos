package exec

import (
	"errors"
	"fmt"

	"github.com/go-git/go-git/v5/plumbing"
	giturl "github.com/kubescape/go-git-url"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	l "github.com/cloudposse/atmos/pkg/logger"
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
	Logger                      *l.Logger
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

func parseDescribeAffectedCliArgs(cmd *cobra.Command, args []string) (DescribeAffectedCmdArgs, error) {
	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	atmosConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}
	logger, err := l.InitializeLoggerFromCliConfig(&atmosConfig)
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	err = ValidateStacks(atmosConfig)
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	// Process flags
	flags := cmd.Flags()

	ref, err := flags.GetString("ref")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	sha, err := flags.GetString("sha")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	repoPath, err := flags.GetString("repo-path")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	if format != "" && format != "yaml" && format != "json" {
		return DescribeAffectedCmdArgs{}, fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'json' (default) and 'yaml'", format)
	}

	if format == "" {
		format = "json"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	verbose, err := flags.GetBool("verbose")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	sshKeyPath, err := flags.GetString("ssh-key")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	sshKeyPassword, err := flags.GetString("ssh-key-password")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	includeSpaceliftAdminStacks, err := flags.GetBool("include-spacelift-admin-stacks")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	includeDependents, err := flags.GetBool("include-dependents")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	includeSettings, err := flags.GetBool("include-settings")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	upload, err := flags.GetBool("upload")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	cloneTargetRef, err := flags.GetBool("clone-target-ref")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	stack, err := flags.GetString("stack")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	if repoPath != "" && (ref != "" || sha != "" || sshKeyPath != "" || sshKeyPassword != "") {
		return DescribeAffectedCmdArgs{}, errors.New("if the '--repo-path' flag is specified, the '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")
	}

	// When uploading, always include dependents and settings for all affected components
	if upload {
		includeDependents = true
		includeSettings = true
	}

	if verbose {
		atmosConfig.Logs.Level = u.LogLevelTrace
		err := logger.SetLogLevel(l.LogLevelTrace)
		if err != nil {
			return DescribeAffectedCmdArgs{}, err
		}
	}

	query, err := flags.GetString("query")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	processTemplates, err := flags.GetBool("process-templates")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	processYamlFunctions, err := flags.GetBool("process-functions")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	skip, err := flags.GetStringSlice("skip")
	if err != nil {
		return DescribeAffectedCmdArgs{}, err
	}

	result := DescribeAffectedCmdArgs{
		CLIConfig:                   atmosConfig,
		CloneTargetRef:              cloneTargetRef,
		Format:                      format,
		IncludeDependents:           includeDependents,
		IncludeSettings:             includeSettings,
		IncludeSpaceliftAdminStacks: includeSpaceliftAdminStacks,
		Logger:                      logger,
		OutputFile:                  file,
		Ref:                         ref,
		RepoPath:                    repoPath,
		SHA:                         sha,
		SSHKeyPath:                  sshKeyPath,
		SSHKeyPassword:              sshKeyPassword,
		Verbose:                     verbose,
		Upload:                      upload,
		Stack:                       stack,
		Query:                       query,
		ProcessTemplates:            processTemplates,
		ProcessYamlFunctions:        processYamlFunctions,
		Skip:                        skip,
	}

	return result, nil
}

// ExecuteDescribeAffectedCmd executes `describe affected` command
func ExecuteDescribeAffectedCmd(cmd *cobra.Command, args []string) error {
	a, err := parseDescribeAffectedCliArgs(cmd, args)
	if err != nil {
		return err
	}

	var affected []schema.Affected
	var headHead, baseHead *plumbing.Reference
	var repoUrl string

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
		a.Logger.Trace("\nAffected components and stacks: \n")

		err = printOrWriteToFile(a.Format, a.OutputFile, affected)
		if err != nil {
			return err
		}

		if a.Upload {
			// Parse the repo URL
			gitURL, err := giturl.NewGitURL(repoUrl)
			if err != nil {
				return err
			}

			apiClient, err := pro.NewAtmosProAPIClientFromEnv(a.Logger)
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
		atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, true)
		if err != nil {
			return err
		}

		res, err := u.EvaluateYqExpression(&atmosConfig, affected, a.Query)
		if err != nil {
			return err
		}

		err = printOrWriteToFile(a.Format, a.OutputFile, res)
		if err != nil {
			return err
		}
	}

	return nil
}
