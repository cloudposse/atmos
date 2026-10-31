package config

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"go.yaml.in/yaml/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	artifactconfig "github.com/cloudposse/atmos/pkg/artifact/config"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// artifactsSectionKey is the top-level atmos.yaml key of the artifact configuration.
const artifactsSectionKey = "artifacts"

// artifactOrigins records, per setting, the configuration file that supplied the final value. It is
// tracked independently of the merged map because deep merging fuses the fields of several files
// into one repository: kind from one file and options.path from another must each keep their own
// declaring file. The last source that declares a setting wins, exactly like the value merge.
type artifactOrigins struct {
	repoPath   map[string]schema.ArtifactConfigOrigin
	repoKeys   map[string]schema.ArtifactConfigOrigin
	globalKeys schema.ArtifactConfigOrigin
}

func newArtifactOrigins() *artifactOrigins {
	return &artifactOrigins{
		repoPath: make(map[string]schema.ArtifactConfigOrigin),
		repoKeys: make(map[string]schema.ArtifactConfigOrigin),
	}
}

// resolveArtifactsConfig rebuilds atmosConfig.Artifacts from the retained configuration sources.
//
// Viper cannot be used for this section: it lowercases keys (repository aliases, label keys, and
// S3 tag keys are case-sensitive), splits keys on dots, and, once files are merged, no longer knows
// which file supplied a value. So the section is parsed from each retained source in merge order,
// merged as raw data, and decoded strictly once. While doing so the declaring file of each
// path-valued setting is recorded and relative local paths are anchored to that file's directory
// before the origin is lost. A relative path declared only by a remote import is left unresolved
// with its origin marked remote; artifactconfig validation then requires a local or absolute
// override. The key_pattern templates are plain strings here and are never rendered.
func resolveArtifactsConfig(v *viper.Viper, atmosConfig *schema.AtmosConfiguration) error {
	defer perf.Track(atmosConfig, "config.resolveArtifactsConfig")()

	merged := map[string]any{}
	origins := newArtifactOrigins()
	declared := false
	for _, source := range collectConfigSourcesForCasePreservation(v, v.ConfigFileUsed()) {
		section, ok, err := artifactsSectionFromSource(source)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		declared = true
		origins.record(section, source)
		combined, err := merge.MergeWithOptions(nil, []map[string]any{merged, section}, false, false)
		if err != nil {
			return fmt.Errorf("%w: merge artifacts from %s: %w", errUtils.ErrArtifactConfigInvalid, source.path, err)
		}
		merged = combined
	}
	if !declared {
		return nil
	}
	cfg, err := artifactconfig.Decode(merged)
	if err != nil {
		return err
	}
	origins.apply(&cfg)
	atmosConfig.Artifacts = cfg
	return nil
}

// artifactsSectionFromSource parses the `artifacts` section of one retained source with its
// original key spelling. The boolean is false when the source declares no such section.
func artifactsSectionFromSource(source mergedConfigSource) (map[string]any, bool, error) {
	var document yaml.Node
	if err := yaml.Unmarshal([]byte(source.content), &document); err != nil {
		return nil, false, fmt.Errorf("%w: parse %s: %w", errUtils.ErrArtifactConfigInvalid, source.path, err)
	}
	node := policyMappingValue(&document, artifactsSectionKey)
	if node == nil {
		return nil, false, nil
	}
	value, err := decodeNodeWithYamlFunctionsForFile(node, source.path)
	if err != nil {
		return nil, false, fmt.Errorf("%w: artifacts in %s: %w", errUtils.ErrArtifactConfigInvalid, source.path, err)
	}
	if value == nil {
		return nil, false, nil
	}
	section, ok := value.(map[string]any)
	if !ok {
		return nil, false, fmt.Errorf("%w: artifacts in %s must be a mapping, got %T", errUtils.ErrArtifactConfigInvalid, source.path, value)
	}
	return section, true, nil
}

// record notes which source declares each path-valued setting present in section.
func (o *artifactOrigins) record(section map[string]any, source mergedConfigSource) {
	origin := artifactconfig.NewOrigin(source.path, source.ephemeral)
	if keysDeclared(mapOf(mapValue(section, "verification"))) {
		o.globalKeys = origin
	}
	repos, _ := mapValue(section, "repositories").(map[string]any)
	for alias, raw := range repos {
		repo, ok := raw.(map[string]any)
		if !ok {
			continue
		}
		if _, isString := mapValue(mapOf(mapValue(repo, "options")), artifact.OptionPath).(string); isString {
			o.repoPath[alias] = origin
		}
		if keysDeclared(mapOf(mapValue(repo, "verification"))) {
			o.repoKeys[alias] = origin
		}
	}
}

// apply stores the origins on the decoded configuration and anchors relative local paths.
func (o *artifactOrigins) apply(cfg *schema.ArtifactConfig) {
	if cfg.Verification != nil {
		cfg.Verification.KeysOrigin = o.globalKeys
		cfg.Verification.TrustedKeys = anchorKeys(cfg.Verification.TrustedKeys, o.globalKeys)
	}
	for alias := range cfg.Repositories {
		repo := cfg.Repositories[alias]
		repo.PathOrigin = o.repoPath[alias]
		repo.Options = anchorPathOptions(&repo)
		if repo.Verification != nil {
			repo.Verification.KeysOrigin = o.repoKeys[alias]
			repo.Verification.TrustedKeys = anchorKeys(repo.Verification.TrustedKeys, o.repoKeys[alias])
		}
		cfg.Repositories[alias] = repo
	}
}

// anchorPathOptions returns the repository options with relative local destination paths made
// absolute against the declaring file. Options are copied, never mutated in place. A path declared
// by a remote import, or with no known declaring file, stays as written so validation can reject it.
func anchorPathOptions(repo *schema.ArtifactRepositoryConfig) map[string]any {
	desc, ok := artifact.LookupRepositoryKind(repo.Kind)
	if !ok || len(desc.PathOptions) == 0 || repo.PathOrigin.Remote || repo.PathOrigin.Dir == "" {
		return repo.Options
	}
	out := make(map[string]any, len(repo.Options))
	for key, value := range repo.Options {
		out[key] = value
	}
	for _, option := range desc.PathOptions {
		anchorOption(out, option, repo.PathOrigin)
	}
	return out
}

// anchorOption rewrites one relative path option of options in place to its anchored form.
func anchorOption(options map[string]any, option string, origin schema.ArtifactConfigOrigin) {
	path, isString := options[option].(string)
	if !isString || path == "" || filepath.IsAbs(path) {
		return
	}
	if anchored, err := artifactconfig.AnchorPath(origin, path); err == nil {
		options[option] = anchored
	}
}

// anchorKeys makes relative trusted-key paths absolute against the file that declared them.
func anchorKeys(keys []string, origin schema.ArtifactConfigOrigin) []string {
	if len(keys) == 0 || origin.Remote || origin.Dir == "" {
		return keys
	}
	out := make([]string, len(keys))
	for i, key := range keys {
		out[i] = key
		if key == "" || filepath.IsAbs(key) {
			continue
		}
		if anchored, err := artifactconfig.AnchorPath(origin, key); err == nil {
			out[i] = anchored
		}
	}
	return out
}

// keysDeclared reports whether a verification mapping declares a trusted_keys list.
func keysDeclared(verification map[string]any) bool {
	_, ok := mapValue(verification, "trusted_keys").([]any)
	return ok
}

// mapValue looks up a key case-insensitively, matching how the loader decodes struct fields.
func mapValue(m map[string]any, key string) any {
	if value, ok := m[key]; ok {
		return value
	}
	for candidate, value := range m {
		if strings.EqualFold(candidate, key) {
			return value
		}
	}
	return nil
}

// mapOf returns value as a mapping, or nil when it is not one.
func mapOf(value any) map[string]any {
	m, _ := value.(map[string]any)
	return m
}
