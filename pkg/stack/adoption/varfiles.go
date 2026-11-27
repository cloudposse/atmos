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
		if err := verifyVarSources(data, key, vars); err != nil {
			return nil, err
		}
		return data, nil
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
		return nil, err
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
	anchor, err := canonicalDestinationDir(filepath.Dir(manifest))
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

func verifyVarSources(data []byte, path string, want *yaml.Node) error {
	raw, err := y.Get(data, path)
	if errors.Is(err, y.ErrYAMLPathNotFound) && len(want.Content) == 0 {
		return nil
	}
	if err != nil && !errors.Is(err, y.ErrYAMLPathNotFound) {
		return err
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
		return err
	}
	if len(doc.Content) > 0 && sameIncludes(doc.Content[0], want) {
		return nil
	}
	return fmt.Errorf("%w: existing %s has different variable sources; pass the original --var-file flags in order or choose another component", errUtils.ErrInvalidStackConfig, path)
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

// Stack directories may be created by this registration. Canonicalize their
// existing ancestors so the same checkout has identical references on retry.
func canonicalDestinationDir(dir string) (string, error) {
	resolved, err := filepath.EvalSymlinks(dir)
	if err == nil || !os.IsNotExist(err) {
		return resolved, err
	}
	parent := filepath.Dir(dir)
	if parent == dir {
		return "", err
	}
	resolved, err = canonicalDestinationDir(parent)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolved, filepath.Base(dir)), nil
}
