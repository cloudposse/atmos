package produce

import (
	"context"
	"fmt"
	"os"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Artifact kinds supplied by the kind-specific producers.
const (
	// KindScaffold is the kind of a scaffold template package.
	KindScaffold = "scaffold"

	// ComponentTerraform is the component type of a Terraform root module.
	ComponentTerraform = "terraform"

	// KindComponentTerraform is the kind of a Terraform component package.
	KindComponentTerraform = "component/" + ComponentTerraform
)

// kindProfile is the kind-specific behavior layered over the shared preparation pipeline: the
// kind name, visible default exclusion rules, and an inspection that validates the source and
// supplies producer metadata. The pipeline itself (snapshot, reserved distribution directory,
// documents, Record sealing, and packaging) is shared with the generic files producer.
type kindProfile struct {
	kind    string
	rules   []string
	inspect func(ctx context.Context, source string) (ProducerMetadata, error)
}

//go:generate mockgen -package produce -destination mock_scaffold_validator.go github.com/cloudposse/atmos/pkg/artifact/produce ScaffoldValidator

// ScaffoldValidator validates a scaffold source directory and returns the metadata the scaffold
// declares. It validates configuration only: it never generates output and never runs scaffold hooks.
//
// The implementation lives above this package (it needs the scaffold manifest loader), so the
// scaffold command registers it with [RegisterScaffoldValidator] when the binary starts.
type ScaffoldValidator interface {
	// ValidateScaffold checks the scaffold at dir and returns its declared name, version, and
	// description. An invalid scaffold returns an error wrapping [errUtils.ErrArtifactScaffoldInvalid].
	ValidateScaffold(ctx context.Context, dir string) (ProducerMetadata, error)
}

// ScaffoldValidatorFunc adapts a function to a [ScaffoldValidator].
type ScaffoldValidatorFunc func(ctx context.Context, dir string) (ProducerMetadata, error)

// ValidateScaffold calls the function.
func (f ScaffoldValidatorFunc) ValidateScaffold(ctx context.Context, dir string) (ProducerMetadata, error) {
	defer perf.Track(nil, "produce.ScaffoldValidatorFunc.ValidateScaffold")()

	return f(ctx, dir)
}

var (
	registeredValidatorMu sync.RWMutex
	registeredValidator   ScaffoldValidator
)

// RegisterScaffoldValidator installs the process-wide scaffold validator used by scaffold
// producers that were not given one with [WithScaffoldValidator]. Registering nil removes it.
func RegisterScaffoldValidator(validator ScaffoldValidator) {
	defer perf.Track(nil, "produce.RegisterScaffoldValidator")()

	registeredValidatorMu.Lock()
	defer registeredValidatorMu.Unlock()
	registeredValidator = validator
}

func currentScaffoldValidator() ScaffoldValidator {
	registeredValidatorMu.RLock()
	defer registeredValidatorMu.RUnlock()
	return registeredValidator
}

// WithScaffoldValidator injects the scaffold validator, taking precedence over the registered one.
func WithScaffoldValidator(validator ScaffoldValidator) Option {
	defer perf.Track(nil, "produce.WithScaffoldValidator")()

	return func(p *Producer) { p.scaffoldValidator = validator }
}

// NewScaffold returns the scaffold producer (kind "scaffold"). It validates the scaffold
// configuration at the source root before it packages anything, takes the artifact name, version,
// and description from the scaffold manifest when they are not given explicitly, and treats the
// root README as ordinary payload while the reserved .atmos/artifact directory stays distribution
// metadata. It excludes nothing by default: a scaffold ships exactly what its author selected.
func NewScaffold(opts ...Option) *Producer {
	defer perf.Track(nil, "produce.NewScaffold")()

	p := New(opts...)
	p.profile = &kindProfile{kind: KindScaffold, inspect: p.inspectScaffold}
	return p
}

// NewTerraformComponent returns the component/terraform producer. It excludes Terraform working
// directories, state files, and saved plans by default (see [TerraformExclusions]) and reports each
// exclusion with the plan. Publishing never runs Terraform and needs no stack. Scripts keep their
// executable bits and `.terraform.lock.hcl` stays in the payload.
func NewTerraformComponent(opts ...Option) *Producer {
	defer perf.Track(nil, "produce.NewTerraformComponent")()

	p := New(opts...)
	p.profile = &kindProfile{
		kind:    KindComponentTerraform,
		rules:   TerraformExclusions(),
		inspect: inspectComponent(ComponentTerraform),
	}
	return p
}

// RegisterDefaults registers every built-in producer: the generic files producer and the scaffold
// and component/terraform producers. Options apply to each of them.
func RegisterDefaults(registry *artifact.ProducerRegistry, opts ...Option) error {
	defer perf.Track(nil, "produce.RegisterDefaults")()

	for _, producer := range []*Producer{New(opts...), NewScaffold(opts...), NewTerraformComponent(opts...)} {
		if err := registry.Register(producer); err != nil {
			return err
		}
	}
	return nil
}

// inspectScaffold validates the scaffold source and returns the metadata it declares.
func (p *Producer) inspectScaffold(ctx context.Context, source string) (ProducerMetadata, error) {
	if err := requireDirectory(KindScaffold, source); err != nil {
		return ProducerMetadata{}, err
	}
	validator := p.scaffoldValidator
	if validator == nil {
		validator = currentScaffoldValidator()
	}
	if validator == nil {
		return ProducerMetadata{}, fmt.Errorf("%w: no scaffold validator is registered, so a scaffold cannot be published from this binary", errUtils.ErrArtifactProducerUnavailable)
	}
	meta, err := validator.ValidateScaffold(ctx, source)
	if err != nil {
		return ProducerMetadata{}, err
	}
	return meta, nil
}

// componentKindData is the typed kind data of a component Record. It records the component subtype
// in portable metadata, next to the kind name.
type componentKindData struct {
	ComponentType string `json:"component_type"`
}

// inspectComponent checks that the source is a directory and records the component type. A
// component carries no name or version that is safe to reuse: the name in component.yaml names the
// vendoring manifest, and its source version is the upstream version, so both stay explicit.
func inspectComponent(componentType string) func(context.Context, string) (ProducerMetadata, error) {
	return func(_ context.Context, source string) (ProducerMetadata, error) {
		if err := requireDirectory("component/"+componentType, source); err != nil {
			return ProducerMetadata{}, err
		}
		data, err := artifact.NewKindData(componentKindData{ComponentType: componentType})
		if err != nil {
			return ProducerMetadata{}, err
		}
		return ProducerMetadata{KindData: data}, nil
	}
}

// requireDirectory fails unless source is a directory (a symlink to one is accepted, like every
// top-level source).
func requireDirectory(kind, source string) error {
	info, err := os.Stat(source)
	if err != nil {
		return fmt.Errorf("%w: %s source %q: %w", errUtils.ErrArtifactPackagingInvalid, kind, source, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: a %s source must be a directory, not the file %q", errUtils.ErrArtifactPackagingInvalid, kind, source)
	}
	return nil
}

// overlayMetadata returns base with every non-empty value of over applied. Labels merge, and over
// wins on a shared key.
func overlayMetadata(base, over *ProducerMetadata) ProducerMetadata {
	merged := *base
	merged.Name = firstNonEmpty(over.Name, base.Name)
	merged.Version = firstNonEmpty(over.Version, base.Version)
	merged.Description = firstNonEmpty(over.Description, base.Description)
	merged.Labels = mergeLabels(base.Labels, over.Labels)
	if len(over.KindData) > 0 {
		merged.KindData = over.KindData
	}
	return merged
}
