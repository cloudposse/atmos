package exec

import (
	"fmt"

	"github.com/cloudposse/atmos/internal/tui/templates/term"
	"github.com/cloudposse/atmos/pkg/pager"
	"github.com/cloudposse/atmos/pkg/schema"

	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"

	cfg "github.com/cloudposse/atmos/pkg/config"
)

// ExecuteDescribeConfigCmd executes `describe config` command
func ExecuteDescribeConfigCmd(cmd *cobra.Command, args []string) error {
	flags := cmd.Flags()

	format, err := flags.GetString("format")
	if err != nil {
		return err
	}

	query, err := flags.GetString("query")
	if err != nil {
		return err
	}

	info, err := ProcessCommandLineArgs("", cmd, args, nil)
	if err != nil {
		return err
	}

	atmosConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	var res *schema.AtmosConfiguration

	if query != "" {
		res, err = u.EvaluateYqExpressionWithType[schema.AtmosConfiguration](&atmosConfig, atmosConfig, query)
		if err != nil {
			return err
		}
	} else {
		res = &atmosConfig
	}

	err = ViewConfig(format, res)
	if err != nil {
		err = printOrWriteToFile(format, "", res)
		if err != nil {
			return err
		}
	}
	return nil
}

var ErrTTYNotSupported = fmt.Errorf("tty not supported for this command")

func ViewConfig(format string, data *schema.AtmosConfiguration) error {
	if !term.IsTTYSupportForStdout() {
		return ErrTTYNotSupported
	}
	title := "Atmos Config"
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
		return fmt.Errorf("invalid 'format': %s", format)
	}

	if err := pager.New(title, content).Run(); err != nil {
		return err
	}
	return nil
}
