package helm

import (
	"fmt"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/ui"
)

const defaultHelmProgressInterval = 30 * time.Second

// helmProgressOutput keeps progress rendering testable while production output
// still flows through the globally masked Atmos UI stream.
type helmProgressOutput struct {
	info    func(string)
	success func(string)
	failure func(string)
}

// helmOperationProgress reports durable, line-oriented lifecycle events for a
// single Helm release operation. The line format is safe for TTY and CI output.
type helmOperationProgress struct {
	mu sync.RWMutex

	component string
	stack     string
	release   string
	namespace string
	dryRun    bool

	requestedOperation string
	effectiveOperation string
	lifecycle          releaseLifecycleResolution

	startedAt time.Time
	now       func() time.Time
	interval  time.Duration
	output    helmProgressOutput

	done       chan struct{}
	doneOnce   sync.Once
	finishOnce sync.Once
	wg         sync.WaitGroup
}

func newHelmOperationProgress(
	info *schema.ConfigAndStacksInfo,
	spec *chartSpec,
	requestedOperation string,
	dryRun bool,
) *helmOperationProgress {
	defer perf.Track(nil, "helm.newHelmOperationProgress")()

	progress := &helmOperationProgress{
		requestedOperation: requestedOperation,
		dryRun:             dryRun,
		now:                time.Now,
		interval:           defaultHelmProgressInterval,
		output: helmProgressOutput{
			info:    ui.Info,
			success: ui.Success,
			failure: ui.Error,
		},
		done: make(chan struct{}),
	}
	if info != nil {
		progress.component = info.ComponentFromArg
		progress.stack = info.Stack
	}
	if spec != nil {
		progress.release = spec.ReleaseName
		progress.namespace = spec.Namespace
	}
	return progress
}

func (p *helmOperationProgress) Start() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.startedAt = p.now()
	p.mu.Unlock()
	p.output.info(p.preparingMessage())

	if p.interval <= 0 {
		return
	}
	p.wg.Add(1)
	go p.reportHeartbeats()
}

func (p *helmOperationProgress) Resolved(operation string, lifecycle releaseLifecycleResolution) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.effectiveOperation = operation
	p.lifecycle = lifecycle
	p.mu.Unlock()
	p.output.info(p.runningMessage())
}

func (p *helmOperationProgress) Finish(operationErr error) {
	if p == nil {
		return
	}
	p.finishOnce.Do(func() {
		p.doneOnce.Do(func() { close(p.done) })
		p.wg.Wait()
		if operationErr != nil {
			p.output.failure(p.finishedMessage("failed"))
			return
		}
		p.output.success(p.finishedMessage("succeeded"))
	})
}

func (p *helmOperationProgress) reportHeartbeats() {
	defer p.wg.Done()
	ticker := time.NewTicker(p.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			p.output.info(p.heartbeatMessage())
		case <-p.done:
			return
		}
	}
}

func (p *helmOperationProgress) operation() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.effectiveOperation != "" {
		return p.effectiveOperation
	}
	return p.requestedOperation
}

func (p *helmOperationProgress) preparingMessage() string {
	return fmt.Sprintf("Preparing Helm %s: %s%s", p.requestedOperation, p.identity(), p.dryRunSuffix())
}

func (p *helmOperationProgress) runningMessage() string {
	p.mu.RLock()
	operation := p.effectiveOperation
	policy := p.lifecycle.Policy
	p.mu.RUnlock()

	if operation == releaseOperationDelete {
		return fmt.Sprintf(
			"Helm %s started: %s (wait=%s, timeout=%s)%s",
			operation,
			p.identity(),
			policy.WaitStrategy,
			policy.Timeout,
			p.dryRunSuffix(),
		)
	}
	return fmt.Sprintf(
		"Helm %s started: %s (wait=%s, jobs=%t, timeout=%s, on_failure=%s)%s",
		operation,
		p.identity(),
		policy.WaitStrategy,
		policy.WaitForJobs,
		policy.Timeout,
		policy.OnFailure,
		p.dryRunSuffix(),
	)
}

func (p *helmOperationProgress) heartbeatMessage() string {
	return fmt.Sprintf("Helm %s still in progress: %s (elapsed=%s)%s", p.operation(), p.identity(), p.elapsed(), p.dryRunSuffix())
}

func (p *helmOperationProgress) finishedMessage(status string) string {
	return fmt.Sprintf("Helm %s %s: %s (duration=%s)%s", p.operation(), status, p.identity(), p.elapsed(), p.dryRunSuffix())
}

func (p *helmOperationProgress) identity() string {
	return fmt.Sprintf(
		"component=%s, stack=%s, release=%s, namespace=%s",
		valueOrDefault(p.component, "unknown"),
		valueOrDefault(p.stack, "unknown"),
		valueOrDefault(p.release, "unknown"),
		valueOrDefault(p.namespace, "default"),
	)
}

func (p *helmOperationProgress) elapsed() time.Duration {
	p.mu.RLock()
	startedAt := p.startedAt
	p.mu.RUnlock()
	if startedAt.IsZero() {
		return 0
	}
	elapsed := p.now().Sub(startedAt).Round(time.Second)
	if elapsed < 0 {
		return 0
	}
	return elapsed
}

func (p *helmOperationProgress) dryRunSuffix() string {
	if p.dryRun {
		return " (dry run)"
	}
	return ""
}

func valueOrDefault(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}

func reportReleaseProgress(spec *chartSpec, operation string, lifecycle releaseLifecycleResolution) {
	if spec == nil || spec.Progress == nil {
		return
	}
	spec.Progress.Resolved(operation, lifecycle)
}
