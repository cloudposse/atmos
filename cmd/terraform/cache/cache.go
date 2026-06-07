// Package cache provides the `atmos terraform cache` command group for inspecting
// and managing the Terraform registry cache.
package cache

import (
	"encoding/json"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/data"
	"github.com/cloudposse/atmos/pkg/schema"
	tfcache "github.com/cloudposse/atmos/pkg/terraform/cache"
)

// atmosConfigPtr is set by SetAtmosConfig before command execution.
var atmosConfigPtr *schema.AtmosConfiguration

// SetAtmosConfig sets the Atmos configuration for the cache command. Called from
// root.go after atmosConfig is initialized.
func SetAtmosConfig(config *schema.AtmosConfiguration) {
	atmosConfigPtr = config
}

var cacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage the Terraform registry cache",
	Long: `List, inspect, prune, and delete providers and modules cached by the Terraform registry cache.

The registry cache is the ephemeral proxy that intercepts traffic to provider and
module registries and stores the fetched artifacts on disk. It is distinct from
Terraform's own provider plugin cache (the 'plugin_cache' setting / TF_PLUGIN_CACHE_DIR),
which is a separate caching layer that Terraform manages itself.`,
}

func init() {
	cacheCmd.Annotations = map[string]string{"experimental": "true"}
	cacheCmd.AddCommand(listCmd)
	cacheCmd.AddCommand(statsCmd)
	cacheCmd.AddCommand(pruneCmd)
	cacheCmd.AddCommand(deleteCmd)
	cacheCmd.AddCommand(mirrorCmd)
	cacheCmd.AddCommand(trustCmd)
	cacheCmd.AddCommand(untrustCmd)
}

// GetCacheCommand returns the cache command for parent registration.
func GetCacheCommand() *cobra.Command {
	return cacheCmd
}

// resolveCacheRoot loads the Atmos config and resolves the cache root.
func resolveCacheRoot() (string, error) {
	configInfo := schema.ConfigAndStacksInfo{}
	atmosConfig, err := cfg.InitCliConfig(configInfo, false)
	if err != nil {
		return "", err
	}
	return tfcache.ResolveRoot(&atmosConfig)
}

// renderFormatted writes v as JSON or YAML for machine-readable formats, or calls
// renderTable for the default human-readable table. Shared by the list and stats
// subcommands so their output handling stays consistent.
func renderFormatted(format string, v any, renderTable func()) error {
	switch format {
	case "json":
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return err
		}
		_ = data.Writeln(string(b))
	case "yaml":
		b, err := yaml.Marshal(v)
		if err != nil {
			return err
		}
		_ = data.Write(string(b))
	default:
		renderTable()
	}
	return nil
}
