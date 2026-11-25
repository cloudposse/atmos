package engine

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// catalogPin also recognizes canonical commit references emitted for nested catalog stacks.
func catalogPin(pins map[string]string, repository string) string {
	if pin := pins[repository]; pin != "" {
		return pin
	}
	for source, pin := range pins {
		if atmosbridge.PinnedCatalogURI(source, pin) == repository {
			return pin
		}
	}
	return ""
}

// generateCatalogStack passes trusted snapshots through Terragrunt's source-map mechanism.
func (e *unitEvaluator) generateCatalogStack(ctx context.Context, path string, stack *config.StackConfig) error {
	files := atmosbridge.NewCatalogFiles(e.environment.FS)
	sourceMap := map[string]string{}
	// Source-map keys omit the query. Check every enabled source, including unmapped
	// ones, so a sibling that requests a different ref cannot silently use this pin.
	revisions := map[string]string{}
	add := func(source, name string) error {
		repository, subdir := atmosbridge.CatalogRepository(source)
		key, _, _ := strings.Cut(repository, "?")
		pin := catalogPin(e.request.Catalogs, repository)
		if previous, seen := revisions[key]; seen && previous != pin {
			return errors.New("Terragrunt source mapping requires one revision of each catalog per stack file")
		}
		revisions[key] = pin
		if pin == "" {
			if strings.HasPrefix(repository, "git::") || strings.HasPrefix(repository, "github.com/") {
				if _, err := e.environment.FS.Stat(localPath(filepath.Dir(path), source)); err != nil {
					return &model.Problem{
						Code: model.CodeUnsupportedSetting, Summary: "Remote Terragrunt catalog needs a configured commit", Key: "source", File: path, Safe: true,
						Hints: []string{"Add the repository source and its commit SHA to components.terraform.terragrunt.catalogs."},
					}
				}
			}
			return nil
		}
		catalog, err := atmosbridge.FetchCatalog(ctx, e.logger, e.request.Environment, repository, subdir, pin)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			return &model.Problem{Code: model.CodeEvaluationError, Summary: "Could not resolve the pinned Terragrunt catalog", Key: "source", File: path, Detail: err.Error(), Err: err, Hints: []string{"Check the catalog commit, Git access, and the //subdirectory in the stack source."}}
		}
		sourceMap[key] = catalog.Root
		return files.Include(catalog, name)
	}
	for _, u := range stack.Units {
		if u.IsEnabled() {
			if err := add(u.Source, unitFile); err != nil {
				return err
			}
		}
	}
	for _, s := range stack.Stacks {
		if s.IsEnabled() {
			if err := add(s.Source, stackFile); err != nil {
				return err
			}
		}
	}
	if len(sourceMap) == 0 {
		return atmosbridge.GenerateStack(ctx, e.logger, e.environment, path, e.request.Tofu)
	}
	return atmosbridge.GenerateStackWithSources(ctx, e.logger, e.environment.WithFS(files), path, e.request.Tofu, sourceMap)
}
