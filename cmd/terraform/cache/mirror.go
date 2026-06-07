package cache

import (
	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/perf"
	tfmirror "github.com/cloudposse/atmos/pkg/terraform/mirror"
)

// mirrorRun is the mirror entry point, indirected through a package variable so
// tests can substitute a fake without driving a real terraform subprocess.
var mirrorRun = tfmirror.Run

var mirrorParser *flags.StandardParser

var mirrorCmd = &cobra.Command{
	Use: "mirror [component] -s <stack>",
	// `warm` is the term the PRD/North Star uses for eager pre-seeding; keep it as an
	// alias so it is discoverable, with `mirror` canonical (it wraps `providers mirror`).
	Aliases: []string{"warm"},
	Short:   "Pre-seed the cache with providers for multiple platforms",
	Long: `Eagerly mirror the providers required by one or more components into the cache
for multiple platforms, using 'terraform/tofu providers mirror'. Platforms come
from components.terraform.cache.mirror.platforms, overridable with --platform;
with no platforms configured, the current host platform is used.

Target components like 'atmos terraform plan': a single component with --stack,
or a fleet with --all, --components, or --query (optionally scoped with --stack).
With --all and no --stack, every component in every stack is mirrored — the
foundation for an air-gapped bundle.

The mirror writes the canonical filesystem_mirror layout the lazy proxy already
serves, so the same cache directory works lazily (proxy), eagerly (mirror), and
offline (filesystem_mirror).`,
	Example: `  atmos terraform cache mirror vpc -s plat-ue2-prod
  atmos terraform cache mirror vpc -s plat-ue2-prod --platform=linux_amd64 --platform=darwin_arm64
  atmos terraform cache mirror --all
  atmos terraform cache mirror --all -s plat-ue2-prod
  atmos terraform cache mirror --components=vpc,eks -s plat-ue2-prod`,
	Args: cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		defer perf.Track(atmosConfigPtr, "cache.mirror.RunE")()

		v := viper.GetViper()
		if err := mirrorParser.BindFlagsToViper(cmd, v); err != nil {
			return err
		}

		var component string
		if len(args) == 1 {
			component = args[0]
		}

		// stack, components, and query are persistent terraform flags inherited from
		// the parent command (see cmd/terraform/terraform.go); read from Viper here.
		return mirrorRun(tfmirror.Options{
			Component:     component,
			Stack:         v.GetString("stack"),
			PlatformsFlag: v.GetStringSlice("platform"),
			All:           v.GetBool("all"),
			Components:    v.GetStringSlice("components"),
			Query:         v.GetString("query"),
			Format:        v.GetString("format"),
		})
	},
}

func init() {
	mirrorParser = flags.NewStandardParser(
		flags.WithStringSliceFlag("platform", "", nil, "Target platform as os_arch, e.g. linux_amd64 (repeatable); overrides configured platforms"),
		flags.WithBoolFlag("all", "", false, "Mirror every component in every stack (scope with --stack)"),
		flags.WithStringFlag("format", "f", "", "Output format: json or yaml (default: human-readable progress)"),
	)
	mirrorParser.RegisterFlags(mirrorCmd)
	if err := mirrorParser.BindToViper(viper.GetViper()); err != nil {
		panic(err)
	}
}
