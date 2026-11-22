package azure

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"reflect"

	errUtils "github.com/cloudposse/atmos/errors"
)

// msalCacheState is a decoded MSAL token cache file: top-level sections
// ("AccessToken", "RefreshToken", "IdToken", "Account", "AppMetadata") map entry
// keys to entries. Numbers are kept as json.Number so they round-trip verbatim.
type msalCacheState map[string]interface{}

// normalizeMSALCacheBytes strips a UTF-8 BOM and surrounding whitespace.
func normalizeMSALCacheBytes(data []byte) []byte {
	return bytes.TrimSpace(stripBOM(data))
}

// decodeMSALCacheState decodes normalized cache bytes. Empty input or a JSON null
// is an empty cache; anything that is not a JSON object returns an error.
func decodeMSALCacheState(data []byte) (msalCacheState, error) {
	state := msalCacheState{}
	if len(data) == 0 {
		return state, nil
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var decoded map[string]interface{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	if decoder.More() {
		return nil, fmt.Errorf("%w: unexpected data after the JSON object", errUtils.ErrAzureMSALCacheUnparseable)
	}
	for name, value := range decoded {
		state[name] = value
	}
	return state, nil
}

// readMSALCacheState reads and decodes the cache file at cachePath. A missing file
// is an empty cache. Content that is not a JSON object returns
// ErrAzureMSALCacheUnparseable.
func readMSALCacheState(cachePath string) (msalCacheState, error) {
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return msalCacheState{}, nil
		}
		return nil, fmt.Errorf("failed to read MSAL cache: %w", err)
	}
	state, err := decodeMSALCacheState(normalizeMSALCacheBytes(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %s (left unchanged): %w", errUtils.ErrAzureMSALCacheUnparseable, cachePath, err)
	}
	return state, nil
}

// mergeMSALCacheState applies this process's changes to the cache state currently
// on disk and returns the result (current is updated in place).
//
// base is the state this process loaded into MSAL (nil when it loaded nothing
// usable) and ours is MSAL's state now. An entry is written only when it differs
// from base, so entries another process (az, or another Atmos) added or refreshed
// since the load are kept, and an unchanged in-memory copy never overwrites a newer
// one on disk. Entries are never removed: Atmos does not remove individual cache
// entries (logout deletes a realm cache file as a whole), and skipping removals
// means a cache MSAL could not fully load is never emptied.
func mergeMSALCacheState(current, base, ours msalCacheState) msalCacheState {
	for name, oursValue := range ours {
		oursSection, isSection := oursValue.(map[string]interface{})
		if !isSection {
			if base == nil || !reflect.DeepEqual(base[name], oursValue) {
				current[name] = oursValue
			}
			continue
		}

		currentSection, ok := current[name].(map[string]interface{})
		if !ok {
			currentSection = make(map[string]interface{}, len(oursSection))
			current[name] = currentSection
		}
		baseSection, _ := base[name].(map[string]interface{})
		for key, entry := range oursSection {
			if baseEntry, loaded := baseSection[key]; base != nil && loaded && reflect.DeepEqual(baseEntry, entry) {
				continue // Unchanged since the load: keep whatever is on disk now.
			}
			currentSection[key] = entry
		}
	}
	return current
}
