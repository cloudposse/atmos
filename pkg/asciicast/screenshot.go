package asciicast

import (
	"image"
	"image/png"
	"os"

	"github.com/cloudposse/atmos/pkg/perf"
)

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

func writeScreenshotPNG(path string, img image.Image) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return png.Encode(file, img)
}
