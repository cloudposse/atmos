package s3test

import (
	"bufio"
	"bytes"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// hexBase is the radix of aws-chunked chunk sizes.
const hexBase = 16

// readBody returns the request payload, decoding aws-chunked framing when the SDK used it.
func readBody(r *http.Request) ([]byte, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if !isChunked(r) {
		return raw, nil
	}
	return decodeAWSChunked(raw)
}

func isChunked(r *http.Request) bool {
	return strings.Contains(r.Header.Get("Content-Encoding"), "aws-chunked") ||
		strings.HasPrefix(r.Header.Get("X-Amz-Content-Sha256"), "STREAMING-")
}

// decodeAWSChunked strips the aws-chunked framing: "<hex-size>[;ext]\r\n<data>\r\n" repeated, ended
// by a zero-size chunk and optional trailers.
func decodeAWSChunked(raw []byte) ([]byte, error) {
	reader := bufio.NewReader(bytes.NewReader(raw))
	var out bytes.Buffer
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		sizeText, _, _ := strings.Cut(strings.TrimSpace(line), ";")
		size, err := strconv.ParseInt(sizeText, hexBase, bitSize64)
		if err != nil {
			return nil, err
		}
		if size == 0 {
			return out.Bytes(), nil
		}
		if _, err := io.CopyN(&out, reader, size); err != nil {
			return nil, err
		}
		if _, err := reader.Discard(2); err != nil {
			return nil, err
		}
	}
}
