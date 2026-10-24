// Package releaseplan plans independently versioned plugin releases from committed sources.
package releaseplan

import (
	"context"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/bmatcuk/doublestar/v4"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	packageinventory "github.com/cloudposse/atmos/pkg/plugin/inventory"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
	"github.com/cloudposse/atmos/pkg/plugin/platform"
)

// Inventory declares packages and CI runner assignments independently of the CI provider.
type Inventory = packageinventory.File

// Package identifies a package and its additional repository-relative build inputs.
type Package = packageinventory.Package

// Entry pins one package to committed source and its manifest version.
type Entry struct {
	Name       string `json:"name"`
	Path       string `json:"path"`
	Version    string `json:"version"`
	Source     string `json:"source"`
	Repository string `json:"repository"`
}

// BuildEntry assigns one declared platform to a CI runner and artifact name.
type BuildEntry struct {
	Entry
	Platform   string `json:"platform"`
	Runner     string `json:"runner"`
	Artifact   string `json:"artifact"`
	Executable string `json:"executable"`
}

// Matrix is an explicit CI matrix include list.
type Matrix[T any] struct {
	Include []T `json:"include"`
}

// Plan includes all declared packages for idempotent release reconciliation.
type Plan struct {
	Packages Matrix[Entry]      `json:"packages"`
	Build    Matrix[BuildEntry] `json:"build"`
}

type settings struct {
	repository            Repository
	previous              *Inventory
	inventory, head, base string
}

// Option configures committed-source planning.
type Option func(*settings)

// WithRepository injects repository access.
func WithRepository(repository Repository) Option {
	defer perf.Track(nil, "releaseplan.WithRepository")()

	return func(s *settings) { s.repository = repository }
}

// WithInventory selects a repository-relative inventory file.
func WithInventory(value string) Option {
	defer perf.Track(nil, "releaseplan.WithInventory")()

	return func(s *settings) { s.inventory = value }
}

// WithHead selects the committed revision to plan.
func WithHead(value string) Option {
	defer perf.Track(nil, "releaseplan.WithHead")()

	return func(s *settings) { s.head = value }
}

// WithBase checks independent version increases against a committed revision.
func WithBase(value string) Option {
	defer perf.Track(nil, "releaseplan.WithBase")()

	return func(s *settings) { s.base = value }
}

var slug = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// Create reads only committed objects, ignoring uncommitted working tree changes.
func Create(ctx context.Context, options ...Option) (*Plan, error) {
	defer perf.Track(nil, "releaseplan.Create")()

	s := settings{repository: Git{}, inventory: "plugins/releases.yaml", head: "HEAD"}
	for _, option := range options {
		option(&s)
	}
	if s.repository == nil || !safePath(s.inventory) || s.head == "" {
		return nil, errUtils.ErrPluginReleasePlan
	}
	if err := s.resolve(ctx); err != nil {
		return nil, err
	}
	inventory, err := s.loadInventory(ctx, s.head)
	if err != nil {
		return nil, err
	}
	changed, existing, err := s.comparison(ctx)
	if err != nil {
		return nil, err
	}
	result := &Plan{Packages: Matrix[Entry]{Include: []Entry{}}, Build: Matrix[BuildEntry]{Include: []BuildEntry{}}}
	for _, key := range slices.Sorted(maps.Keys(inventory.Packages)) {
		entry, builds, err := s.planPackage(ctx, key, inventory, changed, existing)
		if err != nil {
			return nil, fmt.Errorf("%w: %s: %w", errUtils.ErrPluginReleasePlan, key, err)
		}
		result.Packages.Include = append(result.Packages.Include, entry)
		result.Build.Include = append(result.Build.Include, builds...)
	}
	return result, nil
}

func (s *settings) loadInventory(ctx context.Context, revision string) (*Inventory, error) {
	raw, err := s.read(ctx, revision, s.inventory)
	if err != nil {
		return nil, err
	}
	file, err := packageinventory.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", errUtils.ErrPluginReleasePlan, err)
	}
	return file, nil
}

func (s *settings) resolve(ctx context.Context) error {
	for _, ref := range []*string{&s.head, &s.base} {
		if *ref == "" {
			continue
		}
		raw, err := s.repository.Run(ctx, "rev-parse", "--verify", "--end-of-options", *ref+"^{commit}")
		if err != nil {
			return fmt.Errorf("%w: resolve revision: %w", errUtils.ErrPluginReleasePlan, err)
		}
		*ref = strings.TrimSpace(string(raw))
	}
	return nil
}

func (s *settings) read(ctx context.Context, ref, filename string) ([]byte, error) {
	raw, err := s.repository.Run(ctx, "show", ref+":"+filename)
	if err != nil {
		return nil, fmt.Errorf("%w: read %s: %w", errUtils.ErrPluginReleasePlan, filename, err)
	}
	return raw, nil
}

func (s *settings) comparison(ctx context.Context) ([]string, []string, error) {
	if s.base == "" {
		return nil, nil, nil
	}
	changed, err := s.repository.Run(ctx, "diff", "--name-only", "--no-relative", "--no-renames", "-z", s.base, s.head, "--")
	if err != nil {
		return nil, nil, fmt.Errorf("%w: compare revisions: %w", errUtils.ErrPluginReleasePlan, err)
	}
	existing, err := s.repository.Run(ctx, "ls-tree", "--full-tree", "-r", "--name-only", "-z", s.base)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: list base files: %w", errUtils.ErrPluginReleasePlan, err)
	}
	files := strings.Split(string(existing), "\x00")
	if slices.Contains(files, s.inventory) {
		s.previous, err = s.loadInventory(ctx, s.base)
		if err != nil {
			return nil, nil, err
		}
	}
	return strings.Split(string(changed), "\x00"), files, nil
}

func (s *settings) planPackage(ctx context.Context, key string, inventory *Inventory, changed, existing []string) (Entry, []BuildEntry, error) {
	config := inventory.Packages[key]
	if err := validatePackage(key, config); err != nil {
		return Entry{}, nil, err
	}
	filename := config.Path + "/plugin.yaml"
	raw, err := s.read(ctx, s.head, filename)
	if err != nil {
		return Entry{}, nil, err
	}
	definition, version, err := parseManifest(raw)
	if err != nil {
		return Entry{}, nil, err
	}
	if path.Base(definition.Metadata.Name) != key {
		return Entry{}, nil, fmt.Errorf("%w: inventory key must match manifest name", errUtils.ErrPluginReleasePlan)
	}
	previousFile, err := s.previousManifest(key, filename)
	if err != nil {
		return Entry{}, nil, err
	}
	if slices.Contains(existing, previousFile) {
		requiresIncrease := affected(changed, config) || inventoryChanged(s.previous, inventory, key, definition)
		if err := s.checkVersion(ctx, previousFile, version, requiresIncrease); err != nil {
			return Entry{}, nil, err
		}
	}
	source, err := s.sourceRevision(ctx, config)
	if err != nil {
		return Entry{}, nil, err
	}
	entry := Entry{Name: key, Path: config.Path, Repository: config.Repository, Version: version.String(), Source: source}
	builds, err := buildEntries(&entry, definition, inventory.Runners)
	return entry, builds, err
}

func (s *settings) sourceRevision(ctx context.Context, config Package) (string, error) {
	// Pin the latest input change, not merely the last manifest edit. This allows
	// reviewed fixes within a release PR while preserving source across unrelated commits.
	// First-parent history retains the integrated snapshot at merges rather than
	// selecting a feature commit that may omit other newly accepted shared inputs.
	args := []string{"log", "-1", "--first-parent", "--format=%H", s.head, "--", ":(top,literal)" + config.Path, ":(top,literal)" + s.inventory}
	for _, input := range config.Inputs {
		args = append(args, ":(top,glob)"+input)
	}
	source, err := s.repository.Run(ctx, args...)
	if err != nil {
		return "", fmt.Errorf("%w: find source commit: %w", errUtils.ErrPluginReleasePlan, err)
	}
	if strings.TrimSpace(string(source)) == "" {
		return "", fmt.Errorf("%w: source commit missing", errUtils.ErrPluginReleasePlan)
	}
	return strings.TrimSpace(string(source)), nil
}

func validatePackage(key string, config Package) error {
	if !slug.MatchString(key) || !safePath(config.Path) || strings.ContainsAny(config.Path, "*?[]") {
		return errUtils.ErrPluginReleasePlan
	}
	if !strings.HasPrefix(config.Repository, "oci://") {
		return fmt.Errorf("%w: repository must start with oci://", errUtils.ErrPluginReleasePlan)
	}
	if _, err := pluginoci.ParseDestination(config.Repository); err != nil {
		return err
	}
	for _, input := range config.Inputs {
		if !safePath(input) || !validPattern(input) {
			return fmt.Errorf("%w: invalid build input %q", errUtils.ErrPluginReleasePlan, input)
		}
	}
	return nil
}

// validPattern restricts inputs to globs shared by Git and doublestar.
func validPattern(value string) bool {
	if !doublestar.ValidatePattern(value) || strings.ContainsAny(value, "{}") {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if strings.Contains(part, "**") && part != "**" {
			return false
		}
	}
	return true
}

func safePath(value string) bool {
	return packageinventory.SafePath(value)
}

func parseManifest(raw []byte) (*manifest.Config, *semver.Version, error) {
	definition, err := manifest.Parse(raw)
	if err != nil {
		return nil, nil, err
	}
	version, err := semver.StrictNewVersion(definition.Metadata.Version)
	if err != nil || version.Prerelease() != "" || version.Metadata() != "" {
		return nil, nil, fmt.Errorf("%w: releases require a stable major.minor.patch version", errUtils.ErrPluginReleasePlan)
	}
	return definition, version, nil
}

func (s *settings) previousManifest(key, fallback string) (string, error) {
	if s.previous == nil {
		return fallback, nil
	}
	config, ok := s.previous.Packages[key]
	if !ok {
		return fallback, nil
	}
	if !safePath(config.Path) {
		return "", errUtils.ErrPluginReleasePlan
	}
	return config.Path + "/plugin.yaml", nil
}

func inventoryChanged(previous, current *Inventory, key string, definition *manifest.Config) bool {
	if previous == nil {
		return true
	}
	old, exists := previous.Packages[key]
	next := current.Packages[key]
	if !exists || old.Path != next.Path || old.Repository != next.Repository || !slices.Equal(old.Inputs, next.Inputs) {
		return true
	}
	for target := range definition.Spec.Platforms {
		if previous.Runners[target] != current.Runners[target] {
			return true
		}
	}
	return false
}

func (s *settings) checkVersion(ctx context.Context, filename string, current *semver.Version, requiresIncrease bool) error {
	raw, err := s.read(ctx, s.base, filename)
	if err != nil {
		return err
	}
	_, previous, err := parseManifest(raw)
	if err != nil {
		return err
	}
	if current.LessThan(previous) {
		return fmt.Errorf("%w: version cannot decrease", errUtils.ErrPluginReleasePlan)
	}
	if requiresIncrease && !current.GreaterThan(previous) {
		return fmt.Errorf("%w: changed build inputs require metadata.version to increase", errUtils.ErrPluginReleasePlan)
	}
	return nil
}

func affected(changed []string, config Package) bool {
	for _, filename := range changed {
		if strings.HasPrefix(filename, config.Path+"/") {
			return true
		}
		for _, pattern := range config.Inputs {
			if doublestar.MatchUnvalidated(pattern, filename) {
				return true
			}
		}
	}
	return false
}

func buildEntries(entry *Entry, definition *manifest.Config, runners map[string]string) ([]BuildEntry, error) {
	if len(definition.Spec.Platforms) == 0 {
		return nil, fmt.Errorf("%w: package declares no release platforms", errUtils.ErrPluginReleasePlan)
	}
	result := make([]BuildEntry, 0, len(definition.Spec.Platforms))
	for _, target := range slices.Sorted(maps.Keys(definition.Spec.Platforms)) {
		if _, err := platform.Parse(target); err != nil {
			return nil, err
		}
		runner := runners[target]
		executable := definition.Spec.Platforms[target].Executable
		if strings.TrimSpace(runner) == "" || !safePath(executable) {
			return nil, fmt.Errorf("%w: %s requires a runner and safe executable path", errUtils.ErrPluginReleasePlan, target)
		}
		result = append(result, BuildEntry{Entry: *entry, Platform: target, Runner: runner, Artifact: strings.ReplaceAll(target, "/", "-"), Executable: executable})
	}
	return result, nil
}
