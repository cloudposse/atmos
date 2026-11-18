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

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// maxManifestKeyBytes bounds how much of an object key the subject scan retains. A longer key can
// never be the "subject" field, so the remainder is skipped without being kept.
const maxManifestKeyBytes = 64

// readSubject streams a stored manifest or index and returns its subject, or nil when it has none.
// The scan is bounded by the descriptor size and the whole body is verified against its size and
// digest before any result is returned. It keeps constant memory: every other top-level field,
// including arbitrarily long strings, is skipped byte by byte without being buffered. A large
// manifest without a subject is therefore never rejected.
func readSubject(ctx context.Context, store Fetcher, desc *ocispec.Descriptor) (*ocispec.Descriptor, error) {
	if desc.Size < 0 {
		return nil, errUtils.ErrOCIIntegrity
	}
	if err := desc.Digest.Validate(); err != nil {
		return nil, wrapIntegrity(err)
	}
	reader, err := store.Fetch(ctx, *desc)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	// Some stores, such as ORAS local storage, ignore the fetch context, so every read is guarded here.
	subject, err := verifySubject(ctx, contextReader{ctx: ctx, reader: reader}, desc)
	if err != nil {
		return nil, canceledOr(ctx, err)
	}
	// A cancellation that raced the final reads must not be reported as a verified result.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return subject, nil
}

// verifySubject scans the body for the subject and then reads the remainder so the size and digest
// cover every byte.
func verifySubject(ctx context.Context, reader io.Reader, desc *ocispec.Descriptor) (*ocispec.Descriptor, error) {
	check := &bodyCheck{verifier: desc.Digest.Verifier()}
	scanner := bufio.NewReader(io.TeeReader(io.LimitReader(reader, desc.Size), check))
	if err := expectByte(scanner, '{'); err != nil {
		return nil, integrityError(err)
	}
	subject, err := scanForSubject(scanner)
	if err != nil {
		return nil, err
	}
	// The scan may stop at the subject or at the closing brace; either way the rest of the body is read
	// so the size and digest cover every byte before the result is trusted.
	if _, err := io.Copy(io.Discard, scanner); err != nil {
		return nil, integrityError(err)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := check.finish(reader, desc.Size); err != nil {
		return nil, err
	}
	return subject, nil
}

// canceledOr returns the context error when the context ended, since a canceled read is not an
// integrity failure; otherwise it returns err unchanged.
func canceledOr(ctx context.Context, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("reading the manifest subject: %w", ctxErr)
	}
	return err
}

// contextReader fails a read as soon as the context is done.
type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (c contextReader) Read(p []byte) (int, error) {
	defer perf.Track(nil, "oci.contextReader.Read")()

	if err := c.ctx.Err(); err != nil {
		return 0, err
	}
	return c.reader.Read(p)
}

// bodyCheck accumulates the size and digest of the bytes a scan consumed.
type bodyCheck struct {
	verifier digest.Verifier
	size     int64
}

func (b *bodyCheck) Write(p []byte) (int, error) {
	defer perf.Track(nil, "oci.bodyCheck.Write")()

	b.size += int64(len(p))
	return b.verifier.Write(p)
}

// finish reports an integrity failure unless exactly want bytes were read, nothing follows them in
// source, and they match the descriptor digest.
func (b *bodyCheck) finish(source io.Reader, want int64) error {
	if b.size != want {
		return fmt.Errorf("%w: the manifest has %d bytes but %d were declared", errUtils.ErrOCIIntegrity, b.size, want)
	}
	var extra [1]byte
	for {
		n, err := source.Read(extra[:])
		if n > 0 {
			return fmt.Errorf("%w: the manifest is longer than the declared %d bytes", errUtils.ErrOCIIntegrity, want)
		}
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return integrityError(err)
		}
	}
	if !b.verifier.Verified() {
		return fmt.Errorf("%w: the manifest does not match its digest", errUtils.ErrOCIIntegrity)
	}
	return nil
}

// scanForSubject reads the members of a manifest object, after its opening brace, until it finds
// the subject descriptor or the object ends.
func scanForSubject(scanner *bufio.Reader) (*ocispec.Descriptor, error) {
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
