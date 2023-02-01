package exec

import (
	"fmt"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteDescribeAffectedCmd executes `describe affected` command
func ExecuteDescribeAffectedCmd(cmd *cobra.Command, args []string) error {
	info, err := processCommandLineArgs("", cmd, args)
	if err != nil {
		return err
	}

	cliConfig, err := cfg.InitCliConfig(info, true)
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

	affected, err := ExecuteDescribeAffected(cliConfig, ref, sha, repoPath, sshKeyPath, sshKeyPassword, verbose)
	if err != nil {
		return err
	}

	u.PrintInfoVerbose(verbose && file == "", fmt.Sprintf("\nAffected components and stacks: \n"))

	err = printOrWriteToFile(format, file, affected)
	if err != nil {
		return err
	}

	return nil
}
