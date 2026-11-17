package docs

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	errUtils "github.com/cloudposse/atmos/errors"
	cfg "github.com/cloudposse/atmos/pkg/config"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ExecuteGenerate implements 'atmos docs generate <doc-type>': it loads atmos.yaml honoring the
// global --base-path, --config, --config-path, and --profile flags, selects the
// docs.generate.<doc-type> entry, and generates the document with lifecycle hooks.
func ExecuteGenerate(cmd *cobra.Command, args []string) error {
	defer perf.Track(nil, "docs.ExecuteGenerate")()

	if len(args) == 0 {
		return errUtils.ErrMissingDocType
	}
	// Parsing merges the parent's persistent flags (--base-path, --config, ...) into cmd.Flags().
	if err := cmd.ParseFlags(args); err != nil && !errors.Is(err, pflag.ErrHelp) {
		return err
	}
	info, err := globalFlagsInfo(cmd)
	if err != nil {
		return err
	}
	rootConfig, err := cfg.InitCliConfig(info, false)
	if err != nil {
		return err
	}

	// Retrieve the <doc-type> generation config from atmos.yaml.
	// The target directory is taken from docs.generate.<doc-type>.base-dir.
	docsGenerate, ok := rootConfig.Docs.Generate[args[0]]
	if !ok {
		return errUtils.Build(fmt.Errorf("%w: %q", errUtils.ErrNoDocsGenerateEntry, args[0])).
			WithHint(configuredDocTypesHint(rootConfig.Docs.Generate)).
			Err()
	}

	if len(docsGenerate.Input) == 0 {
		log.Debug("docs.generate['<doc-type>'].input is empty in atmos.yaml")
	}
	if len(docsGenerate.Template) == 0 {
		log.Debug("No docs.generate['<doc-type>'].template' defined, generating minimal <doc-type>.")
	}

	// Generate a single document in the targetDir using the default renderer.
	return GenerateWithHooks(&rootConfig, cmd, args[0], &docsGenerate)
}

// globalFlagsInfo carries the persistent configuration-selection flags into the config loader.
// Profiles fall back to ATMOS_PROFILE inside the loader when the flag is unset.
func globalFlagsInfo(cmd *cobra.Command) (schema.ConfigAndStacksInfo, error) {
	var info schema.ConfigAndStacksInfo
	var err error
	if info.BasePath, err = cmd.Flags().GetString("base-path"); err != nil {
		return info, err
	}
	if info.AtmosConfigFilesFromArg, err = cmd.Flags().GetStringSlice("config"); err != nil {
		return info, err
	}
	if info.AtmosConfigDirsFromArg, err = cmd.Flags().GetStringSlice("config-path"); err != nil {
		return info, err
	}
	if info.ProfilesFromArg, err = cmd.Flags().GetStringSlice("profile"); err != nil {
		return info, err
	}
	return info, nil
}

// configuredDocTypesHint lists the configured docs.generate types in name order.
func configuredDocTypesHint(configured map[string]schema.DocsGenerate) string {
	if len(configured) == 0 {
		return "No docs.generate entries are configured in atmos.yaml"
	}
	names := make([]string, 0, len(configured))
	for name := range configured {
		names = append(names, name)
	}
	slices.Sort(names)
	return "Configured docs.generate types: " + strings.Join(names, ", ")
}
