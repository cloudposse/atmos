package exec

import (
	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

type DescribeWorkflowsArgs struct {
	Format     string
	OutputType string
	Query      string
}

//go:generate go run go.uber.org/mock/mockgen@latest -source=$GOFILE -destination=mock_$GOFILE -package=$GOPACKAGE
type DescribeWorkflowsExec interface {
	Execute(*schema.AtmosConfiguration, *DescribeWorkflowsArgs) error
}

type describeWorkflowsExec struct {
	printOrWriteToFile       func(atmosConfig *schema.AtmosConfiguration, format, file string, data any) error
	IsTTYSupportForStdout    func() bool
	pagerCreator             pager.PageCreator
	executeDescribeWorkflows func(atmosConfig schema.AtmosConfiguration) ([]schema.DescribeWorkflowsItem, map[string][]string, map[string]schema.WorkflowManifest, error)
}

func NewDescribeWorkflowsExec() DescribeWorkflowsExec {
	defer perf.Track(nil, "exec.NewDescribeWorkflowsExec")()

	return &describeWorkflowsExec{
		printOrWriteToFile:       printOrWriteToFile,
		IsTTYSupportForStdout:    term.IsTTYSupportForStdout,
		executeDescribeWorkflows: ExecuteDescribeWorkflows,
		pagerCreator:             pager.New(),
	}
}

// ExecuteDescribeWorkflowsCmd executes `atmos describe workflows` CLI command.
func (d *describeWorkflowsExec) Execute(atmosConfig *schema.AtmosConfiguration, describeWorkflowsArgs *DescribeWorkflowsArgs) error {
	defer perf.Track(atmosConfig, "exec.DescribeWorkflowsExec.Execute")()

	outputType := describeWorkflowsArgs.OutputType
	query := describeWorkflowsArgs.Query
	format := describeWorkflowsArgs.Format

	describeWorkflowsList, describeWorkflowsMap, describeWorkflowsAll, err := d.executeDescribeWorkflows(*atmosConfig)
	if err != nil {
		return err
	}

	var res any

	if outputType == "list" {
		res = describeWorkflowsList
	} else if outputType == "map" {
		res = describeWorkflowsMap
	} else {
		res = describeWorkflowsAll
	}

	if query != "" {
		res, err = u.EvaluateYqExpression(atmosConfig, res, query)
		if err != nil {
			return err
		}
	}

	err = viewWithScroll(&viewWithScrollProps{
		pageCreator:           d.pagerCreator,
		isTTYSupportForStdout: d.IsTTYSupportForStdout,
		printOrWriteToFile:    d.printOrWriteToFile,
		atmosConfig:           atmosConfig,
		displayName:           "Describe Workflows",
		format:                format,
		file:                  "",
		res:                   res,
	})
	if err != nil {
		return err
	}

	return nil
}
