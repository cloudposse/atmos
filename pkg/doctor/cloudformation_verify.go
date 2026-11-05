package doctor

import (
	"context"
	"fmt"
	"reflect"
)

// verifyCloudFormationSettings rejects migrations that change resolved settings through imports
// or inheritance, including an old spelling shadowed by a modern value in another file.
func (e *Engine) verifyCloudFormationSettings(ctx context.Context, o *Options, candidates []candidate) error {
	reporter, ok := e.Stacks.(CloudFormationSettingsReporter)
	if !ok {
		return fmt.Errorf("%w: the stack validator cannot report CloudFormation settings", ErrVerification)
	}
	options := *o.rediscoveryOptions()
	options.Fix = false
	options.DryRun = false
	before, err := e.cloudFormationSettings(ctx, &options, reporter)
	if err != nil {
		return err
	}
	overlay := sourceOverlay{FileSystem: e.FS, files: map[string][]byte{}}
	for _, candidate := range candidates {
		overlay.files[candidate.doc.path] = candidate.content
	}
	candidateEngine := *e
	candidateEngine.FS = overlay
	candidateEngine.Observe = nil
	after, err := candidateEngine.cloudFormationSettings(ctx, &options, reporter)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(before, after) {
		return fmt.Errorf("%w: migration changes effective CloudFormation region or timeout through imports or inheritance", ErrVerification)
	}
	return nil
}

func (e *Engine) cloudFormationSettings(ctx context.Context, o *Options, reporter CloudFormationSettingsReporter) (map[string]any, error) {
	p, err := e.discover(ctx, o, &Report{})
	if err != nil {
		return nil, err
	}
	if !e.canCheckStacks(p) {
		return nil, fmt.Errorf("%w: CloudFormation settings cannot be compared because %s", ErrVerification, e.stackBlocker(p))
	}
	config, err := staticConfig(p)
	if err != nil {
		return nil, err
	}
	inputs, err := e.staticInputs(p, config)
	if err != nil {
		return nil, err
	}
	return reporter.CloudFormationSettings(ctx, config, inputs)
}
