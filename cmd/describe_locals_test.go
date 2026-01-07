package cmd

import (
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
)

// mockDescribeLocalsExec is a mock implementation of DescribeLocalsExec for testing.
type mockDescribeLocalsExec struct {
	executeFunc func(atmosConfig *schema.AtmosConfiguration, args *exec.DescribeLocalsArgs) error
}

func (m *mockDescribeLocalsExec) Execute(atmosConfig *schema.AtmosConfiguration, args *exec.DescribeLocalsArgs) error {
	if m.executeFunc != nil {
		return m.executeFunc(atmosConfig, args)
	}
	return nil
}

func TestGetRunnableDescribeLocalsCmd(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		checkAtmosConfigCalled := false
		processCommandLineArgsCalled := false
		initCliConfigCalled := false
		validateStacksCalled := false
		executeCalled := false

		mockExec := &mockDescribeLocalsExec{
			executeFunc: func(atmosConfig *schema.AtmosConfiguration, args *exec.DescribeLocalsArgs) error {
				executeCalled = true
				return nil
			},
		}

		props := getRunnableDescribeLocalsCmdProps{
			checkAtmosConfig: func(opts ...AtmosValidateOption) {
				checkAtmosConfigCalled = true
			},
			processCommandLineArgs: func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				processCommandLineArgsCalled = true
				return schema.ConfigAndStacksInfo{}, nil
			},
			initCliConfig: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				initCliConfigCalled = true
				return schema.AtmosConfiguration{}, nil
			},
			validateStacks: func(atmosConfig *schema.AtmosConfiguration) error {
				validateStacksCalled = true
				return nil
			},
			newDescribeLocalsExec: mockExec,
		}

		runFunc := getRunnableDescribeLocalsCmd(props)
		cmd := &cobra.Command{}
		cmd.Flags().String("stack", "", "")
		cmd.Flags().String("format", "", "")
		cmd.Flags().String("file", "", "")
		cmd.Flags().String("query", "", "")

		err := runFunc(cmd, []string{})
		require.NoError(t, err)

		assert.True(t, checkAtmosConfigCalled, "checkAtmosConfig should be called")
		assert.True(t, processCommandLineArgsCalled, "processCommandLineArgs should be called")
		assert.True(t, initCliConfigCalled, "initCliConfig should be called")
		assert.True(t, validateStacksCalled, "validateStacks should be called")
		assert.True(t, executeCalled, "execute should be called")
	})

	t.Run("successful execution with component argument", func(t *testing.T) {
		var capturedArgs *exec.DescribeLocalsArgs

		mockExec := &mockDescribeLocalsExec{
			executeFunc: func(atmosConfig *schema.AtmosConfiguration, args *exec.DescribeLocalsArgs) error {
				capturedArgs = args
				return nil
			},
		}

		props := getRunnableDescribeLocalsCmdProps{
			checkAtmosConfig: func(opts ...AtmosValidateOption) {},
			processCommandLineArgs: func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			initCliConfig: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateStacks: func(atmosConfig *schema.AtmosConfiguration) error {
				return nil
			},
			newDescribeLocalsExec: mockExec,
		}

		runFunc := getRunnableDescribeLocalsCmd(props)
		cmd := &cobra.Command{}
		cmd.Flags().String("stack", "", "")
		cmd.Flags().String("format", "", "")
		cmd.Flags().String("file", "", "")
		cmd.Flags().String("query", "", "")

		// Pass component as positional argument.
		err := runFunc(cmd, []string{"vpc"})
		require.NoError(t, err)

		assert.Equal(t, "vpc", capturedArgs.Component)
	})

	// Table-driven tests for error cases to avoid code duplication.
	errorTests := []struct {
		name                  string
		processCommandLineErr error
		initCliConfigErr      error
		validateStacksErr     error
	}{
		{
			name:                  "processCommandLineArgs error",
			processCommandLineErr: errors.New("process error"),
		},
		{
			name:             "initCliConfig error",
			initCliConfigErr: errors.New("init config error"),
		},
		{
			name:              "validateStacks error",
			validateStacksErr: errors.New("validate stacks error"),
		},
	}

	for _, tt := range errorTests {
		t.Run(tt.name, func(t *testing.T) {
			props := getRunnableDescribeLocalsCmdProps{
				checkAtmosConfig: func(opts ...AtmosValidateOption) {},
				processCommandLineArgs: func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
					return schema.ConfigAndStacksInfo{}, tt.processCommandLineErr
				},
				initCliConfig: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
					return schema.AtmosConfiguration{}, tt.initCliConfigErr
				},
				validateStacks: func(atmosConfig *schema.AtmosConfiguration) error {
					return tt.validateStacksErr
				},
				newDescribeLocalsExec: &mockDescribeLocalsExec{},
			}

			runFunc := getRunnableDescribeLocalsCmd(props)
			cmd := &cobra.Command{}

			err := runFunc(cmd, []string{})

			// Determine which error to expect.
			var expectedErr error
			switch {
			case tt.processCommandLineErr != nil:
				expectedErr = tt.processCommandLineErr
			case tt.initCliConfigErr != nil:
				expectedErr = tt.initCliConfigErr
			case tt.validateStacksErr != nil:
				expectedErr = tt.validateStacksErr
			}

			assert.ErrorIs(t, err, expectedErr)
		})
	}

	t.Run("execute error", func(t *testing.T) {
		expectedErr := errors.New("execute error")
		mockExec := &mockDescribeLocalsExec{
			executeFunc: func(atmosConfig *schema.AtmosConfiguration, args *exec.DescribeLocalsArgs) error {
				return expectedErr
			},
		}

		props := getRunnableDescribeLocalsCmdProps{
			checkAtmosConfig: func(opts ...AtmosValidateOption) {},
			processCommandLineArgs: func(componentType string, cmd *cobra.Command, args []string, additionalArgsAndFlags []string) (schema.ConfigAndStacksInfo, error) {
				return schema.ConfigAndStacksInfo{}, nil
			},
			initCliConfig: func(configAndStacksInfo schema.ConfigAndStacksInfo, processStacks bool) (schema.AtmosConfiguration, error) {
				return schema.AtmosConfiguration{}, nil
			},
			validateStacks: func(atmosConfig *schema.AtmosConfiguration) error {
				return nil
			},
			newDescribeLocalsExec: mockExec,
		}

		runFunc := getRunnableDescribeLocalsCmd(props)
		cmd := &cobra.Command{}
		cmd.Flags().String("stack", "", "")
		cmd.Flags().String("format", "", "")
		cmd.Flags().String("file", "", "")
		cmd.Flags().String("query", "", "")

		err := runFunc(cmd, []string{})
		assert.ErrorIs(t, err, expectedErr)
	})
}

func TestSetCliArgsForDescribeLocalsCli(t *testing.T) {
	t.Run("all flags set", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("stack", "", "")
		cmd.Flags().String("format", "", "")
		cmd.Flags().String("file", "", "")
		cmd.Flags().String("query", "", "")

		err := cmd.Flags().Set("stack", "my-stack")
		require.NoError(t, err)
		err = cmd.Flags().Set("format", "json")
		require.NoError(t, err)
		err = cmd.Flags().Set("file", "output.json")
		require.NoError(t, err)
		err = cmd.Flags().Set("query", ".dev")
		require.NoError(t, err)

		args := &exec.DescribeLocalsArgs{}
		err = setCliArgsForDescribeLocalsCli(cmd.Flags(), args)
		require.NoError(t, err)

		assert.Equal(t, "my-stack", args.FilterByStack)
		assert.Equal(t, "json", args.Format)
		assert.Equal(t, "output.json", args.File)
		assert.Equal(t, ".dev", args.Query)
	})

	t.Run("no flags set uses defaults", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("stack", "", "")
		cmd.Flags().String("format", "", "")
		cmd.Flags().String("file", "", "")
		cmd.Flags().String("query", "", "")

		args := &exec.DescribeLocalsArgs{}
		err := setCliArgsForDescribeLocalsCli(cmd.Flags(), args)
		require.NoError(t, err)

		assert.Empty(t, args.FilterByStack)
		assert.Equal(t, "yaml", args.Format)
		assert.Empty(t, args.File)
		assert.Empty(t, args.Query)
	})

	t.Run("only stack flag set", func(t *testing.T) {
		cmd := &cobra.Command{}
		cmd.Flags().String("stack", "", "")
		cmd.Flags().String("format", "", "")
		cmd.Flags().String("file", "", "")
		cmd.Flags().String("query", "", "")

		err := cmd.Flags().Set("stack", "deploy/dev")
		require.NoError(t, err)

		args := &exec.DescribeLocalsArgs{}
		err = setCliArgsForDescribeLocalsCli(cmd.Flags(), args)
		require.NoError(t, err)

		assert.Equal(t, "deploy/dev", args.FilterByStack)
		assert.Equal(t, "yaml", args.Format)
	})
}

func TestDescribeLocalsCmd(t *testing.T) {
	t.Run("command has expected properties", func(t *testing.T) {
		assert.Equal(t, "locals [component]", describeLocalsCmd.Use)
		assert.NotEmpty(t, describeLocalsCmd.Short)
		assert.NotEmpty(t, describeLocalsCmd.Long)
		assert.NotEmpty(t, describeLocalsCmd.Example)
	})

	t.Run("command has expected flags", func(t *testing.T) {
		stackFlag := describeLocalsCmd.Flag("stack")
		assert.NotNil(t, stackFlag, "stack flag should exist")
		assert.Equal(t, "s", stackFlag.Shorthand)

		formatFlag := describeLocalsCmd.Flag("format")
		assert.NotNil(t, formatFlag, "format flag should exist")
		assert.Equal(t, "f", formatFlag.Shorthand)

		fileFlag := describeLocalsCmd.Flag("file")
		assert.NotNil(t, fileFlag, "file flag should exist")

		queryFlag := describeLocalsCmd.Flag("query")
		assert.NotNil(t, queryFlag, "query flag should exist")
		assert.Equal(t, "q", queryFlag.Shorthand)
	})
}
