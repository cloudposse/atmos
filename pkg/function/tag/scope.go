package tag

import (
	"slices"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Context identifies the runtime consumer of an authored YAML value. A tag's
// presence in All does not imply that every consumer evaluates it.
type Context string

const (
	ConfigContext              Context = "CLI configuration"
	MappingKeyContext          Context = "mapping keys"
	StackContext               Context = "stack values"
	StackStructuralContext     Context = "stack structural fields consumed before value evaluation"
	StackUnknownContext        Context = "unclassified stack fields"
	StackCopiedContext         Context = "literal stack data"
	ImportContext              Context = "stack import paths"
	WorkflowContext            Context = "workflow fields"
	WorkflowInteractiveContext Context = "workflow interactive fields"
	ConditionContext           Context = "workflow conditions"
	StackConditionContext      Context = "stack conditions"
	ConfigConditionContext     Context = "CLI configuration conditions"
	VendorContext              Context = "vendor manifests"
	ScaffoldContext            Context = "scaffold manifests"
	SkillRefContext            Context = "ai.skills.<name>.ref"
)

// Shape describes authored node types accepted by a function.
type Shape uint8

const (
	Scalar Shape = 1 << iota
	Sequence
	Mapping
)

// Availability describes syntax and evaluation timing for one consumer. Empty
// Phase means that the consumer has no evaluator for this function. Conditions
// and version refs are field-specific consumers, not general document support.
type Availability struct {
	NativeTag  bool
	StringForm bool
	Shapes     Shape
	Phase      string
}

// Scope returns a function's syntax and timing for a particular consumer.
// Names may include the YAML prefix. Unsupported combinations return zero values.
func Scope(name string, context Context) Availability {
	defer perf.Track(nil, "tag.Scope")()
	name = FromYAML(name)
	if !IsValid(name) {
		return Availability{}
	}
	a := fieldScope(name, context)
	if !a.NativeTag && !a.StringForm {
		return Availability{}
	}
	return a
}

func fieldScope(name string, context Context) Availability {
	switch context {
	case SkillRefContext:
		if name == Version {
			return Availability{NativeTag: true, Shapes: Scalar, Phase: "skill source resolution"}
		}
		return configScope(name)
	case ConfigConditionContext:
		if name == CEL {
			return Availability{StringForm: true, Shapes: Scalar, Phase: "condition evaluation"}
		}
		return configScope(name)
	case StackConditionContext:
		if name == CEL {
			return celScope()
		}
		return stackScope(name)
	case ConditionContext:
		if name == CEL {
			return celScope()
		}
		return workflowScope(name, false)
	default:
		return documentScope(name, context)
	}
}

func documentScope(name string, context Context) Availability {
	switch context {
	case ConfigContext:
		return configScope(name)
	case StackContext:
		return stackScope(name)
	case ScaffoldContext:
		return Availability{NativeTag: IsScaffoldYAML(ToYAML(name)), Shapes: Scalar, Phase: "scaffold manifest loading"}
	case StackStructuralContext, StackUnknownContext, StackCopiedContext:
		return stackLoadingScope(name)
	case ImportContext:
		return Availability{NativeTag: name == Include || name == IncludeRaw || name == Literal, Shapes: Scalar, Phase: "file loading, before import resolution"}
	case WorkflowContext:
		return workflowScope(name, false)
	case WorkflowInteractiveContext:
		return workflowScope(name, true)
	}
	// Vendor manifests use plain YAML decoding; mapping keys have no deferred evaluator.
	return Availability{}
}

func configScope(name string) Availability {
	if name == Append {
		return Availability{NativeTag: true, Shapes: Sequence, Phase: "configuration merging"}
	}
	return Availability{NativeTag: IsAtmosConfigYAML(ToYAML(name)), Shapes: Scalar, Phase: "configuration loading"}
}

func stackScope(name string) Availability {
	a := Availability{NativeTag: name != CEL, StringForm: slices.Contains(StringFormYAML(), ToYAML(name)), Shapes: Scalar, Phase: "after stack merging and templates"}
	switch name {
	case Include, IncludeRaw:
		a.Phase = "file loading, before merging"
	case Append:
		a.Shapes, a.Phase = Sequence, "stack merging"
	case Literal:
		a.Shapes, a.Phase = Scalar|Sequence|Mapping, "file loading; scalar protected through evaluation"
	case CILabels:
		a.Shapes = Scalar | Sequence
	}
	return a
}

func workflowScope(name string, interactive bool) Availability {
	if interactive && (name == Env || name == Exec) {
		return Availability{NativeTag: true, StringForm: true, Shapes: Scalar, Phase: "before workflow execution"}
	}
	return Availability{NativeTag: name == Include || name == IncludeRaw || name == Literal, Shapes: Scalar, Phase: "workflow manifest loading"}
}

func celScope() Availability {
	return Availability{NativeTag: true, StringForm: true, Shapes: Scalar, Phase: "condition evaluation"}
}

// Contexts returns the supported consumer categories in documentation order.
func Contexts() []Context {
	defer perf.Track(nil, "tag.Contexts")()
	return []Context{
		MappingKeyContext, ConfigContext, SkillRefContext, ConfigConditionContext, StackContext, StackStructuralContext, StackUnknownContext, StackCopiedContext, ImportContext,
		WorkflowContext, WorkflowInteractiveContext, ConditionContext, StackConditionContext, VendorContext, ScaffoldContext,
	}
}

// stackLoadingScope covers directives handled before component selection or value evaluation.
func stackLoadingScope(name string) Availability {
	switch name {
	case Include, IncludeRaw, Literal, Append:
		return stackScope(name)
	}
	return Availability{}
}
