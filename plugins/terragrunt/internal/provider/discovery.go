package provider

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

// Discover expands local stacks in an isolated overlay without preparing units.
func (p *Provider) Discover(ctx context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	ctx, sink := withLogs(ctx, request.GetInvocation(), request.GetRootDirectory(), request.GetProviderConfiguration())
	response, err := p.discover(ctx, request)
	if response != nil {
		response.Logs = logEntries(sink)
	}
	return response, withFailureLogs(err, sink)
}

// discoveryRequest validates a discovery request and turns it into an inventory evaluation.
func discoveryRequest(request *pluginv1.DiscoverComponentsRequest) (model.Request, error) {
	req, err := operation(request.GetRootDirectory(), request.GetProvider(), request.GetOperation(), request.GetInvocation())
	if err != nil {
		return req, rpcError(err)
	}
	if err := settings(request.GetProviderConfiguration()); err != nil {
		return req, rpcError(err)
	}
	if err := withFeatures(&req, request.GetProviderConfiguration()); err != nil {
		return req, rpcError(err)
	}
	paths, err := importPaths(request)
	if err != nil {
		return req, rpcError(err)
	}
	req.Phase, req.IncludedPaths, req.ExcludedPaths = "inventory", paths, request.ExcludePaths
	return req, nil
}

func (p *Provider) discover(ctx context.Context, request *pluginv1.DiscoverComponentsRequest) (*pluginv1.DiscoverComponentsResponse, error) {
	req, err := discoveryRequest(request)
	if err != nil {
		return nil, err
	}
	executable := request.GetOperation().GetMode() == ModeExecutable
	result, err := p.evaluate(ctx, req)
	if err != nil {
		return nil, rpcError(err)
	}
	addresses := map[string]*pluginv1.ComponentAddress{}
	for _, unit := range result.Inventory {
		addresses[unit.Config.Path] = &pluginv1.ComponentAddress{Stack: unit.Stack, Component: unit.Name, ComponentType: "terraform"}
	}
	response := &pluginv1.DiscoverComponentsResponse{}
	for _, failure := range result.Failures {
		response.Diagnostics = append(response.Diagnostics, unitDiagnostic(req.BasePath, failure))
	}
	for _, unit := range result.Inventory {
		discovered, err := discoveredUnit(req, unit, addresses)
		if err == nil && executable {
			// The host lists the unit with the engine that runs it.
			discovered.Engine = "terragrunt"
		}
		if err != nil {
			// One unit that cannot be described must not hide the others.
			response.Diagnostics = append(response.Diagnostics, unitDiagnostic(req.BasePath, model.UnitFailure{Stack: unit.Stack, Name: unit.Name, Err: problemFor(err, unit.Config.Path)}))
			continue
		}
		response.Components = append(response.Components, discovered)
	}
	response.Diagnostics = append(response.Diagnostics, warningDiagnostics(result.Warnings)...)
	return response, nil
}

func discoveredUnit(req model.Request, unit model.DiscoveredUnit, addresses map[string]*pluginv1.ComponentAddress) (*pluginv1.DiscoveredComponent, error) {
	source, err := relative(req.BasePath, unit.Config.Path)
	if err != nil {
		return nil, err
	}
	module := unit.Config.Source
	moduleSubdir := ""
	if root, subdir, ok := localModuleSubdir(module); ok {
		moduleSubdir = subdir
		local, err := relative(req.BasePath, root)
		if err != nil {
			return nil, err
		}
		module = "./" + local + "//" + subdir
	} else if len(unit.Config.Folders) > 0 && (module == "" || filepath.IsAbs(module)) {
		path := module
		if path == "" {
			path = unit.Config.Folders[0]
		}
		local, err := relative(req.BasePath, path)
		if err != nil {
			return nil, err
		}
		module = "./" + local
	}
	if module == "" {
		return nil, ErrUnsupported
	}
	generation := ""
	if unit.GenerationRoot != "" {
		generation, err = relative(req.BasePath, unit.GenerationRoot)
		if err != nil {
			return nil, err
		}
	}
	workspace := req.Environment["TF_WORKSPACE"]
	if workspace == "" {
		workspace = "default"
	}
	component := map[string]any{
		"source": module, "vars": map[string]any{},
		"provision":    map[string]any{"workdir": map[string]any{"enabled": true}},
		"backend_type": "local", "backend": map[string]any{"local": map[string]any{}},
		"metadata": map[string]any{"terraform_workspace": workspace},
		"settings": map[string]any{
			"terragrunt":    map[string]any{"generation_root": generation},
			"configuration": map[string]any{"watch_excludes": []string{"**/.terragrunt-cache/**", "**/.terragrunt-stack/**", "**/.terraform/**", "**/.atmos/**"}},
		},
	}
	if moduleSubdir != "" {
		component["metadata"].(map[string]any)["source_subdir"] = moduleSubdir
	}
	discoveryParity(component, &unit.Config)
	config, err := encode(component)
	if err != nil {
		return nil, err
	}
	watched, err := sources(req.BasePath, &unit.Config)
	if err != nil {
		return nil, err
	}
	instance := &pluginv1.DiscoveredComponent{Address: addresses[unit.Config.Path], Provider: "terragrunt", Engine: unitEngine(req, &unit.Config), Source: source, Configuration: config, Sources: watched}
	for _, path := range unit.Config.Dependencies {
		target := addresses[path]
		if target == nil {
			// The target is outside this evaluation (not discovered, or imported on its own).
			// Name it the way discovery would, so the host can decide whether it exists.
			var err error
			if target, err = derivedAddress(req.BasePath, path); err != nil {
				return nil, err
			}
		}
		kind := pluginv1.ComponentDependency_ORDER
		if slices.Contains(unit.Config.OutputDependencies, path) {
			kind = pluginv1.ComponentDependency_OUTPUT
		}
		instance.Dependencies = append(instance.Dependencies, &pluginv1.ComponentDependency{Target: target, Kind: kind})
	}
	return instance, nil
}

func localModuleSubdir(source string) (root, subdir string, ok bool) {
	volume := filepath.VolumeName(source)
	rest := source[len(volume):]
	if strings.ContainsAny(rest, "?:") {
		return "", "", false
	}
	index := strings.Index(filepath.ToSlash(rest), "//")
	if index <= 0 {
		return "", "", false
	}
	root = volume + rest[:index]
	first := strings.SplitN(strings.TrimPrefix(root, "./"), "/", 2)[0]
	if strings.Contains(first, ".") && !strings.HasPrefix(root, ".") && !filepath.IsAbs(root) {
		return "", "", false
	}
	return root, rest[index+2:], true
}

// importPaths adds explicit files or directory roots to the same scoped inventory.
// No HCL evaluation occurs while determining which roots were requested.
func importPaths(request *pluginv1.DiscoverComponentsRequest) ([]string, error) {
	paths := slices.Clone(request.IncludePaths)
	for _, uri := range request.Imports {
		path, found := strings.CutPrefix(uri, "terragrunt://")
		if !found || !filepath.IsLocal(path) || strings.ContainsAny(path, "\\?#*[{") {
			return nil, ErrUnsupported
		}
		// Match the engine's clean relative unit paths, whatever spelling the import used.
		path = filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
		info, err := os.Stat(filepath.Join(request.RootDirectory, filepath.FromSlash(path)))
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			path = directoryPattern(path)
		} else if info.Name() != "terragrunt.hcl" && info.Name() != "terragrunt.stack.hcl" {
			return nil, ErrUnsupported
		}
		paths = append(paths, path)
	}
	if len(paths) == 0 {
		return nil, ErrUnsupported
	}
	return paths, nil
}

// directoryPattern matches every unit below an imported directory. The project root
// ("terragrunt://." or "terragrunt://./") has no prefix: "./**" would not match the
// project-relative unit paths discovery compares against.
func directoryPattern(path string) string {
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(path)))
	if clean == "." {
		return "**"
	}
	return clean + "/**"
}

// derivedAddress names a unit that was not part of the evaluation: the component is its
// directory name and the stack is the directory above it, relative to the project.
func derivedAddress(base, path string) (*pluginv1.ComponentAddress, error) {
	dir, err := relative(base, filepath.Dir(path))
	if err != nil {
		return nil, err
	}
	stack, name := "default", "root"
	if dir != "." {
		name = filepath.Base(dir)
		if parent := filepath.ToSlash(filepath.Dir(dir)); parent != "." {
			stack = parent
		}
	}
	return &pluginv1.ComponentAddress{Stack: stack, Component: name, ComponentType: "terraform"}, nil
}
