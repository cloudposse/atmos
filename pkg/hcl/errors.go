package hcl

import "errors"

var (
	// ErrHCLAddressNotFound is returned when an address does not resolve to
	// an attribute or a block.
	ErrHCLAddressNotFound = errors.New("HCL address not found")

	// ErrHCLUpdateFailed is returned when an edit operation fails (invalid
	// address syntax, attribute already exists for an append, etc.).
	ErrHCLUpdateFailed = errors.New("failed to update HCL")

	// ErrHCLInvalidResult is returned when an edit would produce HCL that
	// fails to parse. The strict editing contract: never persist this.
	ErrHCLInvalidResult = errors.New("edit would produce invalid HCL")

	// ErrHCLParseFailed is returned when the source content is not valid HCL.
	ErrHCLParseFailed = errors.New("failed to parse HCL")

	// ErrHCLReadFile is returned when a file cannot be read.
	ErrHCLReadFile = errors.New("failed to read file")

	// ErrHCLWriteFile is returned when a file cannot be written.
	ErrHCLWriteFile = errors.New("failed to write file")
)
