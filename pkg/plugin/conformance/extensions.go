package conformance

import (
	"context"
	"errors"

	errUtils "github.com/cloudposse/atmos/errors"
)

// extensions runs registered local checks after the package handshake succeeds.
// Live integration steps remain separately selected by the caller.
func (s *suite) extensions(ctx context.Context, filter string, report *Report) error {
	for _, binding := range s.bindings {
		descriptor, found := s.registry.Get(binding.Capability)
		if !found {
			return errUtils.ErrPluginUnsupportedExtension
		}
		name := "extension/" + binding.Capability + "/" + binding.Name
		if descriptor.Test == nil || !matches(filter, name) {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		err := descriptor.Test(ctx, binding)
		report.add(Result{Name: name, Passed: err == nil, Detail: failureDetail(err)})
		if err != nil {
			return errors.Join(errUtils.ErrTestsFailed, err)
		}
	}
	return nil
}
