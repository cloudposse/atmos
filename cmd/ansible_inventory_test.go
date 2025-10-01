package cmd

import (
	"os"
	"os/exec"
	"testing"

	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
)

func TestAnsibleInventory_HelpExit(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		stacksPath := "../tests/fixtures/scenarios/stack-templates"
		_ = os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		_ = os.Setenv("ATMOS_BASE_PATH", stacksPath)
		defer func() {
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
			os.Unsetenv("ATMOS_BASE_PATH")
		}()

		cmds := getAnsibleCommands()
		for _, c := range cmds {
			if c.Use == "inventory" {
				_ = c.RunE(ansibleCmd, []string{"--help"})
				break
			}
		}
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestAnsibleInventory_HelpExit")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 0, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 0")
	}
}
