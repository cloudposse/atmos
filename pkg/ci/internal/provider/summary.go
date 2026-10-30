package provider

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/filelock"
)

const (
	// GitHub rejects the entire step summary when its file exceeds 1 MiB.
	stepSummaryMaxBytes = 1024 * 1024
	summaryLockTimeout  = 5 * time.Second
	// Reserve room for an admonition even when accepted content fills its budget.
	stepSummaryContentMaxBytes = stepSummaryMaxBytes - int64(len(SummarySizeWarning))
)

// SummarySizeWarning is the GitHub admonition used when summary content is omitted.
const SummarySizeWarning = "\n\n> [!WARNING]\n> Output exceeds the maximum summary size. Consider reducing scope using `--tags` or `--labels`.\n"

// appendSummaryFile serializes Atmos writers across processes so checking the
// existing file and appending masked content form one transaction. External
// writers that do not acquire this advisory lock remain outside our control.
func appendSummaryFile(path, content string) error {
	ctx, cancel := context.WithTimeout(context.Background(), summaryLockTimeout)
	defer cancel()
	err := filelock.New(path+".lock").WithExclusive(ctx, func() error {
		return appendSummaryWithinLimit(path, content)
	})
	if err != nil && !errors.Is(err, errUtils.ErrCISummaryWriteFailed) {
		return fmt.Errorf("%w: failed to lock summary %s: %w", errUtils.ErrCISummaryWriteFailed, path, err)
	}
	return err
}

// appendSummaryWithinLimit preserves existing content and complete Markdown
// sections by rejecting an append that cannot fit, rather than cutting it off.
func appendSummaryWithinLimit(path, content string) error {
	var size int64
	info, err := os.Stat(path)
	if err == nil {
		size = info.Size()
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("%w: failed to stat summary %s: %w", errUtils.ErrCISummaryWriteFailed, path, err)
	}
	budget := stepSummaryContentMaxBytes
	if size > 0 && size <= stepSummaryMaxBytes && int64(len(content)) > budget-size {
		existing, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("%w: failed to read summary %s: %w", errUtils.ErrCISummaryWriteFailed, path, err)
		}
		if strings.Contains(string(existing), SummarySizeWarning) {
			// An existing warning already occupies its reserved bytes.
			budget = stepSummaryMaxBytes
		}
	}
	if int64(len(content)) > budget-size {
		return errors.Join(
			fmt.Errorf("%w: skipping summary append to %s: %d existing bytes plus %d new bytes exceed the available budget under the 1 MiB step-summary limit; consider reducing scope using --tags or --labels",
				errUtils.ErrCISummaryWriteFailed, path, size, len(content)),
			appendSummarySizeWarning(path, size),
		)
	}
	return AppendFile(path, content, errUtils.ErrCISummaryWriteFailed)
}

// appendSummarySizeWarning adds one visible warning while the caller holds the
// file lock. An externally filled file may leave no room; never overwrite it.
func appendSummarySizeWarning(path string, size int64) error {
	if size > stepSummaryContentMaxBytes {
		return nil
	}
	content, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%w: failed to read summary %s: %w", errUtils.ErrCISummaryWriteFailed, path, err)
	}
	if strings.Contains(string(content), SummarySizeWarning) {
		return nil
	}
	return AppendFile(path, SummarySizeWarning, errUtils.ErrCISummaryWriteFailed)
}
