package cmd

import (
	"os"
	"os/exec"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestAnsibleRun_NoArgs_ShowsHelpAndExits(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		stacksPath := "../tests/fixtures/scenarios/stack-templates-2"
		_ = os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		_ = os.Setenv("ATMOS_BASE_PATH", stacksPath)
		defer func() {
			os.Unsetenv("ATMOS_BASE_PATH")
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		}()
		parent := ansibleCmd
		actual := &cobra.Command{Use: "run"}
		_ = ansibleRun(parent, actual, "run", []string{})
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestAnsibleRun_NoArgs_ShowsHelpAndExits")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 1")
	}
}

func TestAnsibleVersion_Subcommand(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		stacksPath := "../tests/fixtures/scenarios/stack-templates-2"
		_ = os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		_ = os.Setenv("ATMOS_BASE_PATH", stacksPath)
		defer func() {
			os.Unsetenv("ATMOS_BASE_PATH")
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		}()
		parent := ansibleCmd
		actual := &cobra.Command{Use: "version"}
		_ = ansibleRun(parent, actual, "version", []string{})
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestAnsibleVersion_Subcommand")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 1")
	}
}
