package oci

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
)

// maxManifestKeyBytes bounds how much of an object key the subject scan retains. A longer key can
// never be the "subject" field, so the remainder is skipped without being kept.
const maxManifestKeyBytes = 64

// readSubject streams a stored manifest or index and returns its subject, or nil when it has none.
// The blob was digest-verified when it was pushed, so the scan is bounded by the descriptor size
// and keeps constant memory: every other top-level field, including arbitrarily long strings, is
// skipped byte by byte without being buffered. A large manifest without a subject is therefore
// never rejected.
func readSubject(ctx context.Context, store Fetcher, desc *ocispec.Descriptor) (*ocispec.Descriptor, error) {
	if desc.Size < 0 {
		return nil, errUtils.ErrOCIIntegrity
	}
	reader, err := store.Fetch(ctx, *desc)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	scanner := bufio.NewReader(io.LimitReader(reader, desc.Size))
	if err := expectByte(scanner, '{'); err != nil {
		return nil, integrityError(err)
	}
	for {
		next, err := nextNonSpace(scanner)
		if err != nil {
			return nil, integrityError(err)
		}
		switch next {
		case '}':
			return nil, nil
		case ',':
			continue
		case '"':
		default:
			return nil, fmt.Errorf("%w: unexpected %q in the manifest object", errUtils.ErrOCIIntegrity, next)
		}
		subject, found, err := scanField(scanner)
		if err != nil {
			return nil, integrityError(err)
		}
		if found {
			return subject, nil
		}
	}
}

// scanField consumes one object member whose opening quote was already read. It decodes the member
// only when it is the subject; every other value is skipped.
func scanField(scanner *bufio.Reader) (*ocispec.Descriptor, bool, error) {
	name, err := readKey(scanner)
	if err != nil {
		return nil, false, err
	}
	if err := expectByte(scanner, ':'); err != nil {
		return nil, false, err
	}
	if !strings.EqualFold(name, "subject") {
		return nil, false, skipValue(scanner)
	}
	var subject *ocispec.Descriptor
	if err := json.NewDecoder(scanner).Decode(&subject); err != nil {
		return nil, false, err
	}
	return subject, true, nil
}

// integrityError classifies a manifest scan failure with ErrOCIIntegrity and keeps the cause. An
// error that already carries the sentinel is returned unchanged.
func integrityError(err error) error {
	if errors.Is(err, errUtils.ErrOCIIntegrity) {
		return err
	}
	return fmt.Errorf("%w: reading the manifest subject: %w", errUtils.ErrOCIIntegrity, err)
}

// nextNonSpace returns the next byte that is not JSON whitespace.
func nextNonSpace(scanner *bufio.Reader) (byte, error) {
	for {
		next, err := scanner.ReadByte()
		if err != nil {
			return 0, err
		}
		switch next {
		case ' ', '\t', '\r', '\n':
		default:
			return next, nil
		}
	}
}

func expectByte(scanner *bufio.Reader, want byte) error {
	next, err := nextNonSpace(scanner)
	if err != nil {
		return err
	}
	if next != want {
		return fmt.Errorf("%w: expected %q in the manifest but found %q", errUtils.ErrOCIIntegrity, want, next)
	}
	return nil
}

// readKey reads an object key whose opening quote was consumed. It retains at most
// maxManifestKeyBytes of the key; an overlong key comes back empty because it cannot be "subject".
func readKey(scanner *bufio.Reader) (string, error) {
	var raw bytes.Buffer
	overflow, err := scanString(scanner, &raw, maxManifestKeyBytes)
	if err != nil || overflow {
		return "", err
	}
	if !bytes.ContainsRune(raw.Bytes(), '\\') {
		return raw.String(), nil
	}
	var name string
	if err := json.Unmarshal([]byte(`"`+raw.String()+`"`), &name); err != nil {
		return "", err
	}
	return name, nil
}

// scanString consumes a JSON string whose opening quote was already read. It copies at most limit
// bytes into keep (when keep is not nil) and reports whether the string was longer. Memory stays
// constant however long the string is.
func scanString(scanner *bufio.Reader, keep *bytes.Buffer, limit int) (bool, error) {
	overflow := false
	escaped := false
	for {
		next, err := scanner.ReadByte()
		if err != nil {
			return overflow, err
		}
		if !escaped && next == '"' {
			return overflow, nil
		}
		escaped = !escaped && next == '\\'
		if keep == nil {
			continue
		}
		if keep.Len() >= limit {
			overflow = true
			continue
		}
		keep.WriteByte(next)
	}
}

// skipValue consumes one JSON value, however deeply nested or long, without buffering it.
func skipValue(scanner *bufio.Reader) error {
	first, err := nextNonSpace(scanner)
	if err != nil {
		return err
	}
	switch first {
	case '"':
		_, err := scanString(scanner, nil, 0)
		return err
	case '{', '[':
		return skipContainer(scanner)
	default:
		return skipScalar(scanner)
	}
}

// skipContainer consumes the rest of an object or array whose opening bracket was already read.
func skipContainer(scanner *bufio.Reader) error {
	depth := 1
	for depth > 0 {
		next, err := scanner.ReadByte()
		if err != nil {
			return err
		}
		switch next {
		case '"':
			if _, err := scanString(scanner, nil, 0); err != nil {
				return err
			}
		case '{', '[':
			depth++
		case '}', ']':
			depth--
		}
	}
	return nil
}

// skipScalar consumes the rest of a number, boolean, or null, leaving the delimiter unread.
func skipScalar(scanner *bufio.Reader) error {
	for {
		next, err := scanner.ReadByte()
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
		switch next {
		case ',', '}', ']', ' ', '\t', '\r', '\n':
			return scanner.UnreadByte()
		}
	}
}
