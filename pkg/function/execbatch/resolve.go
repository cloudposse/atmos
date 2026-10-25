package execbatch

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"slices"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Resolve leaves serial evaluation unchanged and delegates only opted-in !exec work.
// The caller supplies the edition's explicit-tag policy for automatic evaluation.
func Resolve(config *schema.AtmosConfiguration, data map[string]any, skip []string, explicitTags bool) (map[string]any, error) {
	defer perf.Track(config, "execbatch.Resolve")()
	if config == nil {
		return data, nil
	}
	settings := config.Settings.Functions.Exec
	if settings.Concurrency() < 1 {
		return nil, fmt.Errorf("%w: settings.functions.exec.max_concurrency must be at least 1", errUtils.ErrInvalidConfig)
	}
	if settings.Concurrency() == 1 || slices.Contains(skip, "exec") {
		return data, nil
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	return evaluate(ctx, data, settings.Concurrency(), nil, explicitTags)
}
