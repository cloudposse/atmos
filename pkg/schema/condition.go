package schema

import (
	"github.com/go-viper/mapstructure/v2"

	"github.com/cloudposse/atmos/pkg/condition"
)

const (
	ConditionPredicateCI      = condition.PredicateCI
	ConditionPredicateLocal   = condition.PredicateLocal
	ConditionPredicateAlways  = condition.PredicateAlways
	ConditionPredicateNever   = condition.PredicateNever
	ConditionPredicateSuccess = condition.PredicateSuccess
	ConditionPredicateFailure = condition.PredicateFailure
)

var ErrInvalidWhenCondition = condition.ErrInvalidWhenCondition

type (
	Condition              = condition.Condition
	ConditionContext       = condition.Context
	ConditionPredicateFunc = condition.PredicateFunc
	ConditionNode          = condition.Node
)

func RegisterConditionPredicate(name string, fn ConditionPredicateFunc) {
	condition.RegisterPredicate(name, fn)
}

func NewCondition(value any) (Condition, error) {
	return condition.New(value)
}

func MustCondition(value any) Condition {
	return condition.Must(value)
}

func ValidateStepCondition(c Condition) error {
	return condition.ValidateStep(c)
}

func ConditionDecodeHook() mapstructure.DecodeHookFunc {
	return condition.DecodeHook()
}
