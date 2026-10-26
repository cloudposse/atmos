package atmos

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/internal/exec"
	cfg "github.com/cloudposse/atmos/pkg/config"
	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/schema"
	pkgstack "github.com/cloudposse/atmos/pkg/stack"
	u "github.com/cloudposse/atmos/pkg/utils"
	atmosyaml "github.com/cloudposse/atmos/pkg/yaml"
)

// stackEditTarget holds the resolved manifest file and in-file path for a
// stack config edit, plus the effective merged value and where it currently
// resolves from. Mirrors cmd/stack/operations.go's unexported editTarget.
type stackEditTarget struct {
	file       string // manifest file to edit.
	inFilePath string // raw dot-path used as the provenance lookup key (components.<type>.<name>.<rel>).
	yqPath     string // escaped dot-path used to address the YAML node safely.
	value      string // effective merged value of the path.
	provFile   string // file provenance attributes the value to.
	provLine   int    // line within provFile.
}

// describeStackComponentForEdit describes a component in a stack with
// provenance tracking enabled, so callers can find the source manifest for
// each effective value. This replicates cmd/stack/operations.go's
// describeComponentForEdit since that logic isn't exported.
func describeStackComponentForEdit(atmosConfig *schema.AtmosConfiguration, stack, component string) (*exec.DescribeComponentResult, error) {
	atmosConfig.TrackProvenance = true

	return exec.ExecuteDescribeComponentWithContext(exec.DescribeComponentContextParams{
		AtmosConfig:          atmosConfig,
		Component:            component,
		Stack:                stack,
		ProcessTemplates:     false,
		ProcessYamlFunctions: false,
		AuthDisabled:         true,
	})
}

// stackEditRequest bundles the parameters shared by resolveStackEditTarget and
// resolveStackTargetByProvenance, keeping each function's argument list small.
type stackEditRequest struct {
	atmosConfig     *schema.AtmosConfiguration
	stack           string
	component       string
	dotPath         string
	fileOverride    string
	requireEditable bool
}

// resolveStackEditTarget describes the component in the stack (with
// provenance) and resolves the manifest file plus in-file path for the given
// component-relative dot-path. When req.requireEditable is true (set/delete),
// the value must resolve to a concrete, writable manifest node; otherwise
// (get) provenance is best-effort and an explicit req.fileOverride is read
// directly for its effective value. This replicates cmd/stack/operations.go's
// resolveEditTarget since that logic isn't exported.
func resolveStackEditTarget(req *stackEditRequest) (*stackEditTarget, error) {
	result, err := describeStackComponentForEdit(req.atmosConfig, req.stack, req.component)
	if err != nil {
		return nil, err
	}

	componentType, _ := result.ComponentSection[cfg.ComponentTypeSectionName].(string)

	tgt := &stackEditTarget{
		// Raw path keys provenance lookups; escaped path addresses YAML nodes.
		inFilePath: pkgstack.BuildComponentInFilePath(componentType, req.component, req.dotPath),
		yqPath:     pkgstack.BuildComponentYqPath(componentType, req.component, req.dotPath),
	}

	// Effective merged value (best-effort; used by get and for messaging).
	if sectionYAML, convErr := u.ConvertToYAML(result.ComponentSection); convErr == nil {
		if v, getErr := atmosyaml.Get([]byte(sectionYAML), req.dotPath); getErr == nil {
			tgt.value = v
		}
	}

	// Explicit file override bypasses provenance resolution.
	if req.fileOverride != "" {
		tgt.file = req.fileOverride
		// For read-only get, reflect the value actually stored in the explicit
		// file rather than the merged value.
		if !req.requireEditable {
			if v, getErr := atmosyaml.GetFile(req.fileOverride, tgt.yqPath); getErr == nil {
				tgt.value = v
			}
		}
		return tgt, nil
	}

	return resolveStackTargetByProvenance(req, result, tgt)
}

// resolveStackTargetByProvenance fills tgt.file from provenance: it finds the
// manifest that defines the effective value, resolves it to an absolute path,
// and verifies the reconstructed in-file path exists there. This replicates
// cmd/stack/operations.go's resolveTargetByProvenance since that logic isn't
// exported.
func resolveStackTargetByProvenance(req *stackEditRequest, result *exec.DescribeComponentResult, tgt *stackEditTarget) (*stackEditTarget, error) {
	// Provenance is keyed by the raw in-file path (components.<type>.<name>.<rel>),
	// matching how merge provenance keys are recorded.
	var entries []merge.ProvenanceEntry
	if result.MergeContext != nil {
		entries = result.MergeContext.GetProvenance(tgt.inFilePath)
	}
	provFile, provLine, ok := pkgstack.PickProvenanceFile(entries)
	if !ok {
		// For read-only get there is nothing to edit; return the best-effort
		// merged value without provenance instead of erroring.
		if !req.requireEditable {
			return tgt, nil
		}
		return nil, fmt.Errorf("%w: %q is not defined for component %q in stack %q", errUtils.ErrAIStackConfigPathNotEditable, req.dotPath, req.component, req.stack)
	}
	tgt.provFile = provFile
	tgt.provLine = provLine

	// Provenance records the file relative to the stacks base path; resolve it
	// to an absolute path for reading and writing.
	absFile := provFile
	if !filepath.IsAbs(absFile) {
		absFile = filepath.Join(req.atmosConfig.StacksBaseAbsolutePath, provFile)
	}

	// Verify the reconstructed in-file path actually exists in the resolved file.
	// When it doesn't (e.g. the value is inherited from a base component under a
	// different key), get still reports the provenance location, but set/delete
	// require an explicit file because there is no concrete node to edit here.
	if _, verifyErr := atmosyaml.GetFile(absFile, tgt.yqPath); verifyErr != nil {
		if !req.requireEditable {
			return tgt, nil
		}
		return nil, fmt.Errorf("%w: %q resolves from %s:%d, but its key there is not %q (likely inherited or imported)",
			errUtils.ErrAIStackConfigPathNotEditable, req.dotPath, provFile, provLine, tgt.inFilePath)
	}
	tgt.file = absFile
	return tgt, nil
}

// stackFormatFilesFromProvenance collects the absolute paths of every
// manifest file that contributed to component's effective configuration, so
// format can normalize all of them rather than a single file. This
// replicates cmd/stack/operations.go's stackFormatFilesFromProvenance since
// that logic isn't exported.
func stackFormatFilesFromProvenance(atmosConfig *schema.AtmosConfiguration, result *exec.DescribeComponentResult, stack, component string) ([]string, error) {
	if result == nil || result.MergeContext == nil {
		return nil, fmt.Errorf("%w: no provenance was found for component %q in stack %q", errUtils.ErrAIStackConfigPathNotEditable, component, stack)
	}

	componentType, _ := result.ComponentSection[cfg.ComponentTypeSectionName].(string)
	prefix := pkgstack.BuildComponentInFilePath(componentType, component, "")
	seen := map[string]bool{}
	files := make([]string, 0)
	for _, path := range result.MergeContext.GetProvenancePaths() {
		if path != prefix && !strings.HasPrefix(path, prefix+".") {
			continue
		}
		provFile, _, ok := pkgstack.PickProvenanceFile(result.MergeContext.GetProvenance(path))
		if !ok {
			continue
		}
		absFile := provFile
		if !filepath.IsAbs(absFile) {
			absFile = filepath.Join(atmosConfig.StacksBaseAbsolutePath, provFile)
		}
		if !seen[absFile] {
			seen[absFile] = true
			files = append(files, absFile)
		}
	}
	sort.Strings(files)
	if len(files) == 0 {
		return nil, fmt.Errorf("%w: no editable manifest files were found for component %q in stack %q", errUtils.ErrAIStackConfigPathNotEditable, component, stack)
	}
	return files, nil
}

// stackProvenanceFileForPath returns the manifest file (relative to the
// stacks base path when possible) that defines dotPath for component. This
// replicates cmd/stack/config.go's provenanceFileForComponentPath since that
// logic isn't exported.
func stackProvenanceFileForPath(atmosConfig *schema.AtmosConfiguration, result *exec.DescribeComponentResult, componentType, component, dotPath string) (string, bool) {
	if result.MergeContext == nil {
		return "", false
	}
	entries := result.MergeContext.GetProvenance(pkgstack.BuildComponentInFilePath(componentType, component, dotPath))
	provFile, _, ok := pkgstack.PickProvenanceFile(entries)
	if !ok {
		return "", false
	}
	return stackRelativePathForDisplay(provFile, atmosConfig.StacksBaseAbsolutePath), true
}

// stackRelativePathForDisplay returns file relative to basePath for
// user-facing display when possible, falling back to a slash-normalized
// absolute/raw path otherwise. This replicates cmd/stack/config.go's
// relativePathForStackDisplay since that logic isn't exported.
func stackRelativePathForDisplay(file, basePath string) string {
	if basePath == "" || !filepath.IsAbs(file) {
		return filepath.ToSlash(file)
	}
	rel, err := filepath.Rel(basePath, file)
	if err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel) {
		return filepath.ToSlash(rel)
	}
	return filepath.ToSlash(file)
}
