package azure

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	// Decoder.More is an array/object iteration check, not an end-of-input check
	// (it reports false before a stray ']' or '}'), so require EOF on a second decode.
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
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
// The base argument is the state this process loaded into MSAL (nil when it loaded nothing
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
			if base == nil || !sameMSALValue(base[name], oursValue) {
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
			if baseEntry, loaded := baseSection[key]; base != nil && loaded && sameMSALValue(baseEntry, entry) {
				continue // Unchanged since the load: keep whatever is on disk now.
			}
			currentSection[key] = entry
		}
	}
	return current
}

// sameMSALValue reports whether two decoded cache values are equal once zero values
// are ignored. MSAL's serializer omits empty fields (omitempty), so an entry loaded
// from disk with an explicit empty field (for example "home_account_id": "") and
// the same entry MSAL marshals back differ only in that field and are unchanged.
func sameMSALValue(a, b interface{}) bool {
	return reflect.DeepEqual(dropMSALZeroValues(a), dropMSALZeroValues(b))
}

// dropMSALZeroValues returns value with map keys holding zero values removed,
// recursively. The input is not modified.
func dropMSALZeroValues(value interface{}) interface{} {
	m, ok := value.(map[string]interface{})
	if !ok {
		return value
	}
	out := make(map[string]interface{}, len(m))
	for key, v := range m {
		if isMSALZeroValue(v) {
			continue
		}
		out[key] = dropMSALZeroValues(v)
	}
	return out
}

// isMSALZeroValue reports whether a decoded JSON value is one that omitempty drops.
func isMSALZeroValue(value interface{}) bool {
	switch v := value.(type) {
	case nil:
		return true
	case string:
		return v == ""
	case bool:
		return !v
	case json.Number:
		return v.String() == "0"
	case []interface{}:
		return len(v) == 0
	case map[string]interface{}:
		return len(v) == 0
	default:
		return false
	}
}
