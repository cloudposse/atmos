package step

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
)

func TestTestFailureErrorSummary(t *testing.T) {
	leaf := errors.New("leaf failure")
	tests := []struct {
		name string
		err  *TestFailureError
		want string
	}{
		{"plural", &TestFailureError{Err: leaf, Failed: 2, Total: 7}, "2 of 7 tests failed"},
		{"singular", &TestFailureError{Err: leaf, Failed: 1, Total: 1}, "1 of 1 test failed"},
		{"one failure among many", &TestFailureError{Err: leaf, Failed: 1, Total: 5}, "1 of 5 tests failed"},
		{"preparation error keeps message", &TestFailureError{Err: leaf}, "leaf failure"},
		{"no counts and no error", &TestFailureError{}, errUtils.ErrTestsFailed.Error()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.err.Error())
		})
	}
}

func TestTestFailureErrorMatching(t *testing.T) {
	leafA := errors.New("assertion A")
	leafB := errors.New("assertion B")
	err := fmt.Errorf("wrapped: %w", &TestFailureError{Err: errors.Join(leafA, leafB), Failed: 2, Total: 3})

	assert.ErrorIs(t, err, errUtils.ErrTestsFailed)
	assert.ErrorIs(t, err, leafA)
	assert.ErrorIs(t, err, leafB)
	assert.NotErrorIs(t, err, errors.New("unrelated"))
	assert.NotContains(t, err.Error(), "assertion A")

	var displayed *TestFailureError
	assert.ErrorAs(t, err, &displayed)
	assert.Equal(t, 2, displayed.Failed)
	assert.Equal(t, 3, displayed.Total)
}
