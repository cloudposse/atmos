package exec

import (
	"errors"
	"fmt"

	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteDescribeAffectedCmd executes `describe affected` command
func ExecuteDescribeAffectedCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
	if err != nil {
		return err
	}

	err = ValidateStacks(cliConfig)
	if err != nil {
		return err
	}

	// Process flags
	flags := cmd.Flags()

	ref, err := flags.GetString("ref")
	if err != nil {
		return err
	}

	sha, err := flags.GetString("sha")
	if err != nil {
		return err
	}

	repoPath, err := flags.GetString("repo-path")
	if err != nil {
		return err
	}

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	if format != "" && format != "yaml" && format != "json" {
		return fmt.Errorf("invalid '--format' flag '%s'. Valid values are 'json' (default) and 'yaml'", format)
	}

	if format == "" {
		format = "json"
	}

	file, err := flags.GetString("file")
	if err != nil {
		return err
	}

	verbose, err := flags.GetBool("verbose")
	if err != nil {
		return err
	}

	sshKeyPath, err := flags.GetString("ssh-key")
	if err != nil {
		return err
	}

	sshKeyPassword, err := flags.GetString("ssh-key-password")
	if err != nil {
		return err
	}

	includeSpaceliftAdminStacks, err := flags.GetBool("include-spacelift-admin-stacks")
	if err != nil {
		return err
	}

	includeDependents, err := flags.GetBool("include-dependents")
	if err != nil {
		return err
	}

	cloneTargetRef, err := flags.GetBool("clone-target-ref")
	if err != nil {
		return err
	}

	if repoPath != "" && (ref != "" || sha != "" || sshKeyPath != "" || sshKeyPassword != "") {
		return errors.New("if the '--repo-path' flag is specified, the '--ref', '--sha', '--ssh-key' and '--ssh-key-password' flags can't be used")
	}

	var affected []schema.Affected

	if repoPath != "" {
		affected, err = ExecuteDescribeAffectedWithTargetRepoPath(cliConfig, repoPath, verbose, includeSpaceliftAdminStacks)
	} else if cloneTargetRef {
		affected, err = ExecuteDescribeAffectedWithTargetRefClone(cliConfig, ref, sha, sshKeyPath, sshKeyPassword, verbose, includeSpaceliftAdminStacks)
	} else {
		affected, err = ExecuteDescribeAffectedWithTargetRefCheckout(cliConfig, ref, sha, verbose, includeSpaceliftAdminStacks)
	}

	if err != nil {
		return err
	}

	// Add dependent components and stacks for each affected component
	if len(affected) > 0 && includeDependents {
		err = addDependentsToAffected(cliConfig, &affected)
		if err != nil {
			return err
		}
	}

	if verbose {
		cliConfig.Logs.Level = u.LogLevelTrace
	}

	u.LogTrace(cliConfig, fmt.Sprintf("\nAffected components and stacks: \n"))

	err = printOrWriteToFile(format, file, affected)
	if err != nil {
		return err
	}

	return nil
}
