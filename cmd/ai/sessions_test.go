package ai

import (
	"testing"

	"github.com/stretchr/testify/assert"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ai/session"
)

func TestParseDuration(t *testing.T) {
	tests := []struct {
		name          string
		durationStr   string
		expectedDays  int
		expectedError error
	}{
		{
			name:          "empty string returns default",
			durationStr:   "",
			expectedDays:  session.DefaultRetentionDays,
			expectedError: nil,
		},
		{
			name:          "valid days",
			durationStr:   "30d",
			expectedDays:  30,
			expectedError: nil,
		},
		{
			name:          "single day",
			durationStr:   "1d",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "valid weeks",
			durationStr:   "2w",
			expectedDays:  14, // 2 * 7
			expectedError: nil,
		},
		{
			name:          "single week",
			durationStr:   "1w",
			expectedDays:  7,
			expectedError: nil,
		},
		{
			name:          "valid months",
			durationStr:   "2m",
			expectedDays:  60, // 2 * 30
			expectedError: nil,
		},
		{
			name:          "single month",
			durationStr:   "1m",
			expectedDays:  30,
			expectedError: nil,
		},
		{
			name:          "valid hours - exact day",
			durationStr:   "24h",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "valid hours - rounds up",
			durationStr:   "25h",
			expectedDays:  2, // Rounds up from 1.04 days
			expectedError: nil,
		},
		{
			name:          "valid hours - multiple days",
			durationStr:   "48h",
			expectedDays:  2,
			expectedError: nil,
		},
		{
			name:          "hours less than a day rounds to 1",
			durationStr:   "12h",
			expectedDays:  1, // Rounds up
			expectedError: nil,
		},
		{
			name:          "single hour rounds to 1 day",
			durationStr:   "1h",
			expectedDays:  1,
			expectedError: nil,
		},
		{
			name:          "large value",
			durationStr:   "365d",
			expectedDays:  365,
			expectedError: nil,
		},
		{
			name:          "invalid format - no number",
			durationStr:   "d",
			expectedDays:  0,
			expectedError: errUtils.ErrAIInvalidDurationFormat,
		},
		{
			name:          "invalid format - no unit",
			durationStr:   "30",
			expectedDays:  0,
			expectedError: errUtils.ErrAIInvalidDurationFormat,
		},
		{
			name:          "invalid format - only text",
			durationStr:   "invalid",
			expectedDays:  0,
			expectedError: errUtils.ErrAIInvalidDurationFormat,
		},
		{
			name:          "invalid unit",
			durationStr:   "30x",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
		},
		{
			name:          "invalid unit - years",
			durationStr:   "1y",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
		},
		{
			name:          "invalid unit - seconds",
			durationStr:   "3600s",
			expectedDays:  0,
			expectedError: errUtils.ErrAIUnsupportedDurationUnit,
		},
		{
			name:          "negative value",
			durationStr:   "-30d",
			expectedDays:  -30,
			expectedError: nil, // parseDuration doesn't validate negative values
		},
		{
			name:          "zero value",
			durationStr:   "0d",
			expectedDays:  0,
			expectedError: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := parseDuration(tt.durationStr)

			if tt.expectedError != nil {
				assert.Error(t, err)
				assert.ErrorIs(t, err, tt.expectedError)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedDays, result)
			}
		})
	}
}
