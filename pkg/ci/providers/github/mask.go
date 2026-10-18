package github

import (
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/ci/internal/provider"
	"github.com/cloudposse/atmos/pkg/perf"
)

// Compile-time assertion that Provider can register values for log masking.
var _ provider.ValueMasker = (*Provider)(nil)

// MaskValue registers value with the runner's log masker by emitting the
// ::add-mask:: workflow command. Workflow commands are written to stderr because the
// runner parses them from the step log stream and stdout is reserved for data. The
// command is written unmasked: Atmos has already registered the value with its own
// masker, which would otherwise replace the payload with its placeholder and leave the
// runner masking nothing. An empty value is ignored since masking the empty string is meaningless.
func (p *Provider) MaskValue(value string) error {
	defer perf.Track(nil, "github.Provider.MaskValue")()

	if value == "" {
		return nil
	}
	if err := writeUnmaskedWorkflowCommand("::add-mask::" + escapeData(value)); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrCIMaskFailed, err)
	}
	return nil
}
