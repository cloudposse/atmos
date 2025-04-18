package cmd

import (
	"os"
	"os/exec"
	"testing"

	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
)

func TestTerraformRun1(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		stacksPath := "../tests/fixtures/scenarios/stack-templates-2"

		err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

		err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
		assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

		// Unset env values after testing
		defer func() {
			os.Unsetenv("ATMOS_BASE_PATH")
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		}()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "test",
		}

		terraformRun(cmd, cmd, []string{})
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestTerraformRun1")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 1")
	}
}

func TestTerraformRun2(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		stacksPath := "../tests/fixtures/scenarios/stack-templates-2"

		err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

		err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
		assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

		// Unset env values after testing
		defer func() {
			os.Unsetenv("ATMOS_BASE_PATH")
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		}()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "test",
		}

		cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing terraform commands")

		terraformRun(cmd, cmd, []string{})
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestTerraformRun2")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 1")
	}
}

func TestTerraformRun3(t *testing.T) {
	if os.Getenv("TEST_EXIT") == "1" {
		stacksPath := "../tests/fixtures/scenarios/stack-templates-2"

		err := os.Setenv("ATMOS_CLI_CONFIG_PATH", stacksPath)
		assert.NoError(t, err, "Setting 'ATMOS_CLI_CONFIG_PATH' environment variable should execute without error")

		err = os.Setenv("ATMOS_BASE_PATH", stacksPath)
		assert.NoError(t, err, "Setting 'ATMOS_BASE_PATH' environment variable should execute without error")

		// Unset env values after testing
		defer func() {
			os.Unsetenv("ATMOS_BASE_PATH")
			os.Unsetenv("ATMOS_CLI_CONFIG_PATH")
		}()

		cmd := &cobra.Command{
			Use:   "test",
			Short: "test",
		}

		cmd.PersistentFlags().Bool("process-templates", true, "Enable/disable Go template processing in Atmos stack manifests when executing terraform commands")
		cmd.PersistentFlags().Bool("process-functions", true, "Enable/disable YAML functions processing in Atmos stack manifests when executing terraform commands")

		terraformRun(cmd, cmd, []string{})
		return
	}
	execPath, err := exec.LookPath(os.Args[0])
	assert.Nil(t, err)
	cmd := exec.Command(execPath, "-test.run=TestTerraformRun3")
	cmd.Env = append(os.Environ(), "TEST_EXIT=1")
	err = cmd.Run()
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		assert.Equal(t, 1, exitError.ExitCode())
	} else {
		assert.Fail(t, "Expected an exit error with code 1")
	}
}
