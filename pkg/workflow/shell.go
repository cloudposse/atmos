package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/theme"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/pkg/errors"
)

// Static error definitions.
var (
	ErrShellCommandFailed = errors.New("shell command failed")
)

// ExecuteShell executes a shell command.
func ExecuteShell(
	atmosConfig *schema.AtmosConfiguration,
	command string,
	commandName string,
	workingDir string,
	env []string,
	dryRun bool,
) error {
	if dryRun {
		u.PrintMessageInColor(fmt.Sprintf("[DRY-RUN] Would execute shell command: %s", command), theme.Colors.Info)
		return nil
	}

	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = workingDir
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error executing shell command '%s': %v\nOutput: %s", command, err, string(output))
	}

	// Print the command output
	fmt.Print(string(output))
	return nil
}

// ExecuteShellCommand executes a shell command with the given name and arguments.
func ExecuteShellCommand(
	atmosConfig *schema.AtmosConfiguration,
	commandName string,
	args []string,
	workingDir string,
	env []string,
	dryRun bool,
	description string,
) error {
	if dryRun {
		u.PrintMessageInColor(fmt.Sprintf("[DRY-RUN] Would execute command: %s %s", commandName, strings.Join(args, " ")), theme.Colors.Info)
		return nil
	}

	cmd := exec.Command(commandName, args...)
	cmd.Dir = workingDir
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("error executing command '%s %s': %v\nOutput: %s", commandName, strings.Join(args, " "), err, string(output))
	}

	// Print the command output
	fmt.Print(string(output))
	return nil
}
