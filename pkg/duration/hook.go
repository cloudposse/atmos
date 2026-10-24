package duration

import (
	"fmt"
	"reflect"
	"time"

	"github.com/go-viper/mapstructure/v2"
)

var durationType = reflect.TypeOf(time.Duration(0))

// DecodeHook returns a mapstructure decode hook that converts configuration values into
// time.Duration using ParseAny: strings in any accepted form, and numbers as seconds.
//
// It replaces mapstructure.StringToTimeDurationHookFunc, which only accepted Go duration
// strings and let a bare YAML integer decode as nanoseconds. Negative values are rejected;
// zero is accepted so callers can decide whether zero is meaningful.
func DecodeHook() mapstructure.DecodeHookFunc {
	return func(from reflect.Type, to reflect.Type, data any) (any, error) {
		if to != durationType {
			return data, nil
		}
		if from == durationType {
			return data, nil
		}
		d, err := ParseAny(data)
		if err != nil {
			return nil, err
		}
		if d < 0 {
			return nil, fmt.Errorf("%w: %v", ErrNegative, data)
		}
		return d, nil
	}
}
