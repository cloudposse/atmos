package engine

import (
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/spf13/afero"
	"github.com/zclconf/go-cty/cty"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// errTrackSources reports configuration that cannot be inspected for file reads.
var errTrackSources = errors.New("cannot track configuration sources")

// lockFileName is the provider lock file Terragrunt keeps beside a unit.
const lockFileName = ".terraform.lock.hcl"

// fileScan is the evaluation scope of one HCL file being inspected.
type fileScan struct {
	// base resolves relative paths named by file functions in this file.
	base string
	ctx  *hcl.EvalContext
}

// queuedFile is an HCL file to inspect. Files the unit evaluated must exist;
// files discovered through read_terragrunt_config are best effort.
type queuedFile struct {
	path     string
	required bool
}

// watchTracker collects everything a unit reads that partial discovery does not
// evaluate: input expressions, generate blocks, extra arguments, and hooks.
type watchTracker struct {
	watchEvaluator
	result  *model.Resolution
	files   map[string]struct{}
	folders map[string]struct{}
	visited map[string]struct{}
	queue   []queuedFile
}

// trackInputSources inspects syntax, not values. Partial discovery intentionally
// leaves inputs unevaluated; syntax watches include conditional and optional reads
// without reading state, decrypting secrets, or invoking user commands. Paths are
// resolved with a pure expression subset (see watchEvaluator) so the common
// get_terragrunt_dir() idioms name exact files; only genuinely unresolvable
// expressions watch the whole project.
func trackInputSources(fs afero.Fs, root string, result *model.Resolution) error {
	tracker := newWatchTracker(fs, root, result)
	if err := tracker.scan(); err != nil {
		return err
	}
	tracker.addStaticWatches()
	tracker.merge()
	return nil
}

func newWatchTracker(fs afero.Fs, root string, result *model.Resolution) *watchTracker {
	tracker := &watchTracker{
		watchEvaluator: watchEvaluator{fs: fs, root: root, unit: unitDirectory(result)},
		result:         result,
		files:          map[string]struct{}{},
		folders:        map[string]struct{}{},
		visited:        map[string]struct{}{},
	}
	if contents, err := afero.ReadFile(fs, tracker.unitFile()); err == nil {
		if file, diagnostics := hclsyntax.ParseConfig(contents, tracker.unitFile(), hcl.InitialPos); !diagnostics.HasErrors() {
			tracker.parent = tracker.includeParent(file.Body.(*hclsyntax.Body))
		}
	}
	return tracker
}

// unitDirectory is the directory get_terragrunt_dir() reports.
func unitDirectory(result *model.Resolution) string {
	if result.Path != "" {
		return filepath.Dir(result.Path)
	}
	for _, path := range result.Files {
		if strings.HasSuffix(path, ".hcl") {
			return filepath.Dir(path)
		}
	}
	return "."
}

func (t *watchTracker) unitFile() string {
	if t.result.Path != "" {
		return t.result.Path
	}
	return filepath.Join(t.unit, "terragrunt.hcl")
}

func (t *watchTracker) scan() error {
	for _, path := range t.result.Files {
		// Lock files are watched, never inspected: they hold no expressions.
		if strings.HasSuffix(path, ".hcl") && filepath.Base(path) != lockFileName {
			t.queue = append(t.queue, queuedFile{path: path, required: true})
		}
	}
	for len(t.queue) > 0 {
		next := t.queue[0]
		t.queue = t.queue[1:]
		if _, seen := t.visited[next.path]; seen {
			continue
		}
		t.visited[next.path] = struct{}{}
		if err := t.scanFile(next); err != nil {
			return err
		}
	}
	return nil
}

func (t *watchTracker) scanFile(next queuedFile) error {
	contents, err := afero.ReadFile(t.fs, next.path)
	if err != nil {
		return optionalFailure(next, fmt.Errorf("track configuration sources %s: %w", next.path, err))
	}
	file, diagnostics := hclsyntax.ParseConfig(contents, next.path, hcl.InitialPos)
	if diagnostics.HasErrors() {
		return optionalFailure(next, fmt.Errorf("%w in %s", errTrackSources, next.path))
	}
	body := file.Body.(*hclsyntax.Body)
	scan := fileScan{base: filepath.Dir(next.path), ctx: t.context(t.locals(body))}
	if inputs := body.Attributes["inputs"]; inputs != nil {
		t.visit(scan, inputs.Expr)
	}
	for _, block := range body.Blocks {
		t.scanBlock(scan, block, next.required)
	}
	return nil
}

// optionalFailure ignores problems in files that were only discovered through
// read_terragrunt_config; the unit's own files must be inspectable.
func optionalFailure(next queuedFile, err error) error {
	if !next.required {
		return nil
	}
	return err
}

func (t *watchTracker) scanBlock(scan fileScan, block *hclsyntax.Block, required bool) {
	switch block.Type {
	case "generate", "remote_state":
		t.visit(scan, block)
	case "terraform":
		t.scanTerraform(scan, block)
	case "locals":
		// Discovery already evaluated the unit's own locals; files reached through
		// read_terragrunt_config are only inspected here.
		if !required {
			t.visit(scan, block)
		}
	}
}

func (t *watchTracker) visit(scan fileScan, node hclsyntax.Node) {
	_ = hclsyntax.VisitAll(node, func(node hclsyntax.Node) hcl.Diagnostics {
		if call, ok := node.(*hclsyntax.FunctionCallExpr); ok {
			t.trackCall(scan, call)
		}
		return nil
	})
}

// trackCall names the files one function call reads.
func (t *watchTracker) trackCall(scan fileScan, call *hclsyntax.FunctionCallExpr) {
	switch call.Name {
	case "file", "fileexists", "filebase64", "filemd5", "filesha1", "filesha256", "filesha512",
		"filebase64sha256", "filebase64sha512", "templatefile", "read_tfvars_file", "read_terragrunt_config",
		"sops_decrypt_file", "mark_as_read":
		t.trackFile(scan, call)
	case "fileset":
		if path, ok := t.firstPath(scan, call); ok {
			t.add(t.folders, path)
		} else {
			t.fallback()
		}
	case "find_in_parent_folders":
		value, diagnostics := call.Value(scan.ctx)
		if diagnostics.HasErrors() || value.IsNull() || !value.IsKnown() || value.Type() != cty.String {
			t.fallback()
			return
		}
		t.addDetected(value.AsString())
	}
}

// trackFile watches the file a read function names and follows configuration files it reads.
func (t *watchTracker) trackFile(scan fileScan, call *hclsyntax.FunctionCallExpr) {
	path, ok := t.firstPath(scan, call)
	if !ok {
		t.fallback()
		return
	}
	t.add(t.files, path)
	if call.Name == "read_terragrunt_config" && strings.HasSuffix(path, ".hcl") {
		t.queue = append(t.queue, queuedFile{path: path})
	}
}

// firstPath evaluates the first argument against the file's directory.
func (t *watchTracker) firstPath(scan fileScan, call *hclsyntax.FunctionCallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}
	path, ok := stringOf(call.Args[0], scan.ctx)
	if !ok {
		return "", false
	}
	return localPath(scan.base, path), true
}

// scanTerraform watches var files and hook scripts. Hooks and extra arguments
// run later under the host; only the files they name matter for selection.
func (t *watchTracker) scanTerraform(scan fileScan, block *hclsyntax.Block) {
	t.visit(scan, block)
	for _, child := range block.Body.Blocks {
		switch child.Type {
		case "extra_arguments":
			for _, name := range []string{"required_var_files", "optional_var_files"} {
				if attribute := child.Body.Attributes[name]; attribute != nil {
					t.watchList(scan, attribute.Expr)
				}
			}
		case "before_hook", "after_hook", "error_hook":
			t.watchHook(scan, child.Body)
		}
	}
}

// listElements returns the element expressions of a literal list. A computed
// list is one element: its whole value is evaluated at once.
func listElements(expression hclsyntax.Expression) []hclsyntax.Expression {
	if tuple, ok := expression.(*hclsyntax.TupleConsExpr); ok {
		return tuple.Exprs
	}
	return []hclsyntax.Expression{expression}
}

// watchList watches each string in a list. Elements that cannot be resolved
// widen the watch to the project root.
func (t *watchTracker) watchList(scan fileScan, expression hclsyntax.Expression) {
	for _, element := range listElements(expression) {
		if path, ok := stringOf(element, scan.ctx); ok {
			t.add(t.files, localPath(t.unit, path))
			continue
		}
		if values, diagnostics := element.Value(scan.ctx); !diagnostics.HasErrors() && values.IsWhollyKnown() && !values.IsNull() && values.CanIterateElements() {
			for _, value := range values.AsValueSlice() {
				if value.Type() == cty.String && !value.IsNull() {
					t.add(t.files, localPath(t.unit, value.AsString()))
				}
			}
			continue
		}
		t.fallback()
	}
}

// watchHook watches the hook program (and script arguments) when they live in
// the project, plus the hook working directory. Resolution is best effort: a
// program found on PATH is not a project file.
func (t *watchTracker) watchHook(scan fileScan, body *hclsyntax.Body) {
	if execute := body.Attributes["execute"]; execute != nil {
		for index, element := range listElements(execute.Expr) {
			word, ok := stringOf(element, scan.ctx)
			if !ok || !strings.ContainsAny(word, `/\`) {
				continue
			}
			path := localPath(t.unit, word)
			if info, err := t.fs.Stat(path); index == 0 || (err == nil && !info.IsDir()) {
				t.add(t.files, path)
			}
		}
	}
	if attribute := body.Attributes["working_dir"]; attribute != nil {
		if directory, ok := stringOf(attribute.Expr, scan.ctx); ok {
			t.add(t.folders, localPath(t.unit, directory))
		}
	}
}

// add records a watch inside the project; paths outside it cannot appear in the
// project's change set and would make the whole inventory unreportable.
func (t *watchTracker) add(set map[string]struct{}, path string) {
	if within(path, t.root) {
		set[path] = struct{}{}
	}
}

func (t *watchTracker) addDetected(path string) {
	if info, err := t.fs.Stat(path); err == nil && info.IsDir() {
		t.add(t.folders, path)
		return
	}
	t.add(t.files, path)
}

// fallback watches the whole project for expressions that cannot be resolved.
// Unknown paths can refer anywhere; a root directory watch is deliberately
// conservative and catches new files as well as changed files.
func (t *watchTracker) fallback() {
	t.folders[t.root] = struct{}{}
}

// addStaticWatches adds inputs to every unit that appear in no expression: the
// lock file beside the unit, and the whole directory Terragrunt copies when the
// source selects a subdirectory with `//`.
func (t *watchTracker) addStaticWatches() {
	t.add(t.files, filepath.Join(t.unit, lockFileName))
	if source := t.result.Source; source != "" && !plainLocalSource(source) {
		if copied, ok := copiedSourceRoot(t.unit, source); ok {
			t.add(t.folders, copied)
		}
	}
}

// copiedSourceRoot returns the directory a local `dir//subdir` source copies: the
// whole tree before `//`, so sibling modules referenced by relative path count.
func copiedSourceRoot(unit, source string) (string, bool) {
	volume := filepath.VolumeName(source)
	rest := source[len(volume):]
	if strings.ContainsAny(rest, "?:") {
		return "", false
	}
	index := strings.Index(filepath.ToSlash(rest), "//")
	if index <= 0 {
		return "", false
	}
	return localPath(unit, volume+rest[:index]), true
}

// merge folds the collected watches into the resolution. Symlinked paths watch
// the resolved target as well as the link, because Git reports changes by the
// target's path. Files already covered by a watched folder are dropped: they add
// nothing, and they differ between checkouts when a folder is a submodule that
// one revision has not populated.
func (t *watchTracker) merge() {
	files := slices.Concat(t.result.Files, slices.Collect(maps.Keys(t.files)))
	folders := slices.Concat(t.result.Folders, slices.Collect(maps.Keys(t.folders)))
	files = append(files, t.symlinkTargets(files)...)
	folders = append(folders, t.symlinkTargets(folders)...)
	folders = slices.Compact(slices.Sorted(slices.Values(folders)))
	files = slices.DeleteFunc(files, func(path string) bool {
		return path != t.result.Path && slices.ContainsFunc(folders, func(folder string) bool { return within(path, folder) })
	})
	t.result.Files = slices.Compact(slices.Sorted(slices.Values(files)))
	t.result.Folders = folders
}

func (t *watchTracker) symlinkTargets(paths []string) []string {
	var targets []string
	for _, path := range paths {
		resolved, err := canonicalPath(path)
		if err == nil && resolved != path && within(resolved, t.root) {
			targets = append(targets, resolved)
		}
	}
	return targets
}
