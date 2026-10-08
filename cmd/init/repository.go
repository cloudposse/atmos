package initcmd

import (
	"net/url"
	"strings"

	"github.com/hashicorp/go-getter"
	"github.com/spf13/viper"

	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/generator/source"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui/spinner"
	"github.com/cloudposse/atmos/pkg/vendor"
)

// applyInitDefaults places configured values below command flags and environment
// overrides. Viper's ordinary flag/env precedence remains authoritative.
func applyInitDefaults(v *viper.Viper) (*schema.AtmosConfiguration, error) {
	progress := spinner.New("Loading configuration")
	progress.Start()
	defer progress.Stop()
	atmosConfig, err := cfg.InitCliConfig(schema.ConfigAndStacksInfo{}, false)
	if err != nil {
		return nil, err
	}
	bindInitDefaults(v, atmosConfig.Init)
	return &atmosConfig, nil
}

func bindInitDefaults(v *viper.Viper, defaults schema.InitConfig) {
	v.SetDefault("ref", defaults.Ref)
	v.SetDefault("depth", defaults.Depth)
	gitEnabled := true
	if defaults.Git != nil {
		gitEnabled = *defaults.Git
	}
	v.SetDefault("git", gitEnabled)
}

// expandInitRepository resolves relative directory paths, such as examples/demo,
// against the configured repository. Bare names remain template identifiers.
func expandInitRepository(opts *initOptions) error {
	if source.IsTemplateSource(opts.templateName) || !strings.Contains(opts.templateName, "/") {
		return nil
	}
	atmosConfig, err := initSourceConfig(opts, "")
	if err != nil {
		return err
	}
	opts.templateName, err = repositoryDirectorySource(atmosConfig.Init.Repository, opts.templateName, opts.ref)
	return err
}

func repositoryDirectorySource(repository, directory, ref string) (string, error) {
	if repository == "" {
		repository = schema.DefaultInitRepository
	}
	if !vendor.IsGitURI(repository) {
		return "", copyOptionError("init.repository must be a Git repository URL")
	}
	root, subdir := getter.SourceDirSubdir(repository)
	if subdir != "" && subdir != "." {
		return "", copyOptionError("init.repository must name a repository, without a subdirectory")
	}
	base, repoQuery, _ := strings.Cut(root, "?")
	dirPath, dirQuery, _ := strings.Cut(directory, "?")
	query, err := url.ParseQuery(repoQuery)
	if err != nil {
		return "", err
	}
	explicit, err := url.ParseQuery(dirQuery)
	if err != nil {
		return "", err
	}
	for key, values := range explicit {
		query[key] = values
	}
	result := strings.TrimRight(base, "/") + "//" + dirPath
	if len(query) > 0 {
		result += "?" + query.Encode()
	}
	result = source.WithRef(result, url.QueryEscape(ref))
	// Preserve the official alias's main default. Custom repositories use
	// their own default branch unless the repository/source or --ref pins one.
	if repository == schema.DefaultInitRepository {
		result = source.WithRef(result, "main")
	}
	return result, nil
}
