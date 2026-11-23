package component

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"os"
	"path/filepath"

	"google.golang.org/protobuf/proto"

	"github.com/cloudposse/atmos/pkg/perf"
)

// Identity binds saved plans to executable bytes, schema, and negotiated package metadata.
func (p *Provider) Identity(ctx context.Context) (string, error) {
	defer perf.Track(nil, "component.Provider.Identity")()

	h := sha256.New()
	data, err := proto.MarshalOptions{Deterministic: true}.Marshal(p.expected)
	if err != nil {
		return "", err
	}
	_, _ = h.Write(data)
	for _, path := range []string{p.executable, p.schemaPath} {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		if !filepath.IsAbs(path) {
			path = filepath.Join(p.directory, path)
		}
		file, err := os.Open(path)
		if err != nil {
			return "", err
		}
		_, err = io.Copy(h, file)
		closeErr := file.Close()
		if err != nil {
			return "", err
		}
		if closeErr != nil {
			return "", closeErr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
