package exec

import (
	"testing"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	gomock "go.uber.org/mock/gomock"
	"github.com/stretchr/testify/assert"
)

func TestErrInvalidFormat_Error(t *testing.T) {
	err := DescribeConfigFormatError{format: "invalid"}
	assert.Equal(t, "invalid 'format': invalid", err.Error())
}

func TestDescribeConfig(t *testing.T) {
	// Setup test data
	config := &schema.AtmosConfiguration{
		Components: schema.Components{
			Terraform: schema.Terraform{
				BasePath: "something",
			},
		},
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				Pager: "less",
			},
		},
	}

	t.Run("NewDescribeConfig", func(t *testing.T) {
		dc := NewDescribeConfig(config)
		assert.Equal(t, config, dc.atmosConfig)
		assert.NotNil(t, dc.pageCreator)
		assert.NotNil(t, dc.printOrWriteToFile)
	})

	t.Run("ExecuteDescribeConfigCmd_NoQuery_YAML_TTY", func(t *testing.T) {
		// Mock dependencies
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockPager := pager.NewMockPageCreator(ctrl)
		mockPager.EXPECT().Run(describeConfigTitle, gomock.Any()).Return(nil)
		dc := &describeConfigExec{
			atmosConfig:           config,
			pageCreator:           mockPager,
			IsTTYSupportForStdout: func() bool { return true },
		}

		err := dc.ExecuteDescribeConfigCmd("", "yaml", "")
		assert.NoError(t, err)
	})

	t.Run("ExecuteDescribeConfigCmd_NoQuery_JSON_TTY", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()
		mockPager := pager.NewMockPageCreator(ctrl)
		mockPager.EXPECT().Run(describeConfigTitle, gomock.Any()).Return(nil)

		dc := &describeConfigExec{
			atmosConfig:           config,
			pageCreator:           mockPager,
			IsTTYSupportForStdout: func() bool { return true },
		}

		err := dc.ExecuteDescribeConfigCmd("", "json", "")
		assert.NoError(t, err)
	})

	t.Run("ExecuteDescribeConfigCmd_NoQuery_InvalidFormat_TTY", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		dc := &describeConfigExec{
			atmosConfig:           config,
			IsTTYSupportForStdout: func() bool { return true },
		}

		err := dc.ExecuteDescribeConfigCmd("", "invalid", "")
		assert.Error(t, err)
		assert.Equal(t, DescribeConfigFormatError{format: "invalid"}, err)
	})

	t.Run("ExecuteDescribeConfigCmd_NoQuery_NoTTY", func(t *testing.T) {
		dc := &describeConfigExec{
			atmosConfig:           config,
			IsTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
				assert.Equal(t, "yaml", format)
				assert.Equal(t, "", file)
				assert.Equal(t, config, data)
				return nil
			},
		}

		err := dc.ExecuteDescribeConfigCmd("", "yaml", "")
		assert.NoError(t, err)
	})

	t.Run("ExecuteDescribeConfigCmd_WithQuery", func(t *testing.T) {
		dc := &describeConfigExec{
			atmosConfig:           config,
			IsTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
				assert.Equal(t, "yaml", format)
				assert.Equal(t, "", file)
				assert.Equal(t, config, data)
				return nil
			},
		}

		err := dc.ExecuteDescribeConfigCmd("", "yaml", "")
		assert.NoError(t, err)
	})

	t.Run("ExecuteDescribeConfigCmd_WithQuery_EvalError", func(t *testing.T) {
		dc := &describeConfigExec{
			atmosConfig:           config,
			IsTTYSupportForStdout: func() bool { return false },
		}

		err := dc.ExecuteDescribeConfigCmd(".component.terraform[", "yaml", "")
		assert.Error(t, err)
		assert.Equal(t, "EvaluateYqExpressionWithType: failed to evaluate YQ expression '.component.terraform[': bad expression, could not find matching `]`", err.Error())
	})
}
