package plugin

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/flags"
	"github.com/cloudposse/atmos/pkg/plugin/catalog"
	"github.com/cloudposse/atmos/pkg/plugin/discovery"
	"github.com/cloudposse/atmos/pkg/plugin/install"
	"github.com/cloudposse/atmos/pkg/plugin/manage"
	pluginoutput "github.com/cloudposse/atmos/pkg/plugin/output"
)

func searchCommand() *cobra.Command {
	parser := newOutputParser(
		flags.WithStringFlag("registry", "", "", "Search only this configured discovery source"),
		flags.WithStringFlag("capability", "", "", "Filter by extension capability"),
		flags.WithStringFlag("runtime", "", "", "Filter by runtime"),
		flags.WithBoolFlag("refresh", "", false, "Refresh discovery metadata even when the cache is fresh"),
	)
	command := &cobra.Command{Use: "search [query]", Short: "Search cached OCI repository indexes and plugin catalogs", Args: cobra.MaximumNArgs(1)}
	registerOutputFlags(command, parser)
	command.RunE = func(cmd *cobra.Command, args []string) error {
		values, err := parseOutputFlags(cmd, parser)
		if err != nil {
			return err
		}
		manager, err := newManager(cmd)
		if err != nil {
			return err
		}
		filter := catalog.Filter{Query: strings.Join(args, " "), Capability: values.GetString("capability"), Runtime: values.GetString("runtime")}
		result, err := manager.Search(cmd.Context(), discovery.Request{Registry: values.GetString("registry"), Filter: filter, Refresh: values.GetBool("refresh")})
		if err != nil {
			return err
		}
		return pluginoutput.Search(result, values.GetString("format"))
	}
	return command
}

type searchResult = pluginoutput.SearchResult

func searchSources(cmd *cobra.Command, manager *manage.Manager, selected string, filter catalog.Filter) ([]searchResult, error) {
	return manager.Search(cmd.Context(), discovery.Request{Registry: selected, Filter: filter})
}

func resolveDiscoveredPackage(cmd *cobra.Command, manager *manage.Manager, source, constraint string) (*catalog.Entry, *catalog.Version, error) {
	registry, packageName, err := install.ParseSelector(source)
	if err != nil {
		return nil, nil, err
	}
	parts := []string{registry, packageName}
	results, err := searchSources(cmd, manager, parts[0], catalog.Filter{})
	if err != nil {
		return nil, nil, err
	}
	var matched *catalog.Entry
	for itemIndex := range results {
		result := results[itemIndex]
		short := strings.SplitN(result.Name, "/", 2)
		if result.Name != source && result.Name != parts[1] && (len(short) != 2 || short[1] != parts[1]) {
			continue
		}
		if matched != nil {
			return nil, nil, errUtils.Build(fmt.Errorf("%w: %s matches several plugins", errUtils.ErrPluginInvalidConfig, source)).
				WithHintf("Use the full name (`%s/<namespace>/%s`) or an `oci://` source.", registry, packageName).
				Err()
		}
		matched = &result.Entry
	}
	if matched == nil {
		return nil, nil, errUtils.Build(fmt.Errorf("%w: %s was not found in registry %s", errUtils.ErrPluginNotInstalled, packageName, registry)).
			WithHint("Run `atmos plugin search` to list available plugins, or install from an `oci://` source directly.").
			Err()
	}
	selected, err := catalog.Select(matched, constraint, manager.Installer.Platform)
	return matched, selected, err
}
