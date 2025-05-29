package exec

import (
	"errors"
	"testing"

	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
)

// Mock interfaces for dependencies
//go:generate mockgen -destination=mock_pager.go -package=exec github.com/cloudposse/atmos/pkg/pager PageCreator
//go:generate mockgen -destination=mock_term.go -package=exec github.com/cloudposse/atmos/internal/tui/templates/term IsTTYSupportForStdout

func TestDescribeWorkflowsExec_Execute(t *testing.T) {
	// Setup mock controller
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	atmosConfig := &schema.AtmosConfiguration{}
	// Create mocks
	mockPagerCreator := pager.NewMockPageCreator(ctrl)

	// Mock data
	describeWorkflowsList := []schema.DescribeWorkflowsItem{
		{File: "workflow1.yaml"},
		{Workflow: "workflow1"},
	}
	describeWorkflowsMap := map[string][]string{
		"stack1": {"workflow1", "workflow2"},
	}
	describeWorkflowsAll := map[string]schema.WorkflowManifest{
		"workflow1": {Name: "workflow1", Workflows: schema.WorkflowConfig{}},
	}
	query := ".workflow1"
	format := "yaml"

	// Define test cases
	tests := []struct {
		name                     string
		args                     DescribeWorkflowsArgs
		executeWorkflowsResult   []schema.DescribeWorkflowsItem
		executeWorkflowsMap      map[string][]string
		executeWorkflowsAll      map[string]schema.WorkflowManifest
		executeWorkflowsErr      error
		queryResult              interface{}
		queryErr                 error
		printOrWriteErr          error
		isTTYSupport             bool
		expectedErr              error
		expectedPrintOrWriteData interface{}
	}{
		{
			name: "Successful execution with list output",
			args: DescribeWorkflowsArgs{
				OutputType: "list",
				Format:     format,
				Query:      "",
			},
			executeWorkflowsResult:   describeWorkflowsList,
			executeWorkflowsMap:      describeWorkflowsMap,
			executeWorkflowsAll:      describeWorkflowsAll,
			executeWorkflowsErr:      nil,
			isTTYSupport:             true,
			expectedPrintOrWriteData: describeWorkflowsList,
			expectedErr:              nil,
		},
		{
			name: "Successful execution with map output",
			args: DescribeWorkflowsArgs{
				OutputType: "map",
				Format:     format,
				Query:      "",
			},
			executeWorkflowsResult:   describeWorkflowsList,
			executeWorkflowsMap:      describeWorkflowsMap,
			executeWorkflowsAll:      describeWorkflowsAll,
			executeWorkflowsErr:      nil,
			isTTYSupport:             true,
			expectedPrintOrWriteData: describeWorkflowsMap,
			expectedErr:              nil,
		},
		{
			name: "Successful execution with default output and query",
			args: DescribeWorkflowsArgs{
				OutputType: "",
				Format:     format,
				Query:      query,
			},
			executeWorkflowsResult:   describeWorkflowsList,
			executeWorkflowsMap:      describeWorkflowsMap,
			executeWorkflowsAll:      describeWorkflowsAll,
			executeWorkflowsErr:      nil,
			isTTYSupport:             true,
			expectedPrintOrWriteData: map[string]interface{}{"Steps": []interface{}{map[string]interface{}{"Command": "step1"}}},
			expectedErr:              nil,
		},
		{
			name: "Error in executeDescribeWorkflows",
			args: DescribeWorkflowsArgs{
				OutputType: "list",
				Format:     format,
				Query:      "",
			},
			executeWorkflowsErr: errors.New("failed to execute workflows"),
			isTTYSupport:        true,
			expectedErr:         errors.New("failed to execute workflows"),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup mock functions
			executeDescribeWorkflows := func(_ schema.AtmosConfiguration) ([]schema.DescribeWorkflowsItem, map[string][]string, map[string]schema.WorkflowManifest, error) {
				return tt.executeWorkflowsResult, tt.executeWorkflowsMap, tt.executeWorkflowsAll, tt.executeWorkflowsErr
			}

			printOrWriteToFile := func(_ *schema.AtmosConfiguration, _, _ string, data interface{}) error {
				assert.Equal(t, tt.expectedPrintOrWriteData, data, "Unexpected data passed to printOrWriteToFile")
				return tt.printOrWriteErr
			}
			mockPagerCreator.EXPECT().Run(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()
			// Setup describeWorkflowsExec with mocks
			d := &describeWorkflowsExec{
				printOrWriteToFile:       printOrWriteToFile,
				IsTTYSupportForStdout:    func() bool { return tt.isTTYSupport },
				executeDescribeWorkflows: executeDescribeWorkflows,
				pagerCreator:             mockPagerCreator,
			}

			// Execute the method
			err := d.Execute(atmosConfig, &tt.args)

			// Assert results
			if tt.expectedErr != nil {
				assert.Error(t, err, "Expected an error")
				assert.EqualError(t, err, tt.expectedErr.Error(), "Unexpected error message")
			} else {
				assert.NoError(t, err, "Unexpected error")
			}
		})
	}
}
