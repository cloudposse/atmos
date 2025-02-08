package hooks

import (
	"fmt"

	log "github.com/charmbracelet/log"
	e "github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/schema"
	u "github.com/cloudposse/atmos/pkg/utils"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v2"
)

type Hooks struct {
	config *schema.AtmosConfiguration
	info   *schema.ConfigAndStacksInfo
	items  map[string]Hook
}

func (h Hooks) HasHooks() bool {
	return len(h.items) > 0
}

func GetHooks(atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo) (*Hooks, error) {
	sections, err := e.ExecuteDescribeComponent(info.ComponentFromArg, info.Stack, true, true, []string{})
	if err != nil {
		return &Hooks{}, fmt.Errorf("failed to execute describe component: %w", err)
	}

	hooksSection := sections["hooks"].(map[string]any)

	yamlData, err := yaml.Marshal(hooksSection)
	if err != nil {
		return &Hooks{}, fmt.Errorf("failed to marshal hooksSection: %w", err)
	}

	var items map[string]Hook
	err = yaml.Unmarshal(yamlData, &items)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal to Hooks: %w", err)
	}

	hooks := Hooks{
		config: atmosConfig,
		info:   info,
		items:  items,
	}

	return &hooks, nil
}

func (h Hooks) RunAll(event HookEvent, atmosConfig *schema.AtmosConfiguration, info *schema.ConfigAndStacksInfo, cmd *cobra.Command, args []string) error {
	log.Debug("running hooks", "count", len(h.items))

	for _, hook := range h.items {
		switch hook.Command {
		case "store":
			storeCmd := &StoreCommand{
				Name:        "store",
				atmosConfig: atmosConfig,
				info:        info,
			}
			err := storeCmd.RunE(&hook, event, cmd, args)
			if err != nil {
				u.LogErrorAndExit(err)
			}
		}
	}
	return nil
}

func (h Hooks) ConvertToHooks(input map[string]any) (Hooks, error) {
	return Hooks{}, nil
}
