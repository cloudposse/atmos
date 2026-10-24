package source

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/cloudposse/atmos/pkg/ai/skills/marketplace"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/version"
)

var skillNamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)

type plugin struct {
	Name   string          `json:"name"`
	Source json.RawMessage `json:"source"`
	Skills json.RawMessage `json:"skills"`
	Strict *bool           `json:"strict"`
}

type marketplaceManifest struct {
	Metadata struct {
		PluginRoot string `json:"pluginRoot"`
	} `json:"metadata"`
	Plugins []plugin `json:"plugins"`
}

func readJSON(path string, target any) error {
	if err := safePath(path); err != nil {
		return err
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

func patternsMatch(patterns []string, name string) (bool, error) {
	for _, pattern := range patterns {
		yes, err := filepath.Match(pattern, name)
		if err != nil {
			return false, err
		}
		if yes {
			return true, nil
		}
	}
	return false, nil
}

func selected(name string, d *schema.AISkillConfig) (bool, error) {
	yes, err := patternsMatch(d.Exclude, name)
	if yes || err != nil {
		return false, err
	}
	if len(d.Include) == 0 {
		return true, nil
	}
	return patternsMatch(d.Include, name)
}

//nolint:revive // Keep sequential validation and I/O errors next to the operation they guard.
func (e *Engine) discover(ctx context.Context, d *schema.AISkillConfig, ref, temp string) (Resolution, []string, error) {
	result := Resolution{Declaration: *d, Version: ref}
	root := filepath.Join(temp, "repo-0")
	repo, err := e.Fetcher.Fetch(ctx, Repository{Source: d.Source, Ref: ref}, root)
	if err != nil {
		return result, nil, err
	}
	result.Repositories = append(result.Repositories, repo)
	roots := []string{root}
	base, err := within(root, d.Subpath)
	if err != nil {
		return result, roots, err
	}
	manifest := filepath.Join(base, ".claude-plugin", "marketplace.json")
	_, statErr := os.Stat(manifest)
	if d.Kind == "marketplace" || (d.Kind == "auto" && statErr == nil) {
		err = e.discoverMarketplace(ctx, &result, &roots, base, temp)
	} else {
		err = discoverSkills(&result, roots, 0, base, "")
	}
	if err != nil {
		return result, roots, err
	}
	if len(result.Skills) == 0 {
		return result, roots, fmt.Errorf("%w: source contains no selected skills", ErrInvalid)
	}
	seen := map[string]bool{}
	filtered := []LockedSkill{}
	for _, skill := range result.Skills {
		yes, err := selected(skill.Name, d)
		if err != nil {
			return result, roots, err
		}
		if !yes {
			continue
		}
		if seen[skill.Name] {
			return result, roots, fmt.Errorf("%w: duplicate skill %s", ErrInvalid, skill.Name)
		}
		seen[skill.Name] = true
		filtered = append(filtered, skill)
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].Name < filtered[j].Name })
	result.Skills = filtered
	return result, roots, nil
}

func (e *Engine) discoverMarketplace(ctx context.Context, result *Resolution, roots *[]string, base, temp string) error {
	var manifest marketplaceManifest
	if err := readJSON(filepath.Join(base, ".claude-plugin", "marketplace.json"), &manifest); err != nil {
		return err
	}
	found := map[string]bool{}
	for _, p := range manifest.Plugins {
		if found[p.Name] {
			return fmt.Errorf("%w: duplicate plugin %s", ErrInvalid, p.Name)
		}
		found[p.Name] = true
		if len(result.Declaration.Plugins) > 0 && !contains(result.Declaration.Plugins, p.Name) {
			continue
		}
		index, pluginRoot, err := e.pluginRoot(ctx, p.Source, pluginLocation{manifest.Metadata.PluginRoot, base, temp}, result, roots)
		if err != nil {
			return fmt.Errorf("plugin %s: %w", p.Name, err)
		}
		if err = discoverPlugin(result, *roots, index, pluginRoot, p); err != nil {
			return err
		}
	}
	for _, name := range result.Declaration.Plugins {
		if !found[name] {
			return fmt.Errorf("%w: plugin %s not found", ErrInvalid, name)
		}
	}
	return nil
}

func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}

type pluginLocation struct{ Prefix, Base, Temp string }

func (e *Engine) pluginRoot(ctx context.Context, raw json.RawMessage, location pluginLocation, result *Resolution, roots *[]string) (int, string, error) {
	prefix, base, temp := location.Prefix, location.Base, location.Temp
	var relative string
	if json.Unmarshal(raw, &relative) == nil {
		if !strings.Contains(relative, "/") && relative != "." && prefix != "" {
			relative = prefix + "/" + relative
		}
		p, err := within(base, relative)
		return 0, p, err
	}
	var spec struct{ Source, Repo, URL, Path, Ref, SHA string }
	if err := json.Unmarshal(raw, &spec); err != nil {
		return 0, "", err
	}
	switch spec.Source {
	case "github":
		spec.URL = spec.Repo
	case "url", "git-subdir":
	default:
		return 0, "", fmt.Errorf("%w: unsupported plugin source %q", ErrInvalid, spec.Source)
	}
	index := len(*roots)
	root := filepath.Join(temp, fmt.Sprintf("repo-%d", index))
	repo, err := e.Fetcher.Fetch(ctx, Repository{Source: spec.URL, Ref: spec.Ref, Commit: spec.SHA}, root)
	if err != nil {
		return 0, "", err
	}
	result.Repositories = append(result.Repositories, repo)
	*roots = append(*roots, root)
	p, err := within(root, spec.Path)
	return index, p, err
}

//nolint:revive // Validate the optional manifest before combining its two skill-path forms.
func pluginSkillPaths(root string, entry *plugin) ([]string, error) {
	var manifest plugin
	err := readJSON(filepath.Join(root, ".claude-plugin", "plugin.json"), &manifest)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	if err == nil && entry.Strict != nil && !*entry.Strict && len(entry.Skills) > 0 {
		return nil, fmt.Errorf("%w: conflicting plugin manifests", ErrInvalid)
	}
	paths := []string{skillsDir}
	for _, raw := range []json.RawMessage{manifest.Skills, entry.Skills} {
		if len(raw) == 0 {
			continue
		}
		var list []string
		var single string
		if json.Unmarshal(raw, &single) == nil {
			list = []string{single}
		} else if err := json.Unmarshal(raw, &list); err != nil {
			return nil, err
		}
		paths = append(paths, list...)
	}
	return paths, nil
}

func discoverPlugin(result *Resolution, roots []string, index int, root string, entry plugin) error {
	paths, err := pluginSkillPaths(root, &entry)
	if err != nil {
		return err
	}

	seen := map[string]bool{}
	for _, path := range paths {
		dir, err := within(root, path)
		if err != nil {
			return err
		}
		if seen[dir] {
			continue
		}
		seen[dir] = true
		if _, err = os.Stat(dir); os.IsNotExist(err) && path == skillsDir {
			continue
		}
		if err = discoverSkills(result, roots, index, dir, entry.Name); err != nil {
			return err
		}
	}
	return nil
}

func discoverSkills(result *Resolution, roots []string, index int, base, pluginName string) error {
	candidates, err := skillDirectories(base)
	if err != nil {
		return err
	}
	for _, dir := range candidates {
		skill, err := inspectSkill(roots[index], dir)
		if err != nil {
			return err
		}
		skill.Repository = index
		skill.Plugin = pluginName
		result.Skills = append(result.Skills, skill)
	}
	return nil
}

func skillDirectories(base string) ([]string, error) {
	if _, err := os.Stat(filepath.Join(base, "SKILL.md")); err == nil {
		return []string{base}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	for _, dir := range []string{base, filepath.Join(base, "agent-skills", skillsDir), filepath.Join(base, skillsDir)} {
		candidates, err := skillChildren(dir)
		if err != nil {
			return nil, err
		}
		if len(candidates) > 0 {
			return candidates, nil
		}
	}
	return nil, nil
}

func skillChildren(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	candidates := []string{}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if _, err := os.Stat(filepath.Join(path, "SKILL.md")); err == nil {
			candidates = append(candidates, path)
		}
	}
	return candidates, nil
}

func inspectSkill(root, dir string) (LockedSkill, error) {
	skill := LockedSkill{}
	digest, err := treeDigest(dir)
	if err != nil {
		return skill, err
	}
	metadata, err := marketplace.ParseSkillMetadata(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		return skill, err
	}
	if !skillNamePattern.MatchString(metadata.Name) {
		return skill, fmt.Errorf("%w: skill name %q", ErrInvalid, metadata.Name)
	}
	if err = marketplace.NewValidator(version.Version).Validate(dir, metadata); err != nil {
		return skill, err
	}
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return skill, err
	}
	return LockedSkill{Name: metadata.Name, Path: filepath.ToSlash(rel), Digest: digest}, nil
}
