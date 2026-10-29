package component

import (
	"errors"
	"testing"

	pluginv1 "github.com/cloudposse/atmos/sdk/proto/atmos/plugin/v1"
)

func FuzzDecodeValue(f *testing.F) {
	f.Add([]byte(`"string"`), []byte{0xa2, 'o', 'k'})
	f.Add([]byte(`"number"`), []byte{42})
	f.Add([]byte(`"bool"`), []byte{0xc0})
	f.Add([]byte(`[]`), []byte{})
	f.Fuzz(func(t *testing.T, typeJSON, payload []byte) {
		_, err := Decode(&pluginv1.ComponentValue{TypeJson: typeJSON, Msgpack: payload})
		if err != nil && (!errors.Is(err, ErrInvalidValue) || err.Error() != ErrInvalidValue.Error()) {
			t.Fatal("decoder exposed payload details")
		}
	})
}
