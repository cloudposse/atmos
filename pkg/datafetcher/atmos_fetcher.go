package datafetcher

import (
	_ "embed"
	"errors"
)

//go:embed schema/atmos-manifest.json
var atmosSchema string

// AtmosFetcher fetches data from the in-memory Atmos storage.
type AtmosFetcher struct{}

// Sentinel error for quick checks.
var ErrAtmosSchemaNotFound = errors.New("atmos schema not found")

var atmosData = map[string][]byte{
	"atmos://schema": []byte(atmosSchema),
}

func (a AtmosFetcher) FetchData(source string) ([]byte, error) {
	if data, exists := atmosData[source]; exists {
		return data, nil
	}
	return nil, ErrAtmosSchemaNotFound
}
