package duration

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestInUnits(t *testing.T) {
	assert.Equal(t, Units{}, InUnits(-time.Second))
	assert.Equal(t, Units{}, InUnits(time.Second-time.Nanosecond))
	assert.Equal(t, Units{Seconds: 1}, InUnits(time.Second))
	age := 365 * 24 * time.Hour
	assert.Equal(t, Units{
		Seconds: 31536000, Minutes: 525600, Hours: 8760,
		Days: 365, Weeks: 52, Months: 12, Years: 1,
	}, InUnits(age))
	assert.EqualValues(t, 3, InUnits(120*24*time.Hour-time.Nanosecond).Months)
	assert.EqualValues(t, 4, InUnits(120*24*time.Hour).Months)
}
