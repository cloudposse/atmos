package helm

import (
	"sort"
	"sync"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/component"
	"github.com/cloudposse/atmos/pkg/dependency"
	"github.com/cloudposse/atmos/pkg/hooks"
	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/schema"
)

const helmBulkCICollectorFlag = "_helm_bulk_ci_collector"

// helmBulkCICollector accumulates one result per graph node so bulk Helm
// commands can write a single CI summary after execution completes.
type helmBulkCICollector struct {
	mu      sync.Mutex
	command string
	results map[string]*schema.HelmCIResult
}

func newHelmBulkCICollector(command string) *helmBulkCICollector {
	return &helmBulkCICollector{
		command: command,
		results: make(map[string]*schema.HelmCIResult),
	}
}

// bulkCollectingProvider wraps the native Helm provider and records failures
// that happen before an operation produces its structured summary.
type bulkCollectingProvider struct {
	component.ComponentProvider
	collector *helmBulkCICollector
}

func (p *bulkCollectingProvider) Execute(ctx *component.ExecutionContext) error {
	startedAt := time.Now()
	err := p.ComponentProvider.Execute(ctx)
	p.collector.finish(ctx, startedAt, time.Now(), err)
	return err
}

func (p *bulkCollectingProvider) OnGraphNodeSkipped(node *dependency.Node) {
	if p == nil || p.collector == nil || node == nil {
		return
	}
	p.collector.markSkipped(node.Stack, node.Component)
}

func helmBulkCollector(ctx *component.ExecutionContext) *helmBulkCICollector {
	if ctx == nil {
		return nil
	}
	collector, _ := ctx.Flags[helmBulkCICollectorFlag].(*helmBulkCICollector)
	return collector
}

func (c *helmBulkCICollector) setSummary(info *schema.ConfigAndStacksInfo, summary map[string]any, operationErr error) {
	if c == nil || info == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	result := c.ensure(info.Stack, info.ComponentFromArg)
	result.Processed = true
	result.Summary = cloneHelmSummary(summary)
	applyHelmResultError(result, operationErr)
}

func (c *helmBulkCICollector) finish(ctx *component.ExecutionContext, startedAt, finishedAt time.Time, execErr error) {
	if c == nil || ctx == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	componentName := ctx.ConfigAndStacksInfo.ComponentFromArg
	if componentName == "" {
		componentName = ctx.Component
	}
	result := c.ensure(ctx.Stack, componentName)
	result.StartedAt = startedAt
	result.FinishedAt = finishedAt
	result.DurationMS = finishedAt.Sub(startedAt).Milliseconds()
	applyHelmResultError(result, execErr)
}

func (c *helmBulkCICollector) markSkipped(stack, componentName string) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	result := c.ensure(stack, componentName)
	result.Status = "skipped"
}

func (c *helmBulkCICollector) ensure(stack, componentName string) *schema.HelmCIResult {
	nodeID := component.GraphNodeID(componentName, stack)
	if result, ok := c.results[nodeID]; ok {
		return result
	}
	result := &schema.HelmCIResult{
		NodeID:    nodeID,
		Stack:     stack,
		Component: componentName,
		Status:    "succeeded",
	}
	c.results[nodeID] = result
	return result
}

func (c *helmBulkCICollector) resultSet() schema.HelmCIResultSet {
	if c == nil {
		return schema.HelmCIResultSet{}
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	results := make([]schema.HelmCIResult, 0, len(c.results))
	for _, result := range c.results {
		copyResult := *result
		copyResult.Summary = cloneHelmSummary(result.Summary)
		results = append(results, copyResult)
	}
	sort.SliceStable(results, func(i, j int) bool {
		if results[i].Stack != results[j].Stack {
			return results[i].Stack < results[j].Stack
		}
		if results[i].Component != results[j].Component {
			return results[i].Component < results[j].Component
		}
		return results[i].NodeID < results[j].NodeID
	})
	return schema.HelmCIResultSet{Command: c.command, Results: results}
}

func applyHelmResultError(result *schema.HelmCIResult, err error) {
	if result == nil || err == nil {
		return
	}
	result.Status = "failed"
	result.ExitCode = errUtils.GetExitCode(err)
	result.Error = err.Error()
}

func cloneHelmSummary(summary map[string]any) map[string]any {
	if summary == nil {
		return nil
	}
	cloned := make(map[string]any, len(summary))
	for key, value := range summary {
		cloned[key] = cloneHelmSummaryValue(value)
	}
	return cloned
}

func cloneHelmSummaryValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		return cloneHelmSummary(typed)
	case []any:
		cloned := make([]any, len(typed))
		for i := range typed {
			cloned[i] = cloneHelmSummaryValue(typed[i])
		}
		return cloned
	case []string:
		return append([]string(nil), typed...)
	default:
		return value
	}
}

func supportsHelmAggregateCI(command string) bool {
	switch command {
	case "plan", "diff", "apply", "deploy":
		return true
	default:
		return false
	}
}

func runHelmAggregateCIHook(
	ctx *component.ExecutionContext,
	atmosConfig *schema.AtmosConfiguration,
	info *schema.ConfigAndStacksInfo,
	resultSet schema.HelmCIResultSet,
	commandErr error,
) {
	event := hooks.AfterHelmPlanAggregate
	if resultSet.Command == "apply" || resultSet.Command == "deploy" {
		event = hooks.AfterHelmApplyAggregate
	}
	if err := runCIHooks(&hooks.RunCIHooksOptions{
		Event:        event,
		AtmosConfig:  atmosConfig,
		Info:         info,
		ForceCIMode:  helmCIModeEnabled(ctx.Flags),
		CommandError: commandErr,
		ExitCode:     errUtils.GetExitCode(commandErr),
		Aggregate:    resultSet,
	}); err != nil {
		log.Warn("Helm CI aggregate hook failed", "command", resultSet.Command, "error", err)
	}
}
