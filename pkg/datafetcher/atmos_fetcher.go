package datafetcher

import (
	"embed"
	"errors"
	"strings"
)

//go:embed schema/*
var schemaFiles embed.FS

// atmosFetcher fetches data from the in-memory Atmos storage.
type atmosFetcher struct{}

// Sentinel error for quick checks.
var ErrAtmosSchemaNotFound = errors.New("atmos schema not found")

// schemaAliases maps legacy schema paths to their current locations so
// historical `atmos://` URIs keep resolving. `schema/config/global/1.0` used to
// be a stale copy of the stack-manifest schema; it now serves the generated
// atmos.yaml configuration schema (see pkg/config/schema).
var schemaAliases = map[string]string{
	"schema/config/global/1.0": "schema/atmos/config/1.0",
}

func (a atmosFetcher) FetchData(source string) ([]byte, error) {
	source = strings.TrimPrefix(source, "atmos://")
	if target, ok := schemaAliases[source]; ok {
		source = target
	}
	data, err := schemaFiles.ReadFile(source + ".json")
	if err != nil {
		return nil, ErrAtmosSchemaNotFound
	}
	return data, nil
}
