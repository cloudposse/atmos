package generator

import "os"

// Option is a functional option for configuring generators.
type Option func(*GeneratorContext)

// WithFormat sets the output format.
func WithFormat(format Format) Option {
	return func(ctx *GeneratorContext) {
		ctx.Format = format
	}
}

// WithDryRun enables dry-run mode (no file writes).
func WithDryRun(dryRun bool) Option {
	return func(ctx *GeneratorContext) {
		ctx.DryRun = dryRun
	}
}

// WithWorkingDir sets the working directory for output files.
func WithWorkingDir(dir string) Option {
	return func(ctx *GeneratorContext) {
		ctx.WorkingDir = dir
	}
}

// ApplyOptions applies functional options to a GeneratorContext.
func ApplyOptions(ctx *GeneratorContext, opts ...Option) {
	for _, opt := range opts {
		opt(ctx)
	}
}

// WriterOption is a functional option for configuring the file writer.
type WriterOption func(*FileWriter)

// WithFileMode sets the file mode for written files.
func WithFileMode(mode os.FileMode) WriterOption {
	return func(w *FileWriter) {
		w.fileMode = mode
	}
}
