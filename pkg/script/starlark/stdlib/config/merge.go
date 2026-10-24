package config

import (
	"encoding/json"
	"strings"

	starjson "go.starlark.net/lib/json"
	"go.starlark.net/starlark"

	"github.com/cloudposse/atmos/pkg/merge"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/convert"
	"github.com/cloudposse/atmos/pkg/script/starlark/internal/threadcontext"
)

// configMerge delegates to the same merge engine used by Atmos configuration.
// JSON numbers retain their lexical representation so even arbitrary-precision
// integers and integral floats survive the native-map boundary without coercion.
func configMerge(thread *starlark.Thread, b *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (starlark.Value, error) {
	if len(args) > 1 {
		return nil, convert.InvalidArgument("config.merge: list_merge_strategy must be a keyword argument")
	}
	var other starlark.Value
	strategy := merge.ListMergeStrategyReplace
	if err := starlark.UnpackArgs(b.Name(), args, kwargs, "other", &other, "list_merge_strategy?", &strategy); err != nil {
		return nil, err
	}
	switch strategy {
	case merge.ListMergeStrategyReplace, merge.ListMergeStrategyAppend, merge.ListMergeStrategyMerge:
	default:
		return nil, convert.InvalidArgument("config.merge: list_merge_strategy must be replace, append, or merge")
	}
	if _, ok := other.(starlark.IterableMapping); !ok {
		return nil, convert.InvalidArgument("config.merge: other must be a config record or dictionary, got %s", other.Type())
	}
	return mergeConfigValues(thread, b.Receiver(), other, strategy)
}

func mergeConfigValues(thread *starlark.Thread, base, other starlark.Value, strategy string) (starlark.Value, error) {
	ctx := threadcontext.Get(thread)
	checked, err := configRecords(ctx, other, 0)
	if err != nil {
		return nil, err
	}
	left, err := configMergeMap(thread, base)
	if err != nil {
		return nil, err
	}
	right, err := configMergeMap(thread, checked)
	if err != nil {
		return nil, err
	}
	result, err := merge.MergeWithOptions(nil, []map[string]any{left, right}, strategy == merge.ListMergeStrategyAppend, strategy == merge.ListMergeStrategyMerge)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return configMergeResult(thread, result)
}

func configMergeMap(thread *starlark.Thread, value starlark.Value) (map[string]any, error) {
	encoded, err := encodeConfigJSON(thread, value)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(encoded))
	decoder.UseNumber()
	var result map[string]any
	if err := decoder.Decode(&result); err != nil {
		return nil, err
	}
	return result, nil
}

func configMergeResult(thread *starlark.Thread, value map[string]any) (starlark.Value, error) {
	encoded, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	decoded, err := starlark.Call(thread, starjson.Module.Members["decode"], starlark.Tuple{starlark.String(encoded)}, nil)
	if err != nil {
		return nil, err
	}
	return configRecords(threadcontext.Get(thread), decoded, 0)
}
