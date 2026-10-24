package function

import (
	"context"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/function/parser"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/cloudposse/atmos/pkg/utils"
)

// StoreFunction implements the store function for retrieving values from configured stores.
type StoreFunction struct {
	BaseFunction
}

// NewStoreFunction creates a new store function handler.
func NewStoreFunction() *StoreFunction {
	defer perf.Track(nil, "function.NewStoreFunction")()

	return &StoreFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagStore,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// Execute processes the store function.
// Usage:
//
//	!store store_name stack component key
//	!store store_name component key            - Uses current stack
//	!store store_name stack component key | default "value"
//	!store store_name stack component key | query ".foo.bar"
func (f *StoreFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.StoreFunction.Execute")()

	log.Debug("Executing store function", "args", args)

	if execCtx == nil || execCtx.AtmosConfig == nil {
		return nil, fmt.Errorf("%w: store function requires AtmosConfig", ErrExecutionFailed)
	}

	// Parse parameters.
	params, err := parseStoreParams(args, execCtx.Stack)
	if err != nil {
		return nil, err
	}
	if err := ensureStoreFunctionCanRead(execCtx, params.storeName); err != nil {
		return nil, err
	}

	// Retrieve the store from atmosConfig.
	store := execCtx.AtmosConfig.Stores[params.storeName]
	if store == nil {
		return nil, fmt.Errorf("%w: store '%s' not found", ErrExecutionFailed, params.storeName)
	}

	// Retrieve the value from the store.
	value, err := store.Get(params.stack, params.component, params.key)
	if err != nil {
		if params.defaultValue != nil {
			return *params.defaultValue, nil
		}
		return nil, fmt.Errorf("%w: failed to get key '%s': %w", ErrExecutionFailed, params.key, err)
	}

	// Execute the YQ expression if provided.
	if params.query != "" {
		value, err = utils.EvaluateYqExpression(execCtx.AtmosConfig, value, params.query)
		if err != nil {
			return nil, err
		}
	}

	return value, nil
}

// StoreGetFunction implements the store.get function for retrieving arbitrary keys from stores.
type StoreGetFunction struct {
	BaseFunction
}

// NewStoreGetFunction creates a new store.get function handler.
func NewStoreGetFunction() *StoreGetFunction {
	defer perf.Track(nil, "function.NewStoreGetFunction")()

	return &StoreGetFunction{
		BaseFunction: BaseFunction{
			FunctionName:    TagStoreGet,
			FunctionAliases: nil,
			FunctionPhase:   PostMerge,
		},
	}
}

// retrieveFromStore gets a value from the store and applies defaults if needed.
func retrieveFromStore(s store.Store, params *storeGetParams) (any, error) {
	value, err := s.GetKey(params.key)
	if err != nil {
		if params.defaultValue != nil {
			return *params.defaultValue, nil
		}
		return nil, fmt.Errorf("%w: failed to get key '%s': %w", ErrExecutionFailed, params.key, err)
	}

	// Check if the retrieved value is nil and use default if provided.
	if value == nil && params.defaultValue != nil {
		return *params.defaultValue, nil
	}

	return value, nil
}

// Execute processes the store.get function.
// Usage:
//
//	!store.get store_name key
//	!store.get store_name key | default "value"
//	!store.get store_name key | query ".foo.bar"
func (f *StoreGetFunction) Execute(ctx context.Context, args string, execCtx *ExecutionContext) (any, error) {
	defer perf.Track(nil, "function.StoreGetFunction.Execute")()

	log.Debug("Executing store.get function", "args", args)

	if execCtx == nil || execCtx.AtmosConfig == nil {
		return nil, fmt.Errorf("%w: store.get function requires AtmosConfig", ErrExecutionFailed)
	}

	// Parse parameters.
	params, err := parseStoreGetParams(args)
	if err != nil {
		return nil, err
	}
	if err := ensureStoreFunctionCanRead(execCtx, params.storeName); err != nil {
		return nil, err
	}

	// Retrieve the store from atmosConfig.
	store := execCtx.AtmosConfig.Stores[params.storeName]
	if store == nil {
		return nil, fmt.Errorf("%w: store '%s' not found", ErrExecutionFailed, params.storeName)
	}

	// Retrieve the value from the store.
	value, err := retrieveFromStore(store, params)
	if err != nil {
		return nil, err
	}

	// Execute the YQ expression if provided.
	if params.query != "" {
		return utils.EvaluateYqExpression(execCtx.AtmosConfig, value, params.query)
	}

	return value, nil
}

// ensureStoreFunctionCanRead guards the !store and !store.get functions against
// reading from a store marked secret: true, returning ErrStoreIsSecret so callers
// are directed to !secret instead.
func ensureStoreFunctionCanRead(execCtx *ExecutionContext, storeName string) error {
	if cfg, ok := execCtx.AtmosConfig.StoresConfig[storeName]; ok && cfg.Secret {
		return fmt.Errorf("%w: store %q", errUtils.ErrStoreIsSecret, storeName)
	}
	return nil
}

// storeParams holds parsed parameters for the store function.
type storeParams struct {
	storeName    string
	stack        string
	component    string
	key          string
	query        string
	defaultValue *string
}

// storeGetParams holds parsed parameters for the store.get function.
type storeGetParams struct {
	storeName    string
	key          string
	query        string
	defaultValue *string
}

// parseStoreParams parses the arguments for the store function.
func parseStoreParams(args string, currentStack string) (*storeParams, error) {
	parsed, err := parser.ParseStore(args)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArguments, err)
	}
	stack := parsed.Stack
	if stack == "" {
		stack = currentStack
	}
	return &storeParams{storeName: parsed.Store, stack: stack, component: parsed.Component, key: parsed.Key, defaultValue: parsed.Default, query: parsed.Query}, nil
}

// parseStoreGetParams parses the arguments for the store.get function.
func parseStoreGetParams(args string) (*storeGetParams, error) {
	parsed, err := parser.ParseStoreGet(args)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrInvalidArguments, err)
	}
	return &storeGetParams{storeName: parsed.Store, key: parsed.Key, defaultValue: parsed.Default, query: parsed.Query}, nil
}
