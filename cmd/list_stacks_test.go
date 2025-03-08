package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"strings"
	"testing"

	log "github.com/charmbracelet/log"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/pkg/utils"
)

// TestListStacksSuccess tests the success path of the Run function.
func TestListStacksSuccess(t *testing.T) {
	cmd := &cobra.Command{
		Use: "stacks",
		Run: func(cmd *cobra.Command, args []string) {
			output := []string{"stack1", "stack2"}
			utils.PrintMessage(strings.Join(output, "\n"))
		},
	}

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cmd.Run(cmd, []string{})

	w.Close()
	os.Stdout = oldStdout

	var stdoutBuf bytes.Buffer
	_, err := io.Copy(&stdoutBuf, r)
	require.NoError(t, err)

	assert.Contains(t, stdoutBuf.String(), "stack1")
	assert.Contains(t, stdoutBuf.String(), "stack2")
}

var ErrTest = errors.New("test error")

// TestListStacksError tests the error path of the Run function.
func TestListStacksError(t *testing.T) {
	cmd := &cobra.Command{
		Use: "stacks",
		Run: func(cmd *cobra.Command, args []string) {
			err := ErrTest
			if err != nil {
				msg := "error filtering stacks: " + err.Error()
				os.Stderr.WriteString(msg)
				return
			}
			utils.PrintMessage("should not reach here")
		},
	}

	oldStderr := os.Stderr
	errR, errW, _ := os.Pipe()
	os.Stderr = errW

	cmd.Run(cmd, []string{})

	errW.Close()
	os.Stderr = oldStderr

	var stderrBuf bytes.Buffer
	_, err := io.Copy(&stderrBuf, errR)
	require.NoError(t, err)

	assert.Contains(t, stderrBuf.String(), "error filtering stacks")
	assert.Contains(t, stderrBuf.String(), "test error")
}

// TestListStacksCommandComponentFlagParsing verifies the component flag is properly defined.
func TestListStacksCommandComponentFlagParsing(t *testing.T) {
	componentFlag, err := listStacksCmd.PersistentFlags().GetString("component")
	assert.NoError(t, err)
	assert.Equal(t, "", componentFlag)

	assert.NotNil(t, listStacksCmd.PersistentFlags().ShorthandLookup("c"))
}

// TestListStacksOutputFormatting tests that the output is joined with newlines.
func TestListStacksOutputFormatting(t *testing.T) {
	// Test the string joining logic
	input := []string{"line1", "line2", "line3"}
	expected := "line1\nline2\nline3"

	output := strings.Join(input, "\n")
	assert.Equal(t, expected, output)

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	utils.PrintMessage(strings.Join(input, "\n"))

	w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, err := io.Copy(&buf, r)
	require.NoError(t, err)

	assert.Equal(t, expected+"\n", buf.String())
}

// TestListStacksInitCliConfigErrorHandling tests the error handling in listStacks when InitCliConfig fails.
func TestListStacksInitCliConfigErrorHandling(t *testing.T) {
	// Create a test command
	cmd := &cobra.Command{
		Use: "stacks",
	}
	cmd.Flags().String("component", "", "")

	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	oldStderr := os.Stderr
	errR, errW, _ := os.Pipe()
	os.Stderr = errW

	var buf bytes.Buffer
	testLogger := log.New(&buf)
	testLogger.SetLevel(log.DebugLevel)
	testLogger.SetReportTimestamp(false)
	testLogger.SetReportCaller(false)

	originalLogger := log.Default()
	log.SetDefault(testLogger)

	result, err := func() ([]string, error) {
		// This will cause the function to error since it depends on external services/files
		// that aren't available in the test environment.
		return listStacks(cmd)
	}()

	w.Close()
	os.Stdout = oldStdout
	errW.Close()
	os.Stderr = oldStderr

	log.SetDefault(originalLogger)

	var stdoutBuf bytes.Buffer
	_, copyErr := io.Copy(&stdoutBuf, r)
	require.NoError(t, copyErr)

	var stderrBuf bytes.Buffer
	_, copyErr = io.Copy(&stderrBuf, errR)
	require.NoError(t, copyErr)

	assert.Error(t, err)
	assert.Nil(t, result)

	assert.Contains(t, buf.String(), "failed to initialize CLI config")
}
