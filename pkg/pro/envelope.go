package pro

import (
	"encoding/json"
	"io"

	"github.com/cloudposse/atmos/pkg/pro/dtos"
)

// DecodeEnvelope decodes an Atmos Pro API response body into the canonical envelope,
// reading the typed payload from `data`. Use this for every Pro response that returns a
// data payload so top-level fields are never mistaken for the payload — the bug class
// where a flat decode silently drops a `data`-nested result (see envelope_test.go).
func DecodeEnvelope[T any](r io.Reader) (*dtos.Envelope[T], error) {
	var env dtos.Envelope[T]
	if err := json.NewDecoder(r).Decode(&env); err != nil {
		return nil, err
	}
	return &env, nil
}
