// Package execbatch evaluates ordinary !exec values with bounded concurrency.
package execbatch

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/policy/guard"
	"github.com/cloudposse/atmos/pkg/scheduler"
)

// Value marks an already evaluated expression so its output is not interpreted as another tag.
type Value struct{ Data any }

// Executor runs one command at its document path. Implementations must support concurrent calls.
type Executor func(context.Context, string, string) (any, error)

type job struct {
	command, path, raw string
	value              *Value
}

// Evaluate copies a document and evaluates its !exec leaves. Only command execution is concurrent;
// discovery and result placement are serial and the caller's maps and slices remain untouched.
func Evaluate(ctx context.Context, input map[string]any, limit int, execute Executor) (map[string]any, error) {
	defer perf.Track(nil, "execbatch.Evaluate")()
	if limit < 1 {
		return nil, fmt.Errorf("%w: settings.functions.exec.max_concurrency must be at least 1", errUtils.ErrInvalidConfig)
	}
	var jobs []job
	output := collect(input, "$", &jobs).(map[string]any)
	if len(jobs) == 0 {
		return output, ctx.Err()
	}
	if execute == nil {
		var err error
		execute, err = ShellExecutor()
		if err != nil {
			return nil, err
		}
	}
	graph, err := jobGraph(ctx, jobs)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	dispatcher := scheduler.DispatcherFunc(func(ctx context.Context, node *dependency.Node) (scheduler.Result, error) {
		if err := ctx.Err(); err != nil {
			return scheduler.Result{}, err
		}
		i, _ := strconv.Atoi(node.ID)
		job := jobs[i]
		value, err := execute(ctx, job.command, job.path)
		if err != nil {
			cancel()
			return scheduler.Result{}, fmt.Errorf("configuration %s: %w", job.path, err)
		}
		return scheduler.Result{Value: value}, nil
	})
	result := scheduler.New(graph, dispatcher, scheduler.WithMaxConcurrency(limit), scheduler.WithFailFast(true)).Run(runCtx)
	if result.Err != nil {
		return nil, result.Err
	}
	for index := range result.Results {
		item := &result.Results[index]
		i, _ := strconv.Atoi(item.NodeID)
		jobs[i].value.Data = item.Value
	}
	return output, nil
}

func collect(raw any, path string, jobs *[]job) any {
	switch value := raw.(type) {
	case map[string]any:
		result := make(map[string]any, len(value))
		for _, key := range slices.Sorted(maps.Keys(value)) {
			result[key] = collect(value[key], path+"["+strconv.Quote(key)+"]", jobs)
		}
		return result
	case []any:
		result := make([]any, len(value))
		for i, item := range value {
			result[i] = collect(item, fmt.Sprintf("%s[%d]", path, i), jobs)
		}
		return result
	case string:
		if value == "!exec" || strings.HasPrefix(value, "!exec ") || strings.HasPrefix(value, "!exec\n") || strings.HasPrefix(value, "!exec\t") {
			result := &Value{}
			*jobs = append(*jobs, job{command: strings.TrimSpace(strings.TrimPrefix(value, "!exec")), path: path, raw: value, value: result})
			return result
		}
	}
	return raw
}

func jobGraph(ctx context.Context, jobs []job) (*dependency.Graph, error) {
	graph := dependency.NewGraph()
	for i, job := range jobs {
		if err := guard.FunctionValue(ctx, "exec", job.raw); err != nil {
			return nil, fmt.Errorf("configuration %s: %w", job.path, err)
		}
		if strings.TrimSpace(job.command) == "" {
			return nil, fmt.Errorf("%w: configuration %s: !exec requires a command", errUtils.ErrInvalidConfig, job.path)
		}
		if err := graph.AddNode(&dependency.Node{ID: fmt.Sprintf("%09d", i)}); err != nil {
			return nil, err
		}
	}

	return graph, nil
}
