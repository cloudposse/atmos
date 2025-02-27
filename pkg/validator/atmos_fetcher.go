package validator

import (
	_ "embed"
	"errors"
)

//go:embed schema/atmos-manifest.json
var atmosSchema string

// AtmosFetcher fetches data from the in-memory Atmos storage.
type AtmosFetcher struct {
	Key string
}

// Sentinel error for quick checks.
var ErrAtmosSchemaNotFound = errors.New("atmos schema not found")

var atmosData = map[string][]byte{
	"atmos://schema": []byte(atmosSchema),
}

func (a *AtmosFetcher) Fetch() ([]byte, error) {
	if data, exists := atmosData[a.Key]; exists {
		return data, nil
	}
	return nil, ErrAtmosSchemaNotFound
}
