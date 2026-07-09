package exec

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gomock "go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
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
				assert.Equal(t, "something", data)
				return nil
			},
		}

		err := dc.ExecuteDescribeConfigCmd(".components.terraform.base_path", "yaml", "")
		assert.NoError(t, err)
	})

	t.Run("ExecuteDescribeConfigCmd_WithQuery_EvalError", func(t *testing.T) {
		printCalled := false
		dc := &describeConfigExec{
			atmosConfig:           config,
			IsTTYSupportForStdout: func() bool { return false },
			printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
				printCalled = true
				return nil
			},
		}

		err := dc.ExecuteDescribeConfigCmd(".component.terraform[", "yaml", "")
		assert.Error(t, err)
		assert.ErrorContains(t, err, "failed to evaluate YQ expression '.component.terraform['")
		assert.False(t, printCalled, "invalid query must not produce any stdout data")
	})
}

// TestDescribeConfigQueryResults verifies that `--query` returns only the queried
// subtree (map, scalar, or list) instead of coercing the result back into a full
// AtmosConfiguration struct.
func TestDescribeConfigQueryResults(t *testing.T) {
	config := &schema.AtmosConfiguration{
		BasePath: "./",
		Stacks: schema.Stacks{
			BasePath:      "stacks",
			NameTemplate:  "{{ .vars.stage }}",
			IncludedPaths: []string{"deploy/**/*", "catalog/**/*"},
		},
	}

	tests := []struct {
		name   string
		query  string
		format string
		verify func(t *testing.T, data any)
	}{
		{
			name:   "map subtree query returns only the subtree",
			query:  ".stacks",
			format: "yaml",
			verify: func(t *testing.T, data any) {
				res, ok := data.(map[string]any)
				require.True(t, ok, "expected map[string]any, got %T", data)
				assert.Equal(t, "{{ .vars.stage }}", res["name_template"])
				assert.Equal(t, "stacks", res["base_path"])
				// The full config would contain these top-level sections.
				assert.NotContains(t, res, "stacks")
				assert.NotContains(t, res, "components")
			},
		},
		{
			name:   "scalar query returns the scalar value in yaml format",
			query:  ".stacks.name_template",
			format: "yaml",
			verify: func(t *testing.T, data any) {
				assert.Equal(t, "{{ .vars.stage }}", data)
			},
		},
		{
			name:   "scalar query returns the scalar value in json format",
			query:  ".stacks.name_template",
			format: "json",
			verify: func(t *testing.T, data any) {
				assert.Equal(t, "{{ .vars.stage }}", data)
			},
		},
		{
			name:   "list query returns the list elements",
			query:  ".stacks.included_paths",
			format: "yaml",
			verify: func(t *testing.T, data any) {
				res, ok := data.([]any)
				require.True(t, ok, "expected []any, got %T", data)
				require.Len(t, res, 2)
				assert.Equal(t, "deploy/**/*", res[0])
				assert.Equal(t, "catalog/**/*", res[1])
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var captured any
			dc := &describeConfigExec{
				atmosConfig:           config,
				IsTTYSupportForStdout: func() bool { return false },
				printOrWriteToFile: func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error {
					assert.Equal(t, tt.format, format)
					captured = data
					return nil
				},
			}

			err := dc.ExecuteDescribeConfigCmd(tt.query, tt.format, "")
			require.NoError(t, err)
			tt.verify(t, captured)
		})
	}
}

// TestDescribeConfigQueryPager verifies that query results (including scalars)
// render through the pager path without being coerced into the config struct.
func TestDescribeConfigQueryPager(t *testing.T) {
	config := &schema.AtmosConfiguration{
		Stacks: schema.Stacks{
			NameTemplate: "{{ .vars.stage }}",
		},
		Settings: schema.AtmosSettings{
			Terminal: schema.Terminal{
				Pager: "less",
			},
		},
	}

	tests := []struct {
		name   string
		query  string
		format string
	}{
		{name: "scalar query yaml via pager", query: ".stacks.name_template", format: "yaml"},
		{name: "scalar query json via pager", query: ".stacks.name_template", format: "json"},
		{name: "map query yaml via pager", query: ".stacks", format: "yaml"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockPager := pager.NewMockPageCreator(ctrl)
			mockPager.EXPECT().Run(describeConfigTitle, gomock.Any()).Return(nil)

			dc := &describeConfigExec{
				atmosConfig:           config,
				pageCreator:           mockPager,
				IsTTYSupportForStdout: func() bool { return true },
			}

			err := dc.ExecuteDescribeConfigCmd(tt.query, tt.format, "")
			require.NoError(t, err)
		})
	}
}
