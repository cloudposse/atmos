package doctor

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"gopkg.in/yaml.v3"

	"github.com/cloudposse/atmos/pkg/validation"
)

const (
	configSchema = "atmos://schema/atmos/config/1.0"
	stackSchema  = "atmos://schema/atmos/manifest/1.0"
)

type document struct {
	path    string
	kind    string
	content []byte
	node    *yaml.Node
	mode    fs.FileMode
	schema  string
	dynamic bool
}

type project struct {
	extra       map[string][]byte
	root        string
	base        string
	docs        []*document
	byPath      map[string]*document
	stackDir    string
	config      map[string]any
	complete    bool
	configCount int
}

func parse(content []byte) (*yaml.Node, error) {
	dec := yaml.NewDecoder(bytes.NewReader(content))
	var node, next yaml.Node
	if err := dec.Decode(&node); err != nil && !errors.Is(err, io.EOF) {
		return nil, err
	}
	if err := dec.Decode(&next); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: multiple YAML documents are not supported", ErrVerification)
	}
	var decoded any
	if err := node.Decode(&decoded); err != nil {
		return nil, err
	}
	return &node, nil
}

type source struct{ path, kind string }

func (e *Engine) discover(ctx context.Context, o *Options, r *Report) (*project, error) {
	p := &project{root: o.Root, byPath: map[string]*document{}, config: map[string]any{}, extra: map[string][]byte{}, complete: true}
	for _, reason := range o.UnverifiedInputs {
		p.complete = false
		r.Findings = append(r.Findings, incomplete("", reason))
	}
	configs, err := configSources(o)
	if err != nil {
		return nil, err
	}
	if err = e.loadSources(ctx, p, o, r, configs); err != nil {
		return nil, err
	}
	sources, err := projectSources(ctx, p, o)
	if err != nil {
		return nil, err
	}
	if err = e.loadSources(ctx, p, o, r, sources); err != nil {
		return nil, err
	}
	sort.Slice(p.docs, func(i, j int) bool { return p.docs[i].path < p.docs[j].path })
	if p.configCount > 1 || len(o.Profiles) > 0 {
		p.complete = false
		r.Findings = append(r.Findings, incomplete("", "Multiple configuration layers or profiles require effective-input verification"))
	}
	if p.configCount == 0 {
		p.complete = false
		if enabled(o, ruleSchema, categoryValidation) {
			r.Findings = append(r.Findings, Finding{RuleID: ruleSchema, Category: categoryValidation, Severity: validation.SeverityWarning, Message: "No project Atmos configuration found", Status: statusManual, Remediation: "Select a configuration with --config or run from the project directory"})
		}
	}
	return p, nil
}

func configSources(o *Options) ([]source, error) {
	var sources []source
	names := o.ConfigFiles
	if len(names) == 0 {
		for _, name := range []string{"atmos.yaml", "atmos.yml", ".atmos.yaml", ".atmos.yml", ".atmos/atmos.yaml"} {
			path := filepath.Join(o.Root, name)
			if _, err := os.Stat(path); err == nil {
				names = append(names, path)
			}
		}
	}
	for _, name := range names {
		sources = append(sources, source{absolute(o.Root, name), documentConfig})
	}
	for _, dir := range append([]string{"atmos.d", ".atmos.d", "profiles", ".atmos/profiles"}, o.ConfigDirs...) {
		paths, err := yamlFiles(absolute(o.Root, dir))
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			sources = append(sources, source{path, documentConfig})
		}
	}
	return sources, nil
}

func (e *Engine) loadSources(ctx context.Context, p *project, o *Options, r *Report, queue []source) error {
	for len(queue) > 0 {
		item := queue[0]
		queue = queue[1:]
		if err := ctx.Err(); err != nil {
			return err
		}
		d, err := e.readDocument(p, o, r, item.path, item.kind)
		if err != nil {
			return err
		}
		if d == nil || d.node == nil {
			continue
		}
		if d.kind == documentConfig {
			var values map[string]any
			if err := d.node.Decode(&values); err == nil {
				mergeMaps(p.config, values)
			}
		}
		for _, path := range localImports(d, p, r) {
			queue = append(queue, source{path, item.kind})
		}
	}
	return nil
}

func projectSources(ctx context.Context, p *project, o *Options) ([]source, error) {
	base := o.BasePath
	if base == "" {
		base = stringAt(p.config, "base_path")
	}
	if base == "" {
		base = o.Root
	}
	base = absolute(o.Root, base)
	p.base = base
	stackDir := stringAt(p.config, ruleStacks, "base_path")
	if stackDir == "" {
		stackDir = ruleStacks
	}
	p.stackDir = absolute(base, stackDir)
	var sources []source
	for _, directory := range []source{{p.stackDir, "stack"}, {filepath.Join(o.Root, ".github", "workflows"), "workflow"}} {
		paths, err := yamlFiles(directory.path)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			sources = append(sources, source{path, directory.kind})
		}
	}
	vendors, err := vendorSources(ctx, p, base)
	if err != nil {
		return nil, err
	}
	return append(sources, vendors...), nil
}

func vendorSources(ctx context.Context, p *project, base string) ([]source, error) {
	var sources []source
	configured := stringAt(p.config, "vendor", "base_path")
	if configured != "" {
		path := absolute(base, configured)
		paths, err := vendorPathFiles(path)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			sources = append(sources, source{path, "vendor"})
		}
	}
	err := filepath.WalkDir(p.root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err = ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			if path != p.root && skipDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		switch entry.Name() {
		case "vendor.yaml", "vendor.yml", "component.yaml", "component.yml":
			sources = append(sources, source{path, "vendor"})
		}
		return nil
	})
	return sources, err
}

func vendorPathFiles(path string) ([]string, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return yamlFiles(path)
	}
	return []string{path}, nil
}

func absolute(base, path string) string {
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Join(base, path)
}

func (e *Engine) readDocument(p *project, o *Options, r *Report, path, kind string) (*document, error) {
	path = filepath.Clean(path)
	allowed, err := allowSource(p, o, r, path)
	if err != nil || !allowed {
		return nil, err
	}
	rel := relative(p.root, path)
	content, err := e.FS.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			p.complete = false
			r.Findings = append(r.Findings, Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityError, File: rel, Message: "Local source does not exist", Status: statusManual})
			return nil, nil
		}
		return nil, err
	}
	info, err := e.FS.Stat(path)
	if err != nil {
		return nil, err
	}
	d := &document{path: path, kind: kind, content: content, mode: info.Mode().Perm()}
	if kind == documentConfig {
		p.configCount++
	}
	d.schema, err = e.documentSchema(p, r, kind)
	if err != nil {
		return nil, err
	}
	d.node, err = parse(content)
	if err != nil {
		p.complete = false
		r.Findings = append(r.Findings, Finding{RuleID: ruleSchema, Category: categoryValidation, Severity: validation.SeverityError, File: rel, Message: err.Error(), Status: statusManual})
		d.node = nil
	}
	d.dynamic = dynamicDocument(d.node, p.templateMarkers())
	if d.dynamic {
		p.complete = false
	}
	p.docs = append(p.docs, d)
	p.byPath[path] = d
	return d, nil
}

func localImports(d *document, p *project, r *Report) []string {
	var paths []string
	root := rootNode(d.node)
	for _, key := range []string{"import", ruleImports} {
		n := mappingValue(root, key)
		if n == nil {
			continue
		}
		p.complete = false // Layered sources need an effective-config verifier before fixing config keys.
		items := n.Content
		if n.Kind == yaml.ScalarNode {
			items = []*yaml.Node{n}
		}
		for _, item := range items {
			paths = append(paths, resolveLocalImport(d, p, r, item)...)
		}
	}
	return paths
}

func incomplete(file, reason string) Finding {
	return Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityWarning, File: file, Message: "Configuration verification incomplete", Status: statusVerificationIncomplete, Reason: reason}
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func skipDirectory(name string) bool {
	switch name {
	case ".git", ".context", "node_modules", ".terraform", ".atmos", ".cache", "build", "dist":
		return true
	}
	return false
}

func yamlFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if os.IsNotExist(err) && path == dir {
			return nil
		}
		if err != nil {
			return err
		}
		if entry.IsDir() {
			if path != dir && skipDirectory(entry.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if ext := filepath.Ext(path); ext == ".yaml" || ext == ".yml" {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

func rootNode(n *yaml.Node) *yaml.Node {
	if n != nil && n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		return n.Content[0]
	}
	return n
}

func mappingValue(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}

func walk(n *yaml.Node, path []string, fn func(*yaml.Node, []string)) {
	if n == nil {
		return
	}
	fn(n, path)
	if n.Kind == yaml.AliasNode {
		walk(n.Alias, path, fn)
	}
	if n.Kind == yaml.DocumentNode {
		for _, child := range n.Content {
			walk(child, path, fn)
		}
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			walk(n.Content[i+1], append(append([]string{}, path...), n.Content[i].Value), fn)
		}
	}
	if n.Kind == yaml.SequenceNode {
		for i, child := range n.Content {
			walk(child, append(append([]string{}, path...), fmt.Sprint(i)), fn)
		}
	}
}

func stringAt(m map[string]any, keys ...string) string {
	var v any = m
	for _, key := range keys {
		object, ok := v.(map[string]any)
		if !ok {
			return ""
		}
		v = object[key]
	}
	s, _ := v.(string)
	return s
}

func mergeMaps(dst, src map[string]any) {
	for key, value := range src {
		child, ok := value.(map[string]any)
		if !ok {
			dst[key] = value
			continue
		}
		target, _ := dst[key].(map[string]any)
		if target == nil {
			target = map[string]any{}
		}
		mergeMaps(target, child)
		dst[key] = target
	}
}

func allowSource(p *project, o *Options, r *Report, path string) (bool, error) {
	path = filepath.Clean(path)
	if _, exists := p.byPath[path]; exists {
		return false, nil
	}
	if !within(p.root, path) {
		p.complete = false
		r.Findings = append(r.Findings, incomplete(path, "Source is outside the project; it will not be edited"))
		return false, nil
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil && resolved != path {
		p.complete = false
		r.Findings = append(r.Findings, incomplete(path, "Symlinked sources require manual review"))
		if !within(p.root, resolved) {
			return false, nil
		}
	}
	rel, _ := filepath.Rel(p.root, path)
	filtered, err := validation.ExcludePaths([]string{filepath.ToSlash(rel)}, o.Exclude)
	if err != nil {
		return false, err
	}
	if len(filtered) == 0 {
		p.complete = false
		return false, nil
	}
	return true, nil
}

func resolveLocalImport(d *document, p *project, r *Report, item *yaml.Node) []string {
	item, optional := importDeclaration(item)
	if item == nil {
		return nil
	}
	value := item.Value
	if !staticImport(item) {
		r.Findings = append(r.Findings, incomplete(d.path, "Dynamic or remote import was not evaluated"))
		return nil
	}
	base := filepath.Dir(d.path)
	if d.kind == "stack" {
		base = p.stackDir
	}
	path := value
	if !filepath.IsAbs(path) {
		path = filepath.Join(base, path)
	}
	if filepath.Ext(path) == "" {
		path += ".yaml"
	}
	matches, err := doublestar.FilepathGlob(path)
	if err != nil || len(matches) == 0 {
		if optional && err == nil {
			return nil
		}
		r.Findings = append(r.Findings, Finding{RuleID: ruleImports, Category: categoryValidation, Severity: validation.SeverityError, File: d.path, Line: item.Line, Message: "Local import does not resolve: " + value, Status: statusManual})
		return nil
	}
	return matches
}

func dynamicDocument(node *yaml.Node, markers []string) bool {
	dynamic := false
	walk(node, nil, func(n *yaml.Node, _ []string) {
		if strings.HasPrefix(n.Tag, "!") && !strings.HasPrefix(n.Tag, "!!") || containsTemplate(n.Value, markers) || strings.HasPrefix(n.Value, "!") {
			dynamic = true
		}
	})
	return dynamic
}

func staticImport(item *yaml.Node) bool {
	return item.Kind == yaml.ScalarNode && item.Tag == "!!str" && !strings.ContainsAny(item.Value, "{}") && !strings.Contains(item.Value, "://") && !strings.HasPrefix(item.Value, "git::")
}

func importDeclaration(item *yaml.Node) (*yaml.Node, bool) {
	if item.Kind != yaml.MappingNode {
		return item, false
	}
	optional := false
	if skip := mappingValue(item, "skip_if_missing"); skip != nil {
		_ = skip.Decode(&optional)
	}
	return mappingValue(item, "path"), optional
}

func (e *Engine) documentSchema(p *project, r *Report, kind string) (string, error) {
	if kind == documentConfig {
		return configSchema, nil
	}
	if kind != "stack" {
		return "", nil
	}
	source := stringAt(p.config, "schemas", "atmos", "manifest")
	if source == "" {
		return stackSchema, nil
	}
	source, reason, err := e.captureSchema(p, source)
	if reason != "" {
		p.complete = false
		r.Checks = append(r.Checks, Check{ID: "schema/atmos", Status: "incomplete", Reason: reason})
		return "", err
	}
	return source, err
}
