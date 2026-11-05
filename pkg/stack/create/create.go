// Package create prepares scaffold-backed stack manifests without deploying resources.
package create

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/generator/catalog"
	"github.com/cloudposse/atmos/pkg/generator/engine"
	"github.com/cloudposse/atmos/pkg/generator/scaffoldhooks"
	"github.com/cloudposse/atmos/pkg/generator/templates"
	generatorui "github.com/cloudposse/atmos/pkg/generator/ui"
	"github.com/cloudposse/atmos/pkg/hooks"
	"github.com/cloudposse/atmos/pkg/perf"
	project "github.com/cloudposse/atmos/pkg/project/config"
	"github.com/cloudposse/atmos/pkg/schema"
)

const recordPermissions = 0o600

// recordDirectory holds the Atmos-owned generation records, keyed by manifest path.
var recordDirectory = filepath.Join(".atmos", "stack-generations")

// Renderer is the shared scaffold questionnaire and rendering API. Stack create reports its own result, so it
// uses the quiet variant that omits the renderer's dry-run report.
type Renderer interface {
	SetEdition(string)
	PlanCreationQuiet(*templates.Configuration, string, bool, map[string]any) (*generatorui.CreationPlan, error)
}

// Options selects a scaffold and its stack-specific inputs.
type Options struct {
	File, Target, Name, Template, Ref string
	Imports, Components               []string
	Values                            map[string]any
	Defaults                          bool
}

// Plan contains the fully rendered files and the scaffold hooks that accompany them.
type Plan struct {
	edition        string
	Root, Manifest string
	// Target is the project-relative directory destination; empty in file mode.
	Target string
	// Manifests lists discoverable generated stack manifests.
	Manifests  []string
	Generation *generatorui.CreationPlan
	Hooks      map[string]hooks.Hook
	// staleRecord is the generation record left behind by a manifest that has since been deleted.
	// The record is Atmos-owned, so Apply replaces it instead of treating it as a conflict.
	staleRecord string
}

// resolveTemplate selects the scaffold for --template and rejects incompatible
// recipes before any questions are asked or files are rendered. The caller must
// call the returned cleanup.
func resolveTemplate(config *schema.AtmosConfiguration, opts *Options, root, file string) (*templates.Configuration, func(), error) {
	noop := func() {}
	name := opts.Template
	if name == "" {
		name = "stack"
	}
	template, cleanup, err := resolveCatalog(config, name, opts.Ref, root, file)
	if err != nil {
		return nil, noop, err
	}
	if err := rejectRefForNonGit(template, opts.Ref); err != nil {
		cleanup()
		return nil, noop, err
	}
	if _, err := stackDefinition(template, opts.Target != ""); err != nil {
		cleanup()
		return nil, noop, err
	}
	return template, cleanup, nil
}

func resolveCatalog(config *schema.AtmosConfiguration, name, ref, root, file string) (*templates.Configuration, func(), error) {
	if config.ScaffoldConfigLoaded || config.Scaffold != nil {
		return catalog.ResolveWithConfig(name, ref, filepath.Join(root, file), config.Scaffold)
	}
	// Compatibility for callers constructing configuration by hand. Loaded
	// configurations mark their catalog as loaded, including an empty one.
	configDir, err := scaffoldConfigDir(config, root)
	if err != nil {
		return nil, func() {}, err
	}
	return catalog.Resolve(name, ref, configDir, filepath.Join(root, file))
}

// Prepare renders and validates a new stack before any generation hook or write.
func Prepare(config *schema.AtmosConfiguration, opts *Options, renderer Renderer) (*Plan, error) {
	defer perf.Track(config, "create.Prepare")()

	req, err := resolveRequest(config, opts)
	if err != nil {
		return nil, err
	}
	declared, err := parseComponents(config, opts.Components)
	if err != nil {
		return nil, err
	}
	values, err := stackValues(opts, declared, req.file)
	if err != nil {
		return nil, err
	}
	template, cleanup, err := resolveTemplate(config, opts, req.root, req.destination())
	if err != nil {
		return nil, err
	}
	defer cleanup()
	renderer.SetEdition(config.Edition)
	generated, err := renderer.PlanCreationQuiet(template, req.renderRoot(), opts.Defaults, values)
	if err != nil {
		return nil, translateRenderError(err)
	}
	req.declared = declared
	if err := req.normalizeOutputs(generated); err != nil {
		return nil, err
	}
	return finishPlan(req, template, generated)
}

// request carries what Prepare resolved before rendering.
type request struct {
	config     *schema.AtmosConfiguration
	root, file string
	target     string
	manifests  []string
	opts       *Options
	declared   []declaredComponent
}

func finishPlan(req *request, template *templates.Configuration, generated *generatorui.CreationPlan) (*Plan, error) {
	hookMap, err := checkRendered(req, template, generated)
	if err != nil {
		return nil, err
	}
	record, err := requestRecord(req, template, generated)
	if err != nil {
		return nil, err
	}
	stale, err := staleRecordPath(req.root, &record)
	if err != nil {
		return nil, err
	}
	generated.Files = append(generated.Files, record)
	if err := engine.ValidateCreation(req.root, withoutPath(generated.Files, stale)); err != nil {
		return nil, err
	}
	plan := &Plan{edition: req.config.Edition, Root: req.root, Manifest: req.file, Target: req.target, Manifests: req.manifests, Generation: generated, Hooks: hookMap, staleRecord: stale}
	if err := plan.validateDirectoryRecord(); err != nil {
		return nil, err
	}
	return plan, nil
}

// checkRendered validates everything the scaffold rendered, and the project it lands in, before anything is written.
func checkRendered(req *request, template *templates.Configuration, generated *generatorui.CreationPlan) (map[string]hooks.Hook, error) {
	if err := req.checkStackOutputs(template.Name, generated); err != nil {
		return nil, err
	}
	hookMap, err := project.DecodeHooks(generated.Config.Spec.Hooks)
	if err != nil {
		return nil, err
	}
	if err := hooks.ValidateEvents(req.config.Edition, hookMap); err != nil {
		return nil, err
	}
	if err := engine.ValidateCreation(req.root, generated.Files); err != nil {
		return nil, err
	}
	// Prove the project still processes with the new stacks before any hook runs or file is written.
	return hookMap, preflightProject(req.config, generated.Files, req.root)
}

func stackDefinition(template *templates.Configuration, directory bool) (*project.ScaffoldConfig, error) {
	for _, file := range template.Files {
		if file.Path != project.ScaffoldConfigFileName {
			continue
		}
		definition, err := project.LoadScaffoldConfigFromContent(file.Content, project.WithSourceDir(template.IncludeSourceDir()))
		if err != nil {
			return nil, err
		}
		if definition.Spec.Stack == nil && !directory {
			break
		}
		for i := range definition.Spec.Fields {
			field := &definition.Spec.Fields[i]
			if reservedStackInput(field.Name) {
				return nil, fmt.Errorf("%w: stack_file, stack_name, stack_imports and stack_components are reserved inputs", errUtils.ErrInvalidConfig)
			}
		}
		return definition, nil
	}
	return nil, fmt.Errorf("%w: scaffold %q must declare spec.stack.manifest", errUtils.ErrInvalidConfig, template.Name)
}

func reservedStackInput(name string) bool {
	switch name {
	case "stack_file", "stack_name", "stack_imports", "stack_components":
		return true
	default:
		return false
	}
}

func stackValues(opts *Options, components []declaredComponent, file string) (map[string]any, error) {
	values := make(map[string]any)
	for key, value := range opts.Values {
		if reservedStackInput(key) {
			return nil, fmt.Errorf("%w: %s is reserved; use stack create flags", errUtils.ErrInvalidFlag, key)
		}
		values[key] = value
	}
	imports := make([]string, 0, len(opts.Imports))
	for _, name := range opts.Imports {
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, fmt.Errorf("%w: import cannot be empty", errUtils.ErrInvalidFlag)
		}
		imports = append(imports, name)
	}
	values["stack_file"], values["stack_name"] = filepath.ToSlash(file), opts.Name
	values["stack_imports"], values["stack_components"] = imports, componentStubs(components)
	return values, nil
}

func knownKind(kind string, commands []schema.Command) bool {
	// These legacy component types predate the provider registry.
	if kind == "terraform" || kind == "helmfile" || kind == "packer" {
		return true
	}
	if _, ok := component.GetProvider(kind); ok {
		return true
	}
	return knownCustomKind(kind, commands)
}

func knownCustomKind(kind string, commands []schema.Command) bool {
	for i := range commands {
		command := &commands[i]
		if command.Component != nil && command.Component.Type == kind {
			return true
		}
		if knownCustomKind(kind, command.Commands) {
			return true
		}
	}
	return false
}

// Apply runs scaffold hooks around exclusive writes of exactly the previewed bytes.
func (p *Plan) Apply(ctx context.Context, skip func(string) bool) error {
	defer perf.Track(nil, "create.Plan.Apply")()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := p.validateCreation(); err != nil {
		return err
	}
	input := scaffoldhooks.RunInput{Edition: p.edition, HooksMap: p.Hooks, Event: hooks.BeforeScaffoldGenerate, Answers: p.Generation.Values, Status: "success", SkipHooks: skip, TargetPath: p.generationRoot()}
	if err := scaffoldhooks.Run(input); err != nil {
		return err
	}
	err := p.applyFiles(ctx)
	input.Event = hooks.AfterScaffoldGenerate
	if err != nil {
		input.Status, input.Err = "failure", err
	}
	hookErr := scaffoldhooks.Run(input)
	if err == nil && hookErr != nil {
		return fmt.Errorf("generation completed for %s; generated files and record were retained; after hook failed: %w", p.destination(), hookErr)
	}
	return errors.Join(err, hookErr)
}

// applyFiles revalidates after before-hooks and preserves rollback of a superseded record.
func (p *Plan) applyFiles(ctx context.Context) error {
	if err := p.validateCreation(); err != nil {
		return err
	}
	finish, err := p.stageStaleRecord()
	if err != nil {
		return err
	}
	err = engine.ApplyCreation(ctx, p.Root, p.Generation.Files)
	return errors.Join(err, finish(err == nil))
}
