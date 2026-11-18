package manifest

import (
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/utils"
)

// localIncludeResolver maps an !include path to the local file it names, or "" for a remote
// include. It is injected so the confinement owns the boundary policy while the include
// resolution rules stay with the loader that defines them.
type localIncludeResolver func(includeFile, manifestFile string, atmosConfig *schema.AtmosConfiguration) string

// confineScaffoldIncludes wraps the local include handlers of policy with a publication-root
// boundary: an include that resolves outside root, or that a forced go-getter source could
// use to read a local file, is rejected.
func confineScaffoldIncludes(policy utils.TagWalkPolicy, root string, resolve localIncludeResolver) (utils.TagWalkPolicy, error) {
	realRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return policy, err
	}
	realRoot, err = filepath.Abs(realRoot)
	if err != nil {
		return policy, err
	}
	for _, tag := range []string{utils.AtmosYamlFuncInclude, utils.AtmosYamlFuncIncludeRaw} {
		next := policy.Handlers[tag]
		policy.Handlers[tag] = func(ctx utils.TagContext, node *yaml.Node, val string) (bool, error) {
			if err := checkScaffoldInclude(realRoot, &ctx, val, resolve); err != nil {
				return false, err
			}
			return next(ctx, node, val)
		}
	}
	return policy, nil
}

func checkScaffoldInclude(root string, ctx *utils.TagContext, val string, resolve localIncludeResolver) error {
	parsed, err := parser.ParseInclude(val)
	if err != nil {
		return err
	}
	// Forced go-getter sources may disguise local files as remote includes.
	// A local getter must not bypass the publication-root check below.
	if localGetterInclude(parsed.Path) {
		return fmt.Errorf("%w: local getter include %q is not allowed during scaffold publication", errUtils.ErrArtifactScaffoldInvalid, parsed.Path)
	}
	local := resolve(parsed.Path, ctx.File, ctx.AtmosConfig)
	if local == "" {
		return nil
	}
	resolved, err := filepath.EvalSymlinks(local)
	if err != nil {
		return err
	}
	if outsideRoot(root, resolved) {
		return fmt.Errorf("%w: include %q lies outside the scaffold publication root", errUtils.ErrArtifactScaffoldInvalid, parsed.Path)
	}
	return nil
}

// outsideRoot reports whether path lies outside root.
func outsideRoot(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel)
}

func localGetterInclude(path string) bool {
	lower := strings.ToLower(path)
	if strings.Contains(lower, "file://") || strings.HasPrefix(lower, "file::") {
		return true
	}
	_, source, forced := strings.Cut(path, "::")
	return forced && (filepath.IsAbs(source) || strings.HasPrefix(source, "."))
}
