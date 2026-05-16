package verification

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"

	"github.com/cloudposse/atmos/pkg/perf"
)

// HTTPDownloader downloads sidecar files using an HTTP client.
type HTTPDownloader struct {
	Client *http.Client
}

// Download fetches a URL and returns its response body.
func (d HTTPDownloader) Download(ctx context.Context, url string) ([]byte, error) {
	defer perf.Track(nil, "verification.HTTPDownloader.Download")()

	client := d.Client
	if client == nil {
		client = http.DefaultClient
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	// #nosec G704 -- verification sidecar URLs come from registry metadata and user configuration.
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%w: %s: HTTP %d", ErrDownloadFailed, url, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// ExecRunner runs verifier commands from PATH.
type ExecRunner struct{}

// Run executes a verifier command.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) error {
	defer perf.Track(nil, "verification.ExecRunner.Run")()

	if _, err := exec.LookPath(name); err != nil {
		return fmt.Errorf("%w: %s", ErrVerifierCommandRequired, name)
	}
	// #nosec G204 -- command and arguments are verifier metadata from trusted registries.
	cmd := exec.CommandContext(ctx, name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%w: %s %v: %w\n%s", ErrSignatureFailed, name, args, err, string(output))
	}
	return nil
}
