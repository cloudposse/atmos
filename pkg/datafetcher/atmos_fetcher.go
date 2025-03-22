package datafetcher

import (
	"embed"
	"errors"
	"strings"
)

//go:embed schemas/*
var schemaFiles embed.FS

// atmosFetcher fetches data from the in-memory Atmos storage.
type atmosFetcher struct{}

// Sentinel error for quick checks.
var ErrAtmosSchemaNotFound = errors.New("atmos schema not found")

func (a atmosFetcher) FetchData(source string) ([]byte, error) {
	source = strings.TrimPrefix(source, "atmos://")
	data, err := schemaFiles.ReadFile(source + ".json")
	if err != nil {
		return nil, ErrAtmosSchemaNotFound
	}
	return data, nil
}
