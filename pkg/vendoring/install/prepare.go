package install

import (
	"context"
	"io"

	"github.com/cloudposse/atmos/pkg/oci"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/vendoring/lockfile"
)

// PreparedPackage owns private staged content and its prepared receipt.
// Call Close after materialization, failure, or cancellation.
type PreparedPackage struct {
	dir       string
	receipt   *lockfile.PreparedRecord
	copyFiles func() error
}

// preparationProgress keeps transport and phase observers scoped to one worker.
type preparationProgress struct {
	bytes func(int64, int64)
	phase func(string)
	retry func(int)
}

func (p preparationProgress) preparing() {
	if p.phase != nil {
		p.phase("Preparing")
	}
}

// Prepare fetches and inventories a package without changing its destination.
func Prepare(ctx context.Context, config *schema.AtmosConfiguration, pkg VendorPackage, progress func(int64, int64)) (*PreparedPackage, error) {
	defer perf.Track(config, "install.Prepare")()
	return prepareWithProgress(ctx, config, pkg, preparationProgress{bytes: progress})
}

func prepareWithProgress(ctx context.Context, config *schema.AtmosConfiguration, pkg VendorPackage, progress preparationProgress) (*PreparedPackage, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := createTempDir()
	if err != nil {
		return nil, err
	}
	if pkg.PkgType() == PkgTypeOci {
		ctx = oci.WithRetryObserver(ctx, progress.retry)
	}
	prepared, err := pkg.installer.prepare(ctx, dir, config, progress)
	if err != nil {
		removeTempDir(dir)
		return nil, err
	}
	prepared.dir = dir
	return prepared, nil
}

// Materialize copies the staged package and records its receipt under mutation locks.
func (p *PreparedPackage) Materialize(ctx context.Context, config *schema.AtmosConfiguration) error {
	defer perf.Track(config, "install.PreparedPackage.Materialize")()
	return p.receipt.Materialize(ctx, config, p.copyFiles)
}

// Close removes the private staging directory. It is safe to call repeatedly or on nil.
func (p *PreparedPackage) Close() {
	defer perf.Track(nil, "install.PreparedPackage.Close")()
	if p != nil && p.dir != "" {
		removeTempDir(p.dir)
		p.dir = ""
	}
}

type downloadProgress struct{ notify func(int64, int64) }

func (p downloadProgress) TrackProgress(_ string, current, total int64, stream io.ReadCloser) io.ReadCloser {
	defer perf.Track(nil, "install.downloadProgress.TrackProgress")()
	if p.notify == nil {
		return stream
	}
	p.notify(current, total)
	return &progressReader{ReadCloser: stream, current: current, total: total, notify: p.notify}
}

type progressReader struct {
	io.ReadCloser
	current, total int64
	notify         func(int64, int64)
}

func (r *progressReader) Read(buf []byte) (int, error) {
	defer perf.Track(nil, "install.progressReader.Read")()
	n, err := r.ReadCloser.Read(buf)
	r.current += int64(n)
	r.notify(r.current, r.total)
	return n, err
}
