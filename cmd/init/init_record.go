package initcmd

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	projectconfig "github.com/cloudposse/atmos/pkg/project/config"
)

// resolveRecordedInitSource lets a source-less update reuse the current project's
// original scaffold. The generator still loads saved answers and applies --set
// overrides through its normal merge path.
func resolveRecordedInitSource(opts *initOptions) error {
	if !opts.update || opts.templateName != "" {
		return nil
	}
	target := opts.targetDir
	if target == "" {
		target = "."
	}
	record, err := projectconfig.LoadProjectRecord(target)
	if err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrInitialization, err)
	}
	if record == nil && opts.interactive {
		return nil // Preserve the existing template/target prompts outside a recorded project.
	}
	if record == nil {
		return errUtils.Build(errUtils.ErrInitialization).
			WithExplanation("Cannot infer an update source without a saved .atmos/scaffold.yaml project record").
			WithHint("Run init --update from a generated project, or specify the template and target directory explicitly").Err()
	}
	source := record.Spec.Source
	if source == projectconfig.SourceEmbedded || source == "" {
		source = record.Metadata.Name
	}
	opts.templateName = source
	opts.targetDir = target
	return nil
}
