package doctor

import (
	"context"
	"path/filepath"
)

const keyWorkflows = "workflows"

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
	sources, err := directorySources(projectDirectories(p, o, r))
	if err != nil {
		return nil, err
	}
	vendors, err := vendorSources(ctx, p, base)
	if err != nil {
		return nil, err
	}
	// Named manifests have a more specific consumer than their containing stack
	// directory. Preserve GitHub and Atmos workflow priority, then named files.
	split := 0
	for split < len(sources) && sources[split].kind != documentStack {
		split++
	}
	ordered := append([]source{}, sources[:split]...)
	ordered = append(ordered, vendors...)
	return append(ordered, sources[split:]...), nil
}

// projectDirectories orders specific consumers ahead of the broad stack walk.
func projectDirectories(p *project, o *Options, r *Report) []source {
	// Specific dialects must precede the broad stack walk: a workflow directory
	// is often nested under stacks. readDocument deduplicates by absolute path.
	directories := []source{{filepath.Join(p.root, ".github", keyWorkflows), "workflow"}}
	workflowDir := stringAt(p.config, keyWorkflows, keyBasePath)
	if workflowDir != "" && !dynamicDiscoveryPath(p, keyWorkflows, keyBasePath) {
		directories = append(directories, source{absolute(p.base, workflowDir), documentAtmosWorkflow})
	} else if dynamicDiscoveryPath(p, keyWorkflows, keyBasePath) {
		addFunctionLimit(o, r, "", "The workflow directory is dynamic; its YAML function scopes cannot be inspected without evaluation")
	}
	directories = append(directories, source{p.stackDir, documentStack})
	return directories
}

func directorySources(directories []source) ([]source, error) {
	var sources []source
	for _, directory := range directories {
		paths, err := yamlFiles(directory.path)
		if err != nil {
			return nil, err
		}
		for _, path := range paths {
			sources = append(sources, source{path, directory.kind})
		}
	}
	return sources, nil
}

// dynamicDiscoveryPath reads authored nodes, because decoding a custom tag to a
// string loses the distinction between a directory and an unevaluated argument.
func dynamicDiscoveryPath(p *project, path ...string) bool {
	for _, d := range p.docs {
		if d.kind != documentConfig {
			continue
		}
		for _, site := range d.sites {
			if startsWith(path, site.path) {
				return true
			}
		}
	}
	return false
}
