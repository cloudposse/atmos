package report_test

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"

	"github.com/cloudposse/atmos/pkg/artifact/invoke"
	"github.com/cloudposse/atmos/pkg/artifact/report"
)

func TestDiscardIgnoresEveryEntry(t *testing.T) {
	var collector report.Collector = report.Discard{}
	assert.NotPanics(t, func() {
		collector.Record(&report.Entry{Step: "publish", OperationID: "op-1", Attempt: 1})
		collector.Record(&report.Entry{Err: errors.New("failed"), Outcome: &invoke.Outcome{Result: &invoke.Result{}}})
	})
}

func TestCollectorMockRecordsEntries(t *testing.T) {
	mock := report.NewMockCollector(gomock.NewController(t))
	entry := &report.Entry{Step: "publish", OperationID: "op-1", Attempt: 2}
	mock.EXPECT().Record(entry)
	var collector report.Collector = mock
	collector.Record(entry)
}
