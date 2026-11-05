package script

import "context"

// ValueMap is a read-only configuration mapping. Get resolves a value only when
// it is accessed, allowing hosts to track dependencies between computed values.
type ValueMap interface {
	Keys() []string
	Get(string) (any, bool, error)
}

// Evaluation describes a configuration function body and its source location.
// It deliberately supplies no process, filesystem, authentication, or step host.
type Evaluation struct {
	Source, Filename string
	Line             int32
	Context          ValueMap
}

// ValueEvaluator is the optional, typed configuration capability of an engine.
// Results contain only native configuration values, never interpreter objects.
type ValueEvaluator interface {
	EvaluateValue(context.Context, Evaluation) (any, error)
}
