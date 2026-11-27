package cloudformation

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	uiutils "github.com/cloudposse/atmos/internal/tui/utils"
	"github.com/cloudposse/atmos/pkg/schema"
)

func selectInitTemplate(cmd *cobra.Command, args []string, interactive bool) (string, error) {
	if len(args) > 0 {
		return templatePath(args[0])
	}
	if !interactive {
		return "", fmt.Errorf("%w: provide a template path, for example: atmos aws cfn init templates/queue.yaml", errUtils.ErrMissingInput)
	}
	paths, err := discoverTemplates(cmd.Context(), os.DirFS("."))
	if err != nil {
		return "", err
	}
	if len(paths) == 0 {
		return "", fmt.Errorf("%w: no CloudFormation templates found under the current directory; pass a template path explicitly", errUtils.ErrMissingInput)
	}
	var selected string
	picker := huh.NewSelect[string]().Title("CloudFormation template").
		Description("Choose the template to register.").Options(huh.NewOptions(paths...)...).Value(&selected)
	if err := promptInitFields(cmd, picker); err != nil {
		return "", err
	}
	return templatePath(selected)
}

func initDeployment(cmd *cobra.Command, path string, names []string, info *schema.ConfigAndStacksInfo, interactive bool) (templateSource, error) {
	base := filepath.Base(path)
	suggestedNames := strings.TrimSuffix(base, filepath.Ext(base))
	var fields []huh.Field
	promptNames := interactive && len(names) == 0
	if promptNames {
		fields = append(fields, huh.NewInput().Title("AWS stack name(s)").
			Description("For multiple deployments, enter names separated by commas.").
			Value(&suggestedNames).Validate(func(value string) error {
			_, err := initNames(path, splitInitNames(value))
			return err
		}))
	}
	if info.Stack == "" {
		info.Stack = "dev"
		if interactive {
			fields = append(fields, huh.NewInput().Title("Atmos stack").
				Description("The Atmos stack groups these deployments.").Value(&info.Stack).Validate(validateInitStack))
		}
	}
	if len(fields) > 0 {
		if err := promptInitFields(cmd, fields...); err != nil {
			return templateSource{}, err
		}
	}
	if promptNames {
		names = splitInitNames(suggestedNames)
	}
	distinct, err := initNames(path, names)
	if err != nil {
		return templateSource{}, err
	}
	return templateSource{path, distinct}, validateInitStack(info.Stack)
}

func splitInitNames(value string) []string {
	names := strings.Split(value, ",")
	for i := range names {
		names[i] = strings.TrimSpace(names[i])
	}
	return names
}

func validateInitStack(value string) error {
	if !manifestName.MatchString(value) {
		return fmt.Errorf("%w: invalid Atmos stack name %q: use letters, numbers, underscores or hyphens, starting with a letter or number", errUtils.ErrInvalidFlagValue, value)
	}
	return nil
}

var runInitForm = func(cmd *cobra.Command, form *huh.Form) error {
	return form.WithInput(cmd.InOrStdin()).WithOutput(cmd.ErrOrStderr()).RunWithContext(cmd.Context())
}

func promptInitFields(cmd *cobra.Command, fields ...huh.Field) error {
	keys := huh.NewDefaultKeyMap()
	keys.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc"), key.WithHelp("ctrl+c/esc", "cancel"))
	form := huh.NewForm(huh.NewGroup(fields...)).WithTheme(uiutils.NewAtmosHuhTheme()).WithKeyMap(keys)
	err := runInitForm(cmd, form)
	if errors.Is(err, huh.ErrUserAborted) {
		return errUtils.ErrUserAborted
	}
	return err
}
