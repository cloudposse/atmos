package version

import (
	"github.com/spf13/cobra"

	"github.com/cloudposse/atmos/cmd/internal"
	"github.com/cloudposse/atmos/internal/exec"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

var (
	checkFlag     bool
	versionFormat string
	// AtmosConfigPtr will be set by SetAtmosConfig before command execution.
	atmosConfigPtr *schema.AtmosConfiguration
)

// SetAtmosConfig sets the Atmos configuration for the version command.
// This is called from root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

// versionCmd represents the version command.
var versionCmd = &cobra.Command{
	Use:     "version",
	Short:   "Display the version of Atmos you are running and check for updates",
	Long:    `This command shows the version of the Atmos CLI you are currently running and checks if a newer version is available. Use this command to verify your installation and ensure you are up to date.`,
	Example: "atmos version",
	Args:    cobra.NoArgs,
	RunE: func(c *cobra.Command, args []string) error {
		defer perf.Track(nil, "version.RunE")()

		return exec.NewVersionExec(atmosConfigPtr).Execute(checkFlag, versionFormat)
	},
}

func init() {
	versionCmd.Flags().BoolVarP(&checkFlag, "check", "c", false, "Run additional checks after displaying version info")
	versionCmd.Flags().StringVar(&versionFormat, "format", "", "Specify the output format")

	// Register this command with the registry.
	// This happens during package initialization via blank import in cmd/root.go.
	internal.Register(&VersionCommandProvider{})
}

// VersionCommandProvider implements the CommandProvider interface.
type VersionCommandProvider struct{}

// GetCommand returns the version command.
func (v *VersionCommandProvider) GetCommand() *cobra.Command {
	return versionCmd
}

// GetName returns the command name.
func (v *VersionCommandProvider) GetName() string {
	return "version"
}

// GetGroup returns the command group for help organization.
func (v *VersionCommandProvider) GetGroup() string {
	return "Other Commands"
}
