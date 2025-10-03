package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
)

const describeConfigTitle = "Atmos Config"

type DescribeConfigFormatError struct {
	format string
}

func (e DescribeConfigFormatError) Error() string {
	return fmt.Sprintf("invalid 'format': %s", e.format)
}

var ErrTTYNotSupported = fmt.Errorf("tty not supported for this command")

type describeConfigExec struct {
	atmosConfig           *schema.AtmosConfiguration
	pageCreator           pager.PageCreator
	printOrWriteToFile    func(atmosConfig *schema.AtmosConfiguration, format string, file string, data any) error
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
	defer perf.Track(nil, "exec.DescribeConfigExec.ExecuteDescribeConfigCmd")()

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

	if d.atmosConfig.Settings.Terminal.IsPagerEnabled() {
		err = d.viewConfig(format, res)
		switch err.(type) {
		case DescribeConfigFormatError:
			return err
		case nil:
			return nil
		default:
			log.Debug("Failed to use pager")
		}
	}
	return d.printOrWriteToFile(d.atmosConfig, format, output, res)
}

func (d *describeConfigExec) viewConfig(format string, data *schema.AtmosConfiguration) error {
	if !d.IsTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	var content string
	var err error
	switch format {
	case "yaml":
		content, err = u.GetHighlightedYAML(data, data)
		if err != nil {
			return err
		}
	case "json":
		content, err = u.GetHighlightedJSON(data, data)
		if err != nil {
			return err
		}
	default:
		return DescribeConfigFormatError{
			format,
		}
	}
	if err := d.pageCreator.Run(describeConfigTitle, content); err != nil {
		return err
	}
	return nil
}
