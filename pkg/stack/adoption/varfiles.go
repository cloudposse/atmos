package adoption

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"unicode"

	"gopkg.in/yaml.v3"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filesystem"
	"github.com/cloudposse/atmos/pkg/filetype"
	"github.com/cloudposse/atmos/pkg/perf"
	y "github.com/cloudposse/atmos/pkg/yaml"
)

// AddVarFiles writes ordered include references, or verifies them on a repeated registration.
func (r *Registration) AddVarFiles(data []byte, target VarFileTarget, exists bool) ([]byte, error) {
	defer perf.Track(nil, "adoption.Registration.AddVarFiles")()

	vars := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
	for _, ref := range r.VarFiles {
		node, err := r.varInclude(target, ref)
		if err != nil {
			return nil, err
		}
		vars.Content = append(vars.Content, node)
	}
	key := target.Dest + ".vars"
	if exists {
		current, err := varNode(data, key)
		if err != nil {
			return nil, err
		}
		if (current == nil && len(vars.Content) == 0) || (current != nil && sameIncludes(current, vars)) {
			return data, nil
		}
		if current != nil && !managedIncludes(current, target.Extensions) {
			return nil, fmt.Errorf("%w: existing %s has different variable sources including hand-authored variables; preserve them and select a different --name", errUtils.ErrInvalidStackConfig, key)
		}
		if !r.ReplaceVarFiles {
			replacement := *r
			replacement.ReplaceVarFiles = true
			return nil, fmt.Errorf("%w: existing %s has different variable sources; preview replacing the ordered references with: %s --dry-run", errUtils.ErrInvalidStackConfig, key, replacement.SaveCommand())
		}
		if len(vars.Content) == 0 {
			return y.Delete(data, key)
		}
	}
	if len(vars.Content) == 0 {
		return data, nil
	}
	return setYAMLData(data, key, vars)
}

func (r *Registration) varInclude(target VarFileTarget, ref string) (*yaml.Node, error) {
	path := ref
	if !filepath.IsAbs(path) {
		path = filepath.Join(target.Dir, path)
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("variable file %q (relative to %s): %w", ref, target.Dir, err)
	}
	if _, err := RelativeDirectory(r.Project, path); err != nil {
		return nil, err
	}
	if !slices.ContainsFunc(target.Extensions, func(ext string) bool { return strings.HasSuffix(path, ext) }) {
		return nil, fmt.Errorf("%w: unsupported variable file %q", errUtils.ErrInvalidComponent, ref)
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%w: expected a regular variable file at %s", errUtils.ErrInvalidComponent, path)
	}
	values, err := filetype.ParseFileByExtension(os.ReadFile, path)
	if err != nil {
		return nil, err
	}
	if _, ok := values.(map[string]any); !ok {
		return nil, fmt.Errorf("%w: %s must contain a variable assignment map", errUtils.ErrInvalidComponent, ref)
	}
	return includeReference(target.File, path)
}

func includeReference(manifest, path string) (*yaml.Node, error) {
	anchor, err := filesystem.CanonicalPath(filepath.Dir(manifest))
	if err != nil {
		return nil, err
	}
	rel, err := filepath.Rel(anchor, path)
	if err != nil {
		return nil, err
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasPrefix(rel, "../") {
		rel = "./" + rel
	}
	if strings.ContainsAny(rel, "?#") || strings.Contains(rel, "{{") {
		return nil, fmt.Errorf("%w: %s cannot be represented as a literal include path", errUtils.ErrInvalidComponent, path)
	}
	if strings.ContainsAny(rel, `|'"`) || strings.IndexFunc(rel, unicode.IsSpace) >= 0 {
		rel = `"` + strings.ReplaceAll(rel, `"`, `""`) + `"`
	}
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!include", Value: rel}, nil
}

func varNode(data []byte, path string) (*yaml.Node, error) {
	raw, err := y.Get(data, path)
	if errors.Is(err, y.ErrYAMLPathNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return nil, err
	}
	if len(doc.Content) == 0 {
		return nil, nil
	}
	return doc.Content[0], nil
}

func managedIncludes(current *yaml.Node, extensions []string) bool {
	if current.Kind != yaml.SequenceNode {
		return false
	}
	for _, n := range current.Content {
		ref := literalInclude(n)
		if ref == "" || (!strings.HasPrefix(ref, "../") && !strings.HasPrefix(ref, "./")) || !slices.ContainsFunc(extensions, func(ext string) bool { return strings.HasSuffix(ref, ext) }) {
			return false
		}
	}
	return true
}

func literalInclude(n *yaml.Node) string {
	if n.Kind != yaml.ScalarNode || n.Tag != "!include" || strings.ContainsAny(n.Value, "?#") || strings.Contains(n.Value, "{{") {
		return ""
	}
	const quote = `"`
	if unquoted, ok := strings.CutPrefix(n.Value, quote); ok {
		if rest, ok := strings.CutSuffix(unquoted, quote); ok && !strings.Contains(strings.ReplaceAll(rest, `""`, ""), quote) {
			return strings.ReplaceAll(rest, `""`, quote)
		}
		return ""
	}
	if strings.ContainsAny(n.Value, `|'"`) || strings.IndexFunc(n.Value, unicode.IsSpace) >= 0 {
		return ""
	}
	return n.Value
}

func sameIncludes(current, want *yaml.Node) bool {
	if current.Kind != yaml.SequenceNode || len(current.Content) != len(want.Content) {
		return false
	}
	for i, node := range current.Content {
		if node.Tag != "!include" || node.Value != want.Content[i].Value {
			return false
		}
	}
	return true
}

// SavedVarFiles recovers the order of plain include references without opening their targets.
// A missing include can therefore be corrected from the same component and stack.
func (r *Registration) SavedVarFiles(kind, component, dir string) ([]string, error) {
	defer perf.Track(nil, "adoption.Registration.SavedVarFiles")()
	if r.Config == nil || !StandaloneStacks(&r.Config.Stacks) {
		return nil, nil
	}
	path, err := StandaloneDestination(r.Config.StacksBaseAbsolutePath, r.Stack)
	if err != nil {
		return nil, err
	}
	b, _, err := ReadRegular(path)
	if err != nil || len(b) == 0 {
		return nil, err
	}
	current, err := varNode(b, y.ComponentPath(kind, component, "vars"))
	if err != nil || current == nil {
		return nil, err
	}
	return savedReferences(current, path, dir, kind)
}

func savedReferences(current *yaml.Node, path, dir, kind string) ([]string, error) {
	extensions := []string{".tfvars", ".tfvars.json"}
	if kind == "packer" {
		extensions = []string{".pkrvars.hcl", ".pkrvars.json"}
	}
	if !managedIncludes(current, extensions) {
		return nil, nil
	}
	var files []string
	for _, n := range current.Content {
		rel, err := filepath.Rel(dir, filepath.Join(filepath.Dir(path), literalInclude(n)))
		if err != nil {
			return nil, err
		}
		files = append(files, filepath.ToSlash(rel))
	}
	return files, nil
}
