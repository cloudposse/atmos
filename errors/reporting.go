package errors

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ErrorReporter routes errors without coupling the errors package to Pro or I/O.
type ErrorReporter interface {
	Capture(error, map[string]string)
	Flush(context.Context)
}

var reporting struct {
	sync.RWMutex
	reporter ErrorReporter
}

// SetErrorReporter installs the invocation's reporter and returns its predecessor.
func SetErrorReporter(reporter ErrorReporter) ErrorReporter {
	reporting.Lock()
	defer reporting.Unlock()
	previous := reporting.reporter
	reporting.reporter = reporter
	return previous
}

func currentReporter() ErrorReporter {
	reporting.RLock()
	defer reporting.RUnlock()
	return reporting.reporter
}

// ReportingError preserves the original error and a detached component snapshot.
// Its state belongs to this occurrence, not to its message or fingerprint.
type ReportingError struct {
	cause       error
	info        schema.ConfigAndStacksInfo
	executionID string
	captured    *atomic.Bool
}

func (e *ReportingError) Error() string { return e.cause.Error() }
func (e *ReportingError) Unwrap() error { return e.cause }

// Claim ensures that printing and subsequently returning the same failure reports once.
func (e *ReportingError) Claim() bool { return e.captured.CompareAndSwap(false, true) }

// ReportingContext returns a fresh copy so consumers cannot modify the snapshot.
func (e *ReportingError) ReportingContext() (schema.ConfigAndStacksInfo, string) {
	return snapshotReportingInfo(&e.info), e.executionID
}

// WithReportingContext attaches resolved metadata while preserving errors.Is/As and exit codes.
func WithReportingContext(err error, info *schema.ConfigAndStacksInfo, executionID string) error {
	if err == nil {
		return nil
	}
	claimed := &atomic.Bool{}
	var existing *ReportingError
	if errors.As(err, &existing) {
		claimed = existing.captured
	}
	return &ReportingError{cause: err, info: snapshotReportingInfo(info), executionID: executionID, captured: claimed}
}

func snapshotReportingInfo(info *schema.ConfigAndStacksInfo) schema.ConfigAndStacksInfo {
	if info == nil {
		return schema.ConfigAndStacksInfo{}
	}
	return schema.ConfigAndStacksInfo{
		Component: info.Component, ComponentFromArg: info.ComponentFromArg,
		ComponentType: info.ComponentType, Stack: info.Stack, StackFromArg: info.StackFromArg,
		SubCommand:               info.SubCommand,
		ComponentMetadataSection: cloneReportingMap(info.ComponentMetadataSection),
		ComponentSettingsSection: cloneReportingMap(info.ComponentSettingsSection),
	}
}

func cloneReportingMap(source map[string]any) map[string]any {
	if source == nil {
		return nil
	}
	result := make(map[string]any, len(source))
	for key, value := range source {
		result[key] = cloneReportingValue(value)
	}
	return result
}

func cloneReportingValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		return cloneReportingMap(v)
	case map[string]string:
		result := make(map[string]string, len(v))
		for key, item := range v {
			result[key] = item
		}
		return result
	case []any:
		result := make([]any, len(v))
		for i, item := range v {
			result[i] = cloneReportingValue(item)
		}
		return result
	case []string:
		return append([]string(nil), v...)
	default:
		return value
	}
}
