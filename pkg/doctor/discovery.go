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

	"gopkg.in/yaml.v3"
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
	// sites are the values that cannot be evaluated statically: templates and custom YAML tags.
	sites []dynamicSite
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
	// limits record why repairs are blocked. complete says whether every input the stack validator reads was captured.
	limits []limit
	// profiles are the active profiles: those selected with --profile or ATMOS_PROFILE, or else
	// the profiles.default value of the base configuration, exactly as the loader chooses them.
	profiles []string
	// excludeHits counts the inputs each --exclude pattern matched, in the order of Options.Exclude.
	excludeHits []int
	// unparsedConfig is the first configuration file that did not parse. Its base_path is unknown.
	unparsedConfig string
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
		p.limit(limitConfig, "", 0, reason)
		addNotice(o, r, incompleteNotice("", reason))
	}
	layout, err := p.locate(o, r)
	if err != nil {
		return nil, err
	}
	if err = e.loadSources(ctx, p, o, r, layout.sources); err != nil {
		return nil, err
	}
	if err = e.loadProfiles(ctx, p, o, r, layout); err != nil {
		return nil, err
	}
	sources, err := projectSources(ctx, p, o, r, layout.configDir)
	if err != nil {
		return nil, err
	}
	e.checkLocalSchemas(p, o, r)
	if err = e.loadSources(ctx, p, o, r, sources); err != nil {
		return nil, err
	}
	p.finishDiscovery(o, r)
	return p, nil
}

// locate finds the project configuration without parsing it and settles the project root.
func (p *project) locate(o *Options, r *Report) (*configLayout, error) {
	layout, err := resolveConfigLayout(o, nil, "", "")
	if err != nil {
		return nil, err
	}
	if !layout.project {
		p.complete = false
		return nil, noProjectError(o, layout.missingConfigs)
	}
	for _, m := range layout.missingConfigs {
		p.limit(limitConfig, "", 0, m.describe())
		addFinding(o, r, missingConfigFinding(m))
	}
	for _, path := range layout.ambient {
		reason := "Ambient configuration is merged at runtime but is outside the project: " + path
		p.limit(limitConfig, "", 0, reason)
		addNotice(o, r, incompleteNotice("", reason))
	}
	rerootProject(p, o, r, layout.root)
	layout.sources = noteOutsideSources(p, o, r, layout.sources)
	return layout, nil
}

// finishDiscovery orders documents and records the layering that limits verification.
func (p *project) finishDiscovery(o *Options, r *Report) {
	sort.Slice(p.docs, func(i, j int) bool { return p.docs[i].path < p.docs[j].path })
	if p.configCount > 1 || len(p.profiles) > 0 {
		p.complete = false // Repairs name the layer that blocks them; see layerBlocker.
		addNotice(o, r, incompleteNotice("", "Multiple configuration layers or profiles require effective-input verification"))
	}
	if p.configCount == 0 {
		p.complete = false
	}
}

// loadProfiles reads the files of the active profiles, which have the highest configuration precedence.
func (e *Engine) loadProfiles(ctx context.Context, p *project, o *Options, r *Report, base *configLayout) error {
	profiles, err := resolveProfiles(o, p, base)
	if err != nil {
		return err
	}
	for _, name := range profiles.missingProfiles {
		p.limit(limitConfig, "", 0, "Active profile was not found: "+name)
		addFinding(o, r, missingProfileFinding(name))
	}
	return e.loadSources(ctx, p, o, r, profiles.sources)
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
		queue = append(queue, importedSources(d, p, o, r, item.kind)...)
	}
	return nil
}

// importedSources lists the files a loaded document imports, read as the kind of document that imports them.
// Vendor manifests import through spec.imports as well as through the top-level import keys.
func importedSources(d *document, p *project, o *Options, r *Report, kind string) []source {
	paths := localImports(d, p, o, r)
	if d.kind == documentVendor {
		paths = append(paths, vendorImports(d, p, o, r)...)
	}
	sources := make([]source, 0, len(paths))
	for _, path := range paths {
		sources = append(sources, source{path, kind})
	}
	return sources
}

// sourceBase resolves the base path that stack and vendor sources are read from.
func sourceBase(p *project, o *Options, r *Report, configDir string) (string, error) {
	if configDir == "" {
		configDir = p.root
	}
	base, err := projectBasePath(o, p, configDir)
	if err != nil {
		return "", err
	}
	if base == configDir || p.unparsedConfig == "" || stringAt(p.config, keyBasePath) != "" || runtimeBasePath(o) != "" {
		return base, nil
	}
	// The configuration that did not parse may declare base_path. Anchoring at the Git root, as
	// Atmos does for a configuration without one, would inspect unrelated files and misplace the
	// syntax error, so only the configuration's own directory is inspected.
	reason := "base_path could not be read because " + relative(configDir, p.unparsedConfig) + " does not parse; only its directory is inspected"
	p.limit(limitConfig, p.unparsedConfig, 0, reason)
	addNotice(o, r, incompleteNotice("", reason))
	return configDir, nil
}

func projectSources(ctx context.Context, p *project, o *Options, r *Report, configDir string) ([]source, error) {
	base, err := sourceBase(p, o, r, configDir)
	if err != nil {
		return nil, err
	}
	if base != p.root && within(base, p.root) {
		// The runtime resolves everything below a base path above the invocation directory.
		rerootProject(p, o, r, base)
	}
	p.base = base
	stackDir := stringAt(p.config, ruleStacks, keyBasePath)
	if stackDir == "" {
		stackDir = ruleStacks
	}
	p.stackDir = absolute(base, stackDir)
	var sources []source
	for _, directory := range []source{{p.stackDir, "stack"}, {filepath.Join(p.root, ".github", "workflows"), "workflow"}} {
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
	configured := stringAt(p.config, "vendor", keyBasePath)
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
			if path != p.root && skipProjectDirectory(p, path, entry.Name()) {
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
	allowed, err := allowSource(p, o, r, path, kind)
	if err != nil || !allowed {
		return nil, err
	}
	rel := relative(p.root, path)
	content, err := e.FS.ReadFile(path)
	if err != nil {
		return nil, p.missingSource(o, r, kind, rel, err)
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
		p.recordParseFailure(o, r, d, rel, err)
	}
	d.sites = dynamicSites(d.node, p.templateMarkers(d))
	if len(d.sites) > 0 {
		// Dynamic values only block the repairs that read them; see blocker. They limit stack
		// validation only when the validator reads this document.
		p.markIncomplete(kind)
	}
	p.docs = append(p.docs, d)
	p.byPath[path] = d
	return d, nil
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// toolDirectory reports version-control and dependency directories, which never hold authored
// Atmos inputs and are skipped wherever they appear.
func toolDirectory(name string) bool {
	switch name {
	case ".git", ".terraform", "node_modules":
		return true
	}
	return false
}

// generatedDirectory reports conventional output and scratch directories. They are skipped only
// directly below the project root, and never when they hold a configured stack, component, or
// vendor root: the same names are valid inside those trees, where Atmos does read them.
func generatedDirectory(name string) bool {
	switch name {
	case ".context", ".atmos", ".cache", "build", "dist":
		return true
	}
	return false
}

// skipProjectDirectory decides whether the project-wide vendor manifest walk descends into path.
func skipProjectDirectory(p *project, path, name string) bool {
	if toolDirectory(name) {
		return true
	}
	return generatedDirectory(name) && filepath.Dir(path) == p.root && !holdsConfiguredRoot(p, path)
}

// holdsConfiguredRoot reports whether path contains, or is inside, a configured stack, component, or vendor root.
func holdsConfiguredRoot(p *project, path string) bool {
	for _, root := range p.configuredRoots() {
		if within(path, root) || within(root, path) {
			return true
		}
	}
	return false
}

// configuredRoots returns the absolute stack, component, and vendor directories.
func (p *project) configuredRoots() []string {
	roots := []string{p.stackDir}
	for _, kind := range []string{"terraform", "helmfile", "packer", "ansible", "helm", "kubernetes", "container"} {
		if dir := stringAt(p.config, "components", kind, keyBasePath); dir != "" {
			roots = append(roots, absolute(p.base, dir))
		}
	}
	if dir := stringAt(p.config, "vendor", keyBasePath); dir != "" {
		roots = append(roots, absolute(p.base, dir))
	}
	return roots
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
			if path != dir && toolDirectory(entry.Name()) {
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

// mappingKeyLine returns the line of key in a mapping, falling back to its value's line.
func mappingKeyLine(n *yaml.Node, key string, value *yaml.Node) int {
	if n != nil && n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			if n.Content[i].Value == key {
				return n.Content[i].Line
			}
		}
	}
	return value.Line
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

func allowSource(p *project, o *Options, r *Report, path, kind string) (bool, error) {
	path = filepath.Clean(path)
	if _, exists := p.byPath[path]; exists {
		return false, nil
	}
	if !within(p.root, path) {
		p.limitKind(kind, path, 0, "Source is outside the project; it will not be edited")
		addNotice(o, r, incompleteNotice(path, "Source is outside the project; it will not be edited"))
		return false, nil
	}
	if resolved, resolveErr := filepath.EvalSymlinks(path); resolveErr == nil && resolved != path {
		p.limitKind(kind, path, 0, "Symlinked sources require manual review")
		addNotice(o, r, incompleteNotice(path, "Symlinked sources require manual review"))
		if !within(p.root, resolved) {
			return false, nil
		}
	}
	rel, _ := filepath.Rel(p.root, path)
	excluded, err := p.excludes(o, filepath.ToSlash(rel))
	if err != nil {
		return false, err
	}
	if excluded {
		p.limitKind(kind, path, 0, "Source is excluded by --exclude")
		return false, nil
	}
	return true, nil
}

func (e *Engine) documentSchema(p *project, r *Report, kind string) (string, error) {
	if kind == documentConfig {
		return configSchema, nil
	}
	if kind != "stack" {
		return "", nil
	}
	source := stringAt(p.config, "schemas", "atmos", "manifest")
	if value, _ := os.LookupEnv("ATMOS_SCHEMAS_ATMOS_MANIFEST"); value != "" {
		source = value
	}
	if localSchemaSource(source) {
		source = e.localSchemaPath(p, source)
	}
	if source == "" {
		return stackSchema, nil
	}
	source, reason, err := e.captureSchema(p, source)
	if reason != "" {
		p.limit(limitProject, "", 0, "manifest schema: "+reason)
		r.Checks = append(r.Checks, Check{ID: "schema/atmos", Status: "incomplete", Reason: reason})
		return "", err
	}
	return source, err
}
