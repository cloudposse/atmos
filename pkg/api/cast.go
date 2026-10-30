package api

import (
	"context"

	"github.com/cloudposse/atmos/pkg/castplan"
)

// CastLibrary is the optional recording service supplied by a step-library host.
// ValidateCast performs structural checks only; GenerateCast checks the staged
// recording through validate before committing it to its destination.
type CastLibrary interface {
	ValidateCast(*castplan.Plan, *StepCall) error
	GenerateCast(context.Context, *castplan.Plan, *StepCall, func(string) error) (string, error)
}
