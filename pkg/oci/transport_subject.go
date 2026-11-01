package oci

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
)

// readSubject streams a stored manifest or index and returns its subject, or nil when it has none.
// The blob was digest-verified when it was pushed, so the scan is bounded by the descriptor size
// and keeps constant memory: every other top-level field is skipped without being buffered. A
// large manifest without a subject is therefore never rejected.
func readSubject(ctx context.Context, store Fetcher, desc *ocispec.Descriptor) (*ocispec.Descriptor, error) {
	if desc.Size < 0 {
		return nil, errUtils.ErrOCIIntegrity
	}
	reader, err := store.Fetch(ctx, *desc)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	decoder := json.NewDecoder(io.LimitReader(reader, desc.Size))
	if err := expectDelim(decoder, '{'); err != nil {
		return nil, err
	}
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return nil, err
		}
		name, ok := key.(string)
		if !ok {
			return nil, errUtils.ErrOCIIntegrity
		}
		if !strings.EqualFold(name, "subject") {
			if err := skipValue(decoder); err != nil {
				return nil, err
			}
			continue
		}
		var subject *ocispec.Descriptor
		if err := decoder.Decode(&subject); err != nil {
			return nil, err
		}
		return subject, nil
	}
	return nil, nil
}

func expectDelim(decoder *json.Decoder, want json.Delim) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if delim, ok := token.(json.Delim); !ok || delim != want {
		return fmt.Errorf("%w: the manifest is not a JSON object", errUtils.ErrOCIIntegrity)
	}
	return nil
}

// skipValue consumes one JSON value, however deeply nested, without buffering it.
func skipValue(decoder *json.Decoder) error {
	depth := 0
	for {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if delim, ok := token.(json.Delim); ok {
			if delim == '{' || delim == '[' {
				depth++
			} else {
				depth--
			}
		}
		if depth == 0 {
			return nil
		}
	}
}
