package setup

import (
	errUtils "github.com/cloudposse/atmos/errors"
	generatorUI "github.com/cloudposse/atmos/pkg/generator/ui"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/terminal"
)

// GeneratorContext holds the I/O and UI components needed for generator commands.
type GeneratorContext struct {
	IOContext iolib.Context
	Terminal  terminal.Terminal
	UI        *generatorUI.InitUI
}

// NewGeneratorContext creates a new generator context with I/O and terminal setup.
// This helper reduces boilerplate in init and scaffold commands.
func NewGeneratorContext() (*GeneratorContext, error) {
	// Create I/O context
	ioCtx, err := iolib.NewContext()
	if err != nil {
		return nil, errUtils.Build(errUtils.ErrCreateIOContext).
			WithExplanation("Failed to create I/O context").
			WithHint("Check terminal capabilities and permissions").
			WithHint("Ensure stdout/stderr are accessible").
			WithHint("Try running with `ATMOS_LOGS_LEVEL=Debug` for more details").
			WithExitCode(1).
			Err()
	}

	// Create terminal writer for I/O
	termWriter := iolib.NewTerminalWriter(ioCtx)
	term := terminal.New(terminal.WithIO(termWriter))

	// Create UI instance
	ui := generatorUI.NewInitUI(ioCtx, term)

	return &GeneratorContext{
		IOContext: ioCtx,
		Terminal:  term,
		UI:        ui,
	}, nil
}
