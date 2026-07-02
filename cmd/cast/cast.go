package cast

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/asciicast"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/flags/compat"
)

var castCmd = &cobra.Command{
	Use:   "cast",
	Short: "Play and render Atmos asciicast recordings",
}

var playCmd = &cobra.Command{
	Use:   "play <input.cast>",
	Short: "Play an asciicast recording in the terminal",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return asciicast.Play(args[0], os.Stdout)
	},
}

var renderCmd = &cobra.Command{
	Use:   "render <input.cast>",
	Short: "Render an asciicast recording to GIF or MP4",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		gif, _ := cmd.Flags().GetString("gif")
		mp4, _ := cmd.Flags().GetString("mp4")
		if gif == "" && mp4 == "" {
			return fmt.Errorf("%w", errUtils.ErrMissingRenderOutput)
		}
		return asciicast.Render(args[0], asciicast.RenderOptions{GIF: gif, MP4: mp4})
	},
}

func init() {
	renderCmd.Flags().String("gif", "", "Write animated GIF output to this path")
	renderCmd.Flags().String("mp4", "", "Write MP4 output to this path")
	castCmd.AddCommand(playCmd, renderCmd)
	internal.Register(&CommandProvider{})
}

type CommandProvider struct{}

func (p *CommandProvider) GetCommand() *cobra.Command {
	return castCmd
}

func (p *CommandProvider) GetName() string {
	return "cast"
}

func (p *CommandProvider) GetGroup() string {
	return "Other Commands"
}

func (p *CommandProvider) GetFlagsBuilder() flags.Builder {
	return nil
}

func (p *CommandProvider) GetPositionalArgsBuilder() *flags.PositionalArgsBuilder {
	return nil
}

func (p *CommandProvider) GetCompatibilityFlags() map[string]compat.CompatibilityFlag {
	return nil
}

func (p *CommandProvider) GetAliases() []internal.CommandAlias {
	return nil
}

func (p *CommandProvider) IsExperimental() bool {
	return false
}
