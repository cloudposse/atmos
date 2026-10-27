// Package create prepares scaffold-backed stack manifests without deploying resources.
package create

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

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

// Renderer is the shared scaffold questionnaire and rendering API.
type Renderer interface {
	SetEdition(string)
	PlanCreationQuiet(*templates.Configuration, string, bool, map[string]any) (*generatorui.CreationPlan, error)
}

// Options selects a scaffold and its stack-specific inputs.
type Options struct {
	File, Name, Template, Ref string
	Imports, Components       []string
	Values                    map[string]any
	Defaults                  bool
}

// Plan contains the fully rendered files and the scaffold hooks that accompany them.
type Plan struct {
	edition        string
	Root, Manifest string
	Generation     *generatorui.CreationPlan
	Hooks          map[string]hooks.Hook
}

// Prepare renders and validates a new stack before any generation hook or write.
func Prepare(config *schema.AtmosConfiguration, opts *Options, renderer Renderer) (*Plan, error) {
	defer perf.Track(config, "create.Prepare")()

	root, file, err := resolveManifest(config, opts.File)
	if err != nil {
		return nil, err
	}
	values, err := stackValues(config, opts, file)
	if err != nil {
		return nil, err
	}
	name := opts.Template
	if name == "" {
		name = "stack"
	}
	template, cleanup, err := catalog.Resolve(name, opts.Ref, scaffoldConfigDir(config, root), filepath.Join(root, file))
	if err != nil {
		return nil, err
	}
	defer cleanup()
	// Reject incompatible recipes before asking questions or rendering files.
	_, err = stackDefinition(template)
	if err != nil {
		return nil, err
	}
	renderer.SetEdition(config.Edition)
	generated, err := renderer.PlanCreationQuiet(template, root, opts.Defaults, values)
	if err != nil {
		return nil, err
	}
	return finishPlan(config, root, file, template, generated)
}

func finishPlan(config *schema.AtmosConfiguration, root, file string, template *templates.Configuration, generated *generatorui.CreationPlan) (*Plan, error) {
	output, err := engine.NewProcessor().ProcessTemplate(generated.Config.Spec.Stack.Manifest, root, generated.Config, generated.Values)
	if err != nil {
		return nil, err
	}
	if filepath.Clean(output) != filepath.Clean(file) {
		return nil, fmt.Errorf("%w: scaffold stack output %q must match --file %q (use .Config.stack_file)", errUtils.ErrInvalidConfig, output, file)
	}
	if err := validateManifest(config, generated.Files, file); err != nil {
		return nil, err
	}
	hookMap, err := project.DecodeHooks(generated.Config.Spec.Hooks)
	if err != nil {
		return nil, err
	}
	if err := hooks.ValidateEvents(config.Edition, hookMap); err != nil {
		return nil, err
	}
	record, err := generationRecord(file, template, generated)
	if err != nil {
		return nil, err
	}
	if err := checkGenerationRecordConflict(root, file, record.Path); err != nil {
		return nil, err
	}
	generated.Files = append(generated.Files, record)
	if err := engine.ValidateCreation(root, generated.Files); err != nil {
		return nil, err
	}
	return &Plan{edition: config.Edition, Root: root, Manifest: file, Generation: generated, Hooks: hookMap}, nil
}

// checkGenerationRecordConflict identifies a prior generation using both readable
// manifest and record paths, so recreating a deleted manifest has an actionable remedy.
func checkGenerationRecordConflict(root, manifest, record string) error {
	if _, err := os.Lstat(filepath.Join(root, record)); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return fmt.Errorf("%w: inspect generation record %s: %w", errUtils.ErrFileWrite, record, err)
	}
	return errUtils.Build(errUtils.ErrFileWrite).
		WithExplanationf("Cannot recreate stack manifest `%s`: generation record `%s` already exists", filepath.ToSlash(manifest), filepath.ToSlash(record)).
		WithHint("Check the previous generation's recorded outputs, then remove its generation record and retry").
		Err()
}

func stackDefinition(template *templates.Configuration) (*project.ScaffoldConfig, error) {
	for _, file := range template.Files {
		if file.Path != project.ScaffoldConfigFileName {
			continue
		}
		definition, err := project.LoadScaffoldConfigFromContent(file.Content, project.WithSourceDir(template.IncludeSourceDir()))
		if err != nil {
			return nil, err
		}
		if definition.Spec.Stack == nil {
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

func stackValues(config *schema.AtmosConfiguration, opts *Options, file string) (map[string]any, error) {
	values := make(map[string]any)
	for key, value := range opts.Values {
		if reservedStackInput(key) {
			return nil, fmt.Errorf("%w: %s is reserved; use stack create flags", errUtils.ErrInvalidFlag, key)
		}
		values[key] = value
	}
	components, err := componentStubs(config, opts.Components)
	if err != nil {
		return nil, err
	}
	for _, name := range opts.Imports {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("%w: import cannot be empty", errUtils.ErrInvalidFlag)
		}
	}
	values["stack_file"], values["stack_name"] = filepath.ToSlash(file), opts.Name
	values["stack_imports"], values["stack_components"] = opts.Imports, components
	return values, nil
}

func componentStubs(config *schema.AtmosConfiguration, declarations []string) (map[string]map[string]any, error) {
	components := make(map[string]map[string]any)
	for _, value := range declarations {
		kind, name, ok := strings.Cut(value, ":")
		if !ok || strings.TrimSpace(name) == "" || !knownKind(kind, config.Commands) {
			return nil, fmt.Errorf("%w: component %q must use a supported kind:name", errUtils.ErrInvalidFlag, value)
		}
		if components[kind] == nil {
			components[kind] = make(map[string]any)
		}
		if _, exists := components[kind][name]; exists {
			return nil, fmt.Errorf("%w: duplicate component %q", errUtils.ErrInvalidFlag, value)
		}
		components[kind][name] = map[string]any{"metadata": map[string]any{"enabled": false}, "vars": map[string]any{}}
	}
	return components, nil
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

func generationRecord(file string, template *templates.Configuration, plan *generatorui.CreationPlan) (engine.File, error) {
	outputs := make(map[string]string)
	for _, output := range plan.Files {
		outputs[filepath.ToSlash(output.Path)] = fmt.Sprintf("%x", sha256.Sum256([]byte(output.Content)))
	}
	record := struct {
		Manifest string                  `yaml:"manifest"`
		Template string                  `yaml:"template"`
		Source   string                  `yaml:"source"`
		Revision string                  `yaml:"revision,omitempty"`
		Scaffold *project.ScaffoldConfig `yaml:"scaffold"`
		Outputs  map[string]string       `yaml:"outputs"`
	}{filepath.ToSlash(file), template.Name, template.Source, template.ResolvedRef, plan.Config, outputs}
	snapshot := *plan.Config
	snapshot.Spec.Values = plan.Values
	record.Scaffold = &snapshot
	content, err := yaml.Marshal(record)
	key := fmt.Sprintf("%x", sha256.Sum256([]byte(filepath.ToSlash(file))))
	return engine.File{Path: filepath.Join(".atmos", "stack-generations", key+".yaml"), Content: string(content), Permissions: recordPermissions}, err
}

// Apply runs scaffold hooks around exclusive writes of exactly the previewed bytes.
func (p *Plan) Apply(ctx context.Context, skip func(string) bool) error {
	defer perf.Track(nil, "create.Plan.Apply")()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := engine.ValidateCreation(p.Root, p.Generation.Files); err != nil {
		return err
	}
	input := scaffoldhooks.RunInput{Edition: p.edition, HooksMap: p.Hooks, Event: hooks.BeforeScaffoldGenerate, Answers: p.Generation.Values, Status: "success", SkipHooks: skip, TargetPath: p.Root}
	if err := scaffoldhooks.Run(input); err != nil {
		return err
	}
	err := engine.ApplyCreation(ctx, p.Root, p.Generation.Files)
	input.Event = hooks.AfterScaffoldGenerate
	if err != nil {
		input.Status = "failure"
		input.Err = err
	}
	hookErr := scaffoldhooks.Run(input)
	return errors.Join(err, hookErr)
}
