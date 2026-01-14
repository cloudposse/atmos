package function

import (
	"context"
	"fmt"
	"strings"

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
	// Split on pipe to separate store parameters and options.
	parts := strings.Split(args, "|")
	storePart := strings.TrimSpace(parts[0])

	// Extract default value and query from pipe parts.
	var defaultValue *string
	var query string
	if len(parts) > 1 {
		var err error
		defaultValue, query, err = extractPipeOptions(parts[1:])
		if err != nil {
			return nil, err
		}
	}

	// Process the main store part.
	storeParts := strings.Fields(storePart)
	partsLength := len(storeParts)
	if partsLength != 3 && partsLength != 4 {
		return nil, fmt.Errorf("%w: store function requires 3 or 4 parameters, got %d", ErrInvalidArguments, partsLength)
	}

	params := &storeParams{
		storeName:    strings.TrimSpace(storeParts[0]),
		defaultValue: defaultValue,
		query:        query,
	}

	if partsLength == 4 {
		params.stack = strings.TrimSpace(storeParts[1])
		params.component = strings.TrimSpace(storeParts[2])
		params.key = strings.TrimSpace(storeParts[3])
	} else {
		params.stack = currentStack
		params.component = strings.TrimSpace(storeParts[1])
		params.key = strings.TrimSpace(storeParts[2])
	}

	return params, nil
}

// parseStoreGetParams parses the arguments for the store.get function.
func parseStoreGetParams(args string) (*storeGetParams, error) {
	// Split on pipe to separate store parameters and options.
	parts := strings.Split(args, "|")
	storePart := strings.TrimSpace(parts[0])

	// Extract default value and query from pipe parts.
	var defaultValue *string
	var query string
	if len(parts) > 1 {
		var err error
		defaultValue, query, err = extractPipeOptions(parts[1:])
		if err != nil {
			return nil, err
		}
	}

	// Process the main store part.
	storeParts := strings.Fields(storePart)
	if len(storeParts) != 2 {
		return nil, fmt.Errorf("%w: store.get function requires 2 parameters, got %d", ErrInvalidArguments, len(storeParts))
	}

	return &storeGetParams{
		storeName:    strings.TrimSpace(storeParts[0]),
		key:          strings.TrimSpace(storeParts[1]),
		defaultValue: defaultValue,
		query:        query,
	}, nil
}

// extractPipeOptions extracts default value and query from pipe-separated parts.
func extractPipeOptions(parts []string) (*string, string, error) {
	var defaultValue *string
	var query string

	for _, p := range parts {
		// Use SplitN to handle values containing spaces (e.g., query ".foo .bar").
		pipeParts := strings.SplitN(strings.TrimSpace(p), " ", 2)
		if len(pipeParts) != 2 {
			return nil, "", fmt.Errorf("%w: invalid pipe parameters", ErrInvalidArguments)
		}
		key := strings.Trim(pipeParts[0], `"'`)
		value := strings.Trim(pipeParts[1], `"'`)
		switch key {
		case "default":
			defaultValue = &value
		case "query":
			query = value
		default:
			return nil, "", fmt.Errorf("%w: invalid pipe identifier '%s'", ErrInvalidArguments, key)
		}
	}

	return defaultValue, query, nil
}
