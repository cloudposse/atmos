// Package githubactions implements the github-actions file manager: it scans
// workflow files for `uses:` action references, matches them against managed
// version entries by owner/repo package, and rewrites the ref from the lock.
// Pinned entries render the Renovate/Dependabot round-trip convention
// `uses: owner/repo@<commit-sha> # <version>`; the trailing comment keeps the
// human-readable version next to the immutable SHA.
package githubactions

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/version/manager"
	"github.com/cloudposse/atmos/pkg/version/managers"
)

// Name is the manager's registry name.
const Name = "github-actions"

// usesLinePattern matches workflow `uses:` lines: indentation and optional
// list dash, the action path, the ref, and an optional trailing comment.
// Local actions (./path) and docker:// references never match because the
// action path requires an owner/repo prefix without a scheme.
var usesLinePattern = regexp.MustCompile(`^(?P<prefix>\s*(?:-\s+)?uses\s*:\s*)(?P<action>[A-Za-z0-9_.-]+/[A-Za-z0-9_./-]+)@(?P<ref>[^\s#]+)(?P<trailer>\s*(?:#.*)?)$`)

// packagePathParts is the number of leading segments forming the owner/repo package.
const packagePathParts = 2

// Manager rewrites GitHub Actions workflow refs from the lock.
type Manager struct{}

// Name returns the manager's registry name.
func (Manager) Name() string {
	defer perf.Track(nil, "githubactions.Manager.Name")()

	return Name
}

// DefaultPaths returns the standard workflow locations.
func (Manager) DefaultPaths() []string {
	defer perf.Track(nil, "githubactions.Manager.DefaultPaths")()

	return []string{".github/workflows/*.yml", ".github/workflows/*.yaml"}
}

// Plan scans workflow files and returns the rewrites needed to match the
// locked versions. Files are rewritten line-by-line, never YAML round-tripped,
// so formatting and comments are preserved.
func (m Manager) Plan(ctx context.Context, in *managers.Input) ([]managers.FileChange, error) {
	defer perf.Track(in.Config, "githubactions.Manager.Plan")()

	byPackage := packageRefs(in)
	if len(byPackage) == 0 {
		return nil, nil
	}
	paths := in.Paths
	if len(paths) == 0 {
		paths = m.DefaultPaths()
	}
	files, err := managers.ExpandPaths(in.Dir, paths)
	if err != nil {
		return nil, err
	}
	var changes []managers.FileChange
	for _, file := range files {
		content, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		updated := rewriteWorkflow(content, byPackage)
		if !bytes.Equal(content, updated) {
			changes = append(changes, managers.FileChange{Path: file, Old: content, New: updated})
		}
	}
	return changes, nil
}

// packageRefs indexes locked references by owner/repo package for entries in
// GitHub-backed ecosystems.
func packageRefs(in *managers.Input) map[string]manager.VersionRef {
	refs := map[string]manager.VersionRef{}
	for name := range in.Entries {
		entry := in.Entries[name]
		switch entry.Datasource {
		case "github-tags", "github-releases", "github", "github/actions", "github-actions":
		default:
			continue
		}
		ref, ok := in.Refs[name]
		if !ok || ref.Version == "" {
			continue
		}
		refs[entry.Package] = ref
	}
	return refs
}

// rewriteWorkflow rewrites every managed `uses:` line in a workflow document.
func rewriteWorkflow(content []byte, byPackage map[string]manager.VersionRef) []byte {
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		match := usesLinePattern.FindStringSubmatch(line)
		if match == nil {
			continue
		}
		prefix, action := match[1], match[2]
		ref, ok := byPackage[actionPackage(action)]
		if !ok {
			continue
		}
		if ref.Pin == manager.PinDigest && ref.Digest != "" {
			lines[i] = prefix + action + "@" + actionRef(&ref)
		} else {
			lines[i] = prefix + action + "@" + ref.Version + match[4]
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// actionPackage returns the owner/repo prefix of an action path, tolerating
// subdirectory actions (owner/repo/subdir) and reusable workflow paths
// (owner/repo/.github/workflows/x.yml).
func actionPackage(action string) string {
	parts := strings.SplitN(action, "/", packagePathParts+1)
	if len(parts) < packagePathParts {
		return action
	}
	return parts[0] + "/" + parts[1]
}

// actionRef renders the ref portion for a managed version: the pinned commit
// SHA with the version as a trailing comment, or the plain version. Any stale
// trailing comment is replaced.
func actionRef(ref *manager.VersionRef) string {
	if ref.Pin == manager.PinDigest && ref.Digest != "" {
		return fmt.Sprintf("%s # %s", ref.Digest, ref.Version)
	}
	return ref.Version
}

func init() {
	managers.Register(Manager{})
}
