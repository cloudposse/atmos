package github

import (
	"fmt"
	stdio "io"

	errUtils "github.com/cloudposse/atmos/errors"
	iolib "github.com/cloudposse/atmos/pkg/io"
)

// writeUnmaskedWorkflowCommand writes one workflow command line to stderr without masking. It exists
// for ::add-mask::, whose payload is the very secret the masker would replace with its placeholder.
func writeUnmaskedWorkflowCommand(line string) error {
	if _, err := stdio.WriteString(iolib.GetContext().RawUI(), line+"\n"); err != nil {
		return fmt.Errorf("%w: %w", errUtils.ErrWriteToStream, err)
	}
	return nil
}
