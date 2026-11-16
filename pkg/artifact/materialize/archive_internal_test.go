package materialize

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencontainers/go-digest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
)

const (
	internalBigBytes = 2 << 20
	internalMaxAfter = 256 << 10
)

// cancelOnBytesReader cancels once it served cancelAfter bytes and counts the bytes served after.
type cancelOnBytesReader struct {
	data        []byte
	cancel      context.CancelFunc
	cancelAfter int

	served        int
	afterCanceled int
	canceled      bool
}

func (c *cancelOnBytesReader) Read(p []byte) (int, error) {
	if len(c.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p, c.data)
	c.data = c.data[n:]
	c.served += n
	if c.canceled {
		c.afterCanceled += n
	}
	if !c.canceled && c.served >= c.cancelAfter {
		c.canceled = true
		c.cancel()
	}
	return n, nil
}

func noise(size int) []byte {
	data := make([]byte, size)
	random := rand.New(rand.NewPCG(3, 4))
	for index := range data {
		data[index] = byte(random.Uint32())
	}
	return data
}

// newTestTree returns a tree over a temporary root whose inventory holds one file entry.
func newTestTree(t *testing.T, name string, body []byte) (*tree, string) {
	t.Helper()
	dir := t.TempDir()
	root, err := os.OpenRoot(dir)
	require.NoError(t, err)
	t.Cleanup(func() { _ = root.Close() })
	record := &artifact.Record{Inventory: []artifact.Entry{{Path: name, Type: artifact.EntryFile, Size: int64(len(body)), Mode: 0o644, Digest: digest.FromBytes(body)}}}
	return newTree(root, record), dir
}

func TestApplyMember_StopsInProgressCopyWhenCanceled(t *testing.T) {
	body := noise(internalBigBytes)
	tests := []struct {
		name        string
		preCanceled bool
	}{
		{"pre-canceled", true},
		{"canceled mid-copy", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr, _ := newTestTree(t, "big.bin", body)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tt.preCanceled {
				cancel()
			}
			reader := &cancelOnBytesReader{data: body, cancel: cancel, cancelAfter: 64 << 10}
			err := applyMember(ctx, tr, &managedChecker{}, &archiveMember{name: "big.bin", kind: memberFile, size: int64(len(body)), body: reader})
			require.ErrorIs(t, err, context.Canceled)
			assert.Less(t, reader.afterCanceled, internalMaxAfter, "the copy stops soon after the cancellation")
			assert.Less(t, reader.served, len(body), "the member is not read to the end")
		})
	}
}

func TestApplyZipFile_StopsInflatingWhenCanceled(t *testing.T) {
	body := noise(internalBigBytes)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, err := zw.Create("big.bin")
	require.NoError(t, err)
	_, err = w.Write(body)
	require.NoError(t, err)
	require.NoError(t, zw.Close())
	zr, err := zip.NewReader(bytes.NewReader(buf.Bytes()), int64(buf.Len()))
	require.NoError(t, err)
	require.Len(t, zr.File, 1)

	tr, dir := newTestTree(t, "big.bin", body)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err = applyZipFile(ctx, tr, zr.File[0], &managedChecker{})
	require.ErrorIs(t, err, context.Canceled)
	info, statErr := os.Stat(filepath.Join(dir, "big.bin"))
	if statErr == nil {
		assert.Zero(t, info.Size(), "a canceled member writes no bytes")
	}
}

// paxRecord encodes one PAX extended header record, whose leading decimal counts its own digits.
func paxRecord(key, value string) string {
	body := " " + key + "=" + value + "\n"
	length := len(body) + 1
	for len(fmt.Sprint(length))+len(body) != length {
		length = len(fmt.Sprint(length)) + len(body)
	}
	return fmt.Sprintf("%d%s", length, body)
}

// rawPaxHeader hand-encodes a local PAX extended header, which archive/tar refuses to write itself.
func rawPaxHeader(records string) []byte {
	const block = 512
	header := make([]byte, block)
	copy(header[0:], "pax")
	copy(header[100:], "0000644\x00")
	copy(header[108:], "0000000\x00")
	copy(header[116:], "0000000\x00")
	copy(header[124:], fmt.Sprintf("%011o\x00", len(records)))
	copy(header[136:], "00000000000\x00")
	copy(header[148:], "        ")
	header[156] = tar.TypeXHeader
	copy(header[257:], "ustar\x0000")
	sum := 0
	for _, b := range header {
		sum += int(b)
	}
	copy(header[148:], fmt.Sprintf("%06o\x00 ", sum))
	padded := make([]byte, (len(records)+block-1)/block*block)
	copy(padded, records)
	return append(header, padded...)
}

// successivePaxTar returns a tar stream of count local PAX headers in a row that are followed by one
// regular file. Reading the headers is metadata work that precedes any visible member.
func successivePaxTar(t *testing.T, count int, name string, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	for index := range count {
		buf.Write(rawPaxHeader(paxRecord("comment", fmt.Sprintf("header-%d", index))))
	}
	writer := tar.NewWriter(&buf)
	require.NoError(t, writer.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Size: int64(len(body)), Mode: 0o644}))
	_, err := writer.Write(body)
	require.NoError(t, err)
	require.NoError(t, writer.Close())
	return buf.Bytes()
}

func TestFillTar_CancellationInterruptsMetadataReads(t *testing.T) {
	body := []byte("payload")
	const (
		headers     = 2000
		cancelAfter = 8 << 10
		// One block of read-ahead is the most a reader may serve after the cancellation.
		maxAfterCanceled = 4 << 10
	)
	tests := []struct {
		name        string
		preCanceled bool
	}{
		{"canceled before the first header", true},
		{"canceled between successive headers", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			data := successivePaxTar(t, headers, "file.txt", body)
			require.Greater(t, len(data), 100*cancelAfter, "the headers must outlast the cancellation point")
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader := &cancelOnBytesReader{data: data, cancel: cancel, cancelAfter: cancelAfter}
			if tc.preCanceled {
				cancel()
			}
			target, _ := newTestTree(t, "file.txt", body)
			checker := &managedChecker{documents: map[string]artifact.Document{}, seen: map[string]bool{}}

			err := fillTar(ctx, target, reader, false, checker)

			require.ErrorIs(t, err, context.Canceled)
			assert.False(t, errors.Is(err, errUtils.ErrArtifactIntegrityFailed), "cancellation is not an integrity failure")
			assert.LessOrEqual(t, reader.afterCanceled, maxAfterCanceled, "metadata reads must stop once the context is canceled")
		})
	}
}

// A gzip trailer that fails its checksum is only noticed while the tail of the stream is drained after
// the last tar member; it must still be reported as an integrity failure, not as a bare gzip error.
func TestFillTar_ClassifiesACorruptGzipTrailer(t *testing.T) {
	body := []byte("payload")
	var archive bytes.Buffer
	gz := gzip.NewWriter(&archive)
	tw := tar.NewWriter(gz)
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "file.txt", Typeflag: tar.TypeReg, Size: int64(len(body)), Mode: 0o644}))
	_, err := tw.Write(body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	data := archive.Bytes()
	data[len(data)-8] ^= 0xff // The CRC32 in the gzip trailer.

	target, _ := newTestTree(t, "file.txt", body)
	checker := &managedChecker{documents: map[string]artifact.Document{}, seen: map[string]bool{}}
	err = fillTar(t.Context(), target, bytes.NewReader(data), true, checker)

	require.ErrorIs(t, err, errUtils.ErrArtifactIntegrityFailed)
}
