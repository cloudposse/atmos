package create

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	generatorui "github.com/cloudposse/atmos/pkg/generator/ui"
	"github.com/cloudposse/atmos/pkg/schema"
)

func resolveRequest(config *schema.AtmosConfiguration, opts *Options) (*request, error) {
	if opts.File != "" && opts.Target != "" {
		return nil, fmt.Errorf("%w: --file and --target are mutually exclusive", errUtils.ErrInvalidFlag)
	}
	req := &request{config: config, opts: opts}
	var err error
	if opts.Target == "" {
		req.root, req.file, err = resolveManifest(config, opts.File)
		if err == nil {
			err = rejectExisting(req.root, req.file)
		}
	} else {
		req.root, req.target, err = resolveDirectory(config, opts.Target)
		// The built-in stack scaffold remains usable with a directory destination.
		req.file = "stack.yaml"
	}
	return req, err
}

func resolveDirectory(config *schema.AtmosConfiguration, target string) (string, string, error) {
	root, err := filepath.Abs(config.BasePathAbsolute)
	if err != nil {
		return "", "", err
	}
	absolute := target
	if !filepath.IsAbs(absolute) {
		absolute = filepath.Join(root, absolute)
	}
	relative, err := filepath.Rel(root, absolute)
	if err != nil || !filepath.IsLocal(relative) {
		return "", "", fmt.Errorf("%w: --target must be inside project base path %s", errUtils.ErrInvalidFlag, root)
	}
	underStacks, err := filepath.Rel(stacksBasePath(config), absolute)
	if err != nil || !filepath.IsLocal(underStacks) {
		return "", "", fmt.Errorf("%w: --target must be under stacks.base_path", errUtils.ErrInvalidConfig)
	}
	if err := validateDirectoryPath(absolute); err != nil {
		return "", "", err
	}
	return root, relative, nil
}

func validateDirectoryPath(path string) error {
	if info, err := os.Stat(path); err == nil && !info.IsDir() {
		return fmt.Errorf("%w: --target must be a directory", errUtils.ErrInvalidFlag)
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("%w: %w", errUtils.ErrReadFile, err)
	}
	return nil
}

func (r *request) destination() string {
	if r.target != "" {
		return r.target
	}
	return r.file
}

func (r *request) renderRoot() string {
	return filepath.Join(r.root, r.target)
}

// normalizeOutputs validates target-relative paths before rebasing them for project validation.
func (r *request) normalizeOutputs(plan *generatorui.CreationPlan) error {
	if r.target == "" {
		return nil
	}
	if err := engine.ValidateCreation(r.renderRoot(), plan.Files); err != nil {
		return err
	}
	for i := range plan.Files {
		plan.Files[i].Path = filepath.Join(r.target, plan.Files[i].Path)
	}
	return nil
}

func (r *request) checkStackOutputs(template string, plan *generatorui.CreationPlan) error {
	manifest, err := r.declaredManifest(plan)
	if err != nil {
		return err
	}
	if r.target != "" {
		r.file = manifest
		documents, manifests, err := validateDirectoryOutputs(r.config, r.root, plan.Files, manifest)
		if err != nil {
			return err
		}
		r.manifests = manifests
		return verifyReservedInputsAcross(template, r.opts, r.declared, documents)
	}
	if filepath.Clean(manifest) != filepath.Clean(r.file) {
		return fmt.Errorf("%w: scaffold stack output %q must match --file %q (use .Config.stack_file)", errUtils.ErrInvalidConfig, manifest, r.file)
	}
	document, err := validateRendered(r.config, r.root, plan.Files, r.file)
	if err != nil {
		return err
	}
	r.manifests = []string{r.file}
	return verifyReservedInputs(template, r.opts, r.declared, document)
}

func (r *request) declaredManifest(plan *generatorui.CreationPlan) (string, error) {
	if plan.Config.Spec.Stack == nil {
		if r.target != "" {
			return "", nil
		}
		return "", fmt.Errorf("%w: scaffold must declare spec.stack.manifest for --file", errUtils.ErrInvalidConfig)
	}
	output, err := engine.NewProcessor().ProcessTemplate(plan.Config.Spec.Stack.Manifest, r.renderRoot(), plan.Config, plan.Values)
	if err != nil {
		return "", err
	}
	if output == "" || !filepath.IsLocal(output) {
		return "", fmt.Errorf("%w: scaffold stack manifest must be a relative output path", errUtils.ErrInvalidConfig)
	}
	if ext := filepath.Ext(output); r.target != "" && ext != ".yaml" && ext != ".yml" {
		return "", fmt.Errorf("%w: scaffold stack manifest must end in .yaml or .yml", errUtils.ErrInvalidConfig)
	}
	return filepath.Join(r.target, output), nil
}

func (p *Plan) generationRoot() string { return filepath.Join(p.Root, p.Target) }

func (p *Plan) destination() string {
	if p.Target != "" {
		return p.Target
	}
	return p.Manifest
}

// validateCreation rechecks both the project and narrower directory boundary, including after hooks.
func (p *Plan) validateCreation() error {
	if err := p.validateDirectoryRecord(); err != nil {
		return err
	}
	if p.Target != "" {
		var files []engine.File
		for _, file := range p.Generation.Files {
			if isRecordPath(file.Path) {
				continue
			}
			relative, err := filepath.Rel(p.Target, file.Path)
			if err != nil {
				return err
			}
			file.Path = relative
			files = append(files, file)
		}
		if err := engine.ValidateCreation(p.generationRoot(), files); err != nil {
			return err
		}
	}
	return engine.ValidateCreation(p.Root, withoutPath(p.Generation.Files, p.staleRecord))
}
