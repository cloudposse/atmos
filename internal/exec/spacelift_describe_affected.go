package exec

import (
	"fmt"
	"github.com/samber/lo"
	"github.com/spf13/cobra"
	"sort"

	cfg "github.com/cloudposse/atmos/pkg/config"
	u "github.com/cloudposse/atmos/pkg/utils"
)

// ExecuteSpaceliftDescribeAffectedCmd executes `spacelift describe affected` command
func ExecuteSpaceliftDescribeAffectedCmd(cmd *cobra.Command, args []string) error {
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

	affected, err := ExecuteDescribeAffected(cliConfig, ref, sha, sshKeyPath, sshKeyPassword, verbose)
	if err != nil {
		return err
	}

	u.PrintInfoVerbose(verbose && file == "", fmt.Sprintf("\nAffected Spacelift stacks: \n"))

	// Get the affected Spacelift stacks from the list of objects
	// https://github.com/samber/lo#filtermap
	res := lo.FilterMap[cfg.Affected, string](affected, func(item cfg.Affected, _ int) (string, bool) {
		if item.SpaceliftStack != "" {
			return item.SpaceliftStack, true
		}
		return "", false
	})

	sort.Strings(res)

	err = printOrWriteToFile(format, file, res)
	if err != nil {
		return err
	}

	return nil
}
