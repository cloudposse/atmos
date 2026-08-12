package helm

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v4/pkg/kube"

	"github.com/cloudposse/atmos/pkg/schema"
)

type progressCapture struct {
	mu       sync.Mutex
	info     []string
	success  []string
	failures []string
}

func (c *progressCapture) output() helmProgressOutput {
	return helmProgressOutput{
		info: func(message string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.info = append(c.info, message)
		},
		success: func(message string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.success = append(c.success, message)
		},
		failure: func(message string) {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.failures = append(c.failures, message)
		},
	}
}

func (c *progressCapture) infoMessages() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.info...)
}

func TestHelmOperationProgressReportsResolvedLifecycleAndSuccess(t *testing.T) {
	startedAt := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	now := startedAt
	capture := &progressCapture{}
	progress := newHelmOperationProgress(
		&schema.ConfigAndStacksInfo{ComponentFromArg: "service", Stack: "dev"},
		&chartSpec{ReleaseName: "service", Namespace: "apps"},
		string(OperationApply),
		false,
	)
	progress.now = func() time.Time { return now }
	progress.interval = 0
	progress.output = capture.output()

	progress.Start()
	progress.Resolved(releaseOperationUpgrade, releaseLifecycleResolution{Policy: effectiveReleasePolicy{
		Operation:    releaseOperationUpgrade,
		WaitStrategy: kube.StatusWatcherStrategy,
		WaitForJobs:  true,
		Timeout:      10 * time.Minute,
		OnFailure:    failurePolicyRollback,
	}})
	now = startedAt.Add(42 * time.Second)
	progress.Finish(nil)

	info := capture.infoMessages()
	require.Len(t, info, 2)
	assert.Equal(t, "Preparing Helm apply: component=service, stack=dev, release=service, namespace=apps", info[0])
	assert.Equal(t, "Helm upgrade started: component=service, stack=dev, release=service, namespace=apps (wait=watcher, jobs=true, timeout=10m0s, on_failure=rollback)", info[1])
	require.Len(t, capture.success, 1)
	assert.Equal(t, "Helm upgrade succeeded: component=service, stack=dev, release=service, namespace=apps (duration=42s)", capture.success[0])
	assert.Empty(t, capture.failures)
}

func TestHelmOperationProgressReportsDeleteDryRunFailure(t *testing.T) {
	startedAt := time.Date(2026, time.August, 12, 10, 0, 0, 0, time.UTC)
	now := startedAt
	capture := &progressCapture{}
	progress := newHelmOperationProgress(
		&schema.ConfigAndStacksInfo{ComponentFromArg: "service", Stack: "dev"},
		&chartSpec{ReleaseName: "service"},
		string(OperationDelete),
		true,
	)
	progress.now = func() time.Time { return now }
	progress.interval = 0
	progress.output = capture.output()

	progress.Start()
	progress.Resolved(releaseOperationDelete, releaseLifecycleResolution{Policy: effectiveReleasePolicy{
		Operation:    releaseOperationDelete,
		WaitStrategy: kube.LegacyStrategy,
		Timeout:      5 * time.Minute,
	}})
	now = startedAt.Add(5 * time.Minute)
	progress.Finish(errors.New("operation failed"))

	info := capture.infoMessages()
	require.Len(t, info, 2)
	assert.Contains(t, info[0], "namespace=default (dry run)")
	assert.Equal(t, "Helm delete started: component=service, stack=dev, release=service, namespace=default (wait=legacy, timeout=5m0s) (dry run)", info[1])
	assert.Empty(t, capture.success)
	require.Len(t, capture.failures, 1)
	assert.Equal(t, "Helm delete failed: component=service, stack=dev, release=service, namespace=default (duration=5m0s) (dry run)", capture.failures[0])
}

func TestHelmOperationProgressEmitsHeartbeat(t *testing.T) {
	capture := &progressCapture{}
	progress := newHelmOperationProgress(
		&schema.ConfigAndStacksInfo{ComponentFromArg: "service", Stack: "dev"},
		&chartSpec{ReleaseName: "service", Namespace: "apps"},
		string(OperationApply),
		false,
	)
	progress.interval = time.Millisecond
	progress.output = capture.output()

	progress.Start()
	progress.Resolved(releaseOperationInstall, releaseLifecycleResolution{Policy: effectiveReleasePolicy{
		Operation:    releaseOperationInstall,
		WaitStrategy: kube.StatusWatcherStrategy,
		Timeout:      time.Minute,
	}})
	require.Eventually(t, func() bool {
		for _, message := range capture.infoMessages() {
			if strings.Contains(message, "Helm install still in progress") {
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond)
	progress.Finish(nil)

	require.Len(t, capture.success, 1)
}

func TestReportReleaseProgressHandlesMissingReporter(t *testing.T) {
	assert.NotPanics(t, func() {
		reportReleaseProgress(nil, releaseOperationInstall, releaseLifecycleResolution{})
		reportReleaseProgress(&chartSpec{}, releaseOperationInstall, releaseLifecycleResolution{})
	})
}
