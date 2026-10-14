package qualitygate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/cloudposse/atmos/magefiles/bench"
)

const (
	// Run budget for one reporter run; the workflow job allows 25 minutes.
	qualityRunBudget = 20 * time.Minute
	// Per-PR timeout so a slow or hung PR cannot spend the shared budget and
	// turn every later PR into a deadline error.
	qualityPRTimeout = 3 * time.Minute
)

// reconcileAll reconciles each PR under its own deadline and keeps going after
// a failure. Every failure is printed as a workflow warning annotation, but the
// run fails only when every attempted PR failed: that points at something
// systemic (credentials, an outage), while isolated failures are retried by the
// next scheduled run and stay visible in the annotations. PRs left unvisited
// when the run budget ends are not failures; the next run selects them again.
func (q *qualityClient) reconcileAll(numbers []int) error {
	timeout := q.prTimeout
	if timeout <= 0 {
		timeout = qualityPRTimeout
	}
	var failures []error
	attempted := 0
	for i, number := range numbers {
		if q.ctx.Err() != nil {
			fmt.Printf("Quality report: run budget exhausted; %d of %d PRs not visited\n", len(numbers)-i, len(numbers))
			break
		}
		attempted++
		prCtx, cancel := context.WithTimeout(q.ctx, timeout)
		scoped := *q
		scoped.ctx = prCtx
		err := scoped.reconcile(number)
		cancel()
		if err != nil {
			err = fmt.Errorf("PR #%d: %w", number, err)
			failures = append(failures, err)
			fmt.Printf("::warning title=Quality gate reconcile failed::%s\n", qualityAnnotation(err.Error()))
			fmt.Printf("Quality report: %v\n", err)
		}
	}
	switch {
	case len(numbers) > 0 && attempted == 0:
		return bench.Error("run budget exhausted before any PR was reconciled")
	case attempted > 0 && len(failures) == attempted:
		return errors.Join(failures...)
	}
	if len(failures) > 0 {
		fmt.Printf("Quality report: %d of %d PRs failed; the next run retries them\n", len(failures), attempted)
	}
	return nil
}

// qualityAnnotation escapes a workflow command message.
func qualityAnnotation(message string) string {
	return strings.NewReplacer("%", "%25", "\r", "%0D", "\n", "%0A").Replace(message)
}
