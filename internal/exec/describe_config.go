package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"

	u "github.com/cloudposse/atmos/pkg/utils"
)

const describeConfigTitle = "Atmos Config"

type ErrInvalidFormat struct {
	format string
}

func (e ErrInvalidFormat) Error() string {
	return fmt.Sprintf("invalid 'format': %s", e.format)
}

var ErrTTYNotSupported = fmt.Errorf("tty not supported for this command")

type describeConfigExec struct {
	atmosConfig           *schema.AtmosConfiguration
	pageCreator           pager.PageCreator
	printOrWriteToFile    func(format string, file string, data any) error
	IsTTYSupportForStdout func() bool
}

func NewDescribeConfig(atmosConfig *schema.AtmosConfiguration) *describeConfigExec {
	return &describeConfigExec{
		atmosConfig:           atmosConfig,
		pageCreator:           pager.New(),
		printOrWriteToFile:    printOrWriteToFile,
		IsTTYSupportForStdout: term.IsTTYSupportForStdout,
	}
}

// ExecuteDescribeConfigCmd executes `describe config` command.
func (d *describeConfigExec) ExecuteDescribeConfigCmd(query, format, output string) error {
	var res *schema.AtmosConfiguration
	var err error
	if query != "" {
		res, err = u.EvaluateYqExpressionWithType[schema.AtmosConfiguration](d.atmosConfig, *d.atmosConfig, query)
		if err != nil {
			return err
		}
	} else {
		res = d.atmosConfig
	}
	err = d.viewConfig(format, res)

	switch err.(type) {
	case ErrInvalidFormat:
		return err
	case nil:
	default:
		fmt.Println(err)
		err = d.printOrWriteToFile(format, output, res)
		if err != nil {
			return err
		}
	}

	return nil
}

func (d *describeConfigExec) viewConfig(format string, data *schema.AtmosConfiguration) error {
	if !d.IsTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	var content string
	var err error
	switch format {
	case "yaml":
		content, err = u.GetAtmosConfigYAML(data)
		if err != nil {
			return err
		}
	case "json":
		content, err = u.GetAtmosConfigJSON(data)
		if err != nil {
			return err
		}
	default:
		return ErrInvalidFormat{
			format,
		}
	}
	if err := d.pageCreator.Run(describeConfigTitle, content); err != nil {
		return err
	}
	return nil
}
