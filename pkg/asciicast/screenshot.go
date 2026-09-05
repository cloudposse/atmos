package asciicast

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ErrScreenshotRenderFailed indicates a marker screenshot could not be
// rendered or written to disk.
var ErrScreenshotRenderFailed = errUtils.ErrScreenshotRenderFailed

const screenshotRenderErrorFormat = "%w: %s: %w"

// RenderMarkerScreenshots scans a finished cast recording for "m" marker
// events (written by session/step "screenshot" actions via
// pkg/io.RecordMarker) and rasterizes a PNG of the terminal's cell-grid state
// as of each marker's timestamp, writing it to the path carried in the
// marker's content. It is a no-op if the recording has no marker events.
func RenderMarkerScreenshots(castPath string) error {
	defer perf.Track(nil, "asciicast.RenderMarkerScreenshots")()

	header, events, err := ReadEvents(castPath)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Stream != "m" || event.Data == "" {
			continue
		}
		grid := buildGridUpTo(&header, events, event.Time)
		img, err := rasterizeGrid(grid)
		if err != nil {
			return err
		}
		if err := writeScreenshotPNG(event.Data, img); err != nil {
			return err
		}
	}
	return nil
}

// writeScreenshotPNG renders img as a PNG to a temporary file in path's
// directory, then renames it into place only once encoding, flushing, and
// closing all succeed -- an existing screenshot at path is never truncated
// (and so never lost) by a failed render.
func writeScreenshotPNG(path string, img image.Image) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, castDirPerm); err != nil {
		return fmt.Errorf(screenshotRenderErrorFormat, ErrScreenshotRenderFailed, path, err)
	}
	tmp, err := os.CreateTemp(dir, ".screenshot-*.png.tmp")
	if err != nil {
		return fmt.Errorf(screenshotRenderErrorFormat, ErrScreenshotRenderFailed, path, err)
	}
	tmpPath := tmp.Name()
	defer func() { _ = os.Remove(tmpPath) }()

	if err := png.Encode(tmp, img); err != nil {
		_ = tmp.Close()
		return fmt.Errorf(screenshotRenderErrorFormat, ErrScreenshotRenderFailed, path, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf(screenshotRenderErrorFormat, ErrScreenshotRenderFailed, path, err)
	}
	if err := os.Rename(tmpPath, path); err != nil { // #nosec G703 -- path is the caller's own resolved screenshot output path, not external input.
		return fmt.Errorf(screenshotRenderErrorFormat, ErrScreenshotRenderFailed, path, err)
	}
	return nil
}
