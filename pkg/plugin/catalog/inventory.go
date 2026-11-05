package catalog

import (
	"context"
	"fmt"
	"maps"
	"path"
	"regexp"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"

	errUtils "github.com/cloudposse/atmos/errors"
	pkgit "github.com/cloudposse/atmos/pkg/git"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/plugin/inventory"
	"github.com/cloudposse/atmos/pkg/plugin/manifest"
	pluginoci "github.com/cloudposse/atmos/pkg/plugin/oci"
)

type inventoryOptions struct {
	file, head string
	runner     pkgit.Runner
}

// InventoryOption configures committed catalog input independently of release planning.
type InventoryOption func(*inventoryOptions)

// WithInventory selects a repository-relative inventory file.
//
//nolint:lintroller // Functional option.
func WithInventory(file string) InventoryOption {
	return func(o *inventoryOptions) { o.file = file }
}

// WithHead selects the committed revision containing the accepted inventory and manifests.
//
//nolint:lintroller // Functional option.
func WithHead(head string) InventoryOption {
	return func(o *inventoryOptions) { o.head = head }
}

// WithInventoryRunner injects Git access for inventory reads.
//
//nolint:lintroller // Functional option.
func WithInventoryRunner(runner pkgit.Runner) InventoryOption {
	return func(o *inventoryOptions) { o.runner = runner }
}

var inventoryName = regexp.MustCompile(`^[a-z][a-z0-9-]*$`)

// ReadInventory selects releases from committed manifests without building a CI release plan.
// It resolves the revision once, so concurrent branch movement cannot mix snapshots.
func ReadInventory(ctx context.Context, options ...InventoryOption) ([]ReleaseSelection, error) {
	defer perf.Track(nil, "plugincatalog.ReadInventory")()

	o := inventoryOptions{file: "plugins/releases.yaml", head: "HEAD", runner: pkgit.NewExecRunner()}
	for _, option := range options {
		option(&o)
	}
	if o.runner == nil || !inventory.SafePath(o.file) || o.head == "" {
		return nil, fmt.Errorf("%w: catalog inventory requires a repository-relative path and a committed revision", errUtils.ErrPluginInvalidConfig)
	}
	result, err := o.runner.Run(ctx, "git", []string{"rev-parse", "--verify", "--end-of-options", o.head + "^{commit}"}, pkgit.RunOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: resolve catalog inventory revision: %w", errUtils.ErrPluginInvalidConfig, err)
	}
	o.head = strings.TrimSpace(result.Stdout)
	if o.head == "" {
		return nil, fmt.Errorf("%w: catalog inventory revision is empty", errUtils.ErrPluginInvalidConfig)
	}
	raw, err := o.read(ctx, o.file)
	if err != nil {
		return nil, err
	}
	file, err := inventory.Parse(raw)
	if err != nil {
		return nil, err
	}
	return o.releases(ctx, file)
}

func (o *inventoryOptions) read(ctx context.Context, filename string) ([]byte, error) {
	result, err := o.runner.Run(ctx, "git", []string{"show", o.head + ":" + filename}, pkgit.RunOptions{})
	if err != nil {
		return nil, fmt.Errorf("%w: read catalog inventory file %s: %w", errUtils.ErrPluginInvalidConfig, filename, err)
	}
	return []byte(result.Stdout), nil
}

func (o *inventoryOptions) releases(ctx context.Context, file *inventory.File) ([]ReleaseSelection, error) {
	releases := make([]ReleaseSelection, 0, len(file.Packages))
	for _, key := range slices.Sorted(maps.Keys(file.Packages)) {
		selection, err := o.release(ctx, key, file.Packages[key])
		if err != nil {
			return nil, fmt.Errorf("%w: catalog inventory package %s: %w", errUtils.ErrPluginInvalidConfig, key, err)
		}
		releases = append(releases, selection)
	}
	return releases, nil
}

func (o *inventoryOptions) release(ctx context.Context, key string, item inventory.Package) (ReleaseSelection, error) {
	if err := validateInventoryPackage(key, item); err != nil {
		return ReleaseSelection{}, err
	}
	raw, err := o.read(ctx, item.Path+"/plugin.yaml")
	if err != nil {
		return ReleaseSelection{}, err
	}
	definition, err := manifest.Parse(raw)
	if err != nil {
		return ReleaseSelection{}, err
	}
	if path.Base(definition.Metadata.Name) != key {
		return ReleaseSelection{}, fmt.Errorf("%w: inventory key must match manifest name", errUtils.ErrPluginInvalidConfig)
	}
	version, err := semver.StrictNewVersion(definition.Metadata.Version)
	if err != nil || version.Prerelease() != "" || version.Metadata() != "" {
		return ReleaseSelection{}, fmt.Errorf("%w: inventory releases require a stable major.minor.patch version", errUtils.ErrPluginInvalidConfig)
	}
	return ReleaseSelection{Name: key, Version: version.String(), Repository: item.Repository}, nil
}

func validateInventoryPackage(key string, item inventory.Package) error {
	if !inventoryName.MatchString(key) || !inventory.SafePath(item.Path) || strings.ContainsAny(item.Path, "*?[]") {
		return fmt.Errorf("%w: invalid package name or path", errUtils.ErrPluginInvalidConfig)
	}
	if !strings.HasPrefix(item.Repository, "oci://") {
		return fmt.Errorf("%w: repository must start with oci://", errUtils.ErrPluginInvalidConfig)
	}
	if _, err := pluginoci.ParseDestination(item.Repository); err != nil {
		return err
	}
	return nil
}
