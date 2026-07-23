package yaml

import "errors"

var (
	// ErrNilAtmosConfig is returned when atmosConfig is nil.
	ErrNilAtmosConfig = errors.New("atmosConfig cannot be nil")

	// ErrIncludeInvalidArguments is returned when !include has invalid arguments.
	ErrIncludeInvalidArguments = errors.New("invalid number of arguments in the !include function")

	// ErrIncludeFileNotFound is returned when !include references a non-existent file.
	ErrIncludeFileNotFound = errors.New("the !include function references a file that does not exist")

	// ErrIncludeAbsPath is returned when converting to absolute path fails.
	ErrIncludeAbsPath = errors.New("failed to convert the file path to an absolute path in the !include function")

	// ErrIncludeProcessFailed is returned when processing stack manifest fails.
	ErrIncludeProcessFailed = errors.New("failed to process the stack manifest with the !include function")

	// ErrInvalidYAMLFunction is returned when a YAML function has invalid syntax.
	ErrInvalidYAMLFunction = errors.New("invalid Atmos YAML function")

	// ErrInvalidYAMLExpression is returned when a dot-path or yq expression cannot be parsed or evaluated.
	ErrInvalidYAMLExpression = errors.New("invalid YAML path or expression")

	// ErrYAMLPathNotFound is returned when a requested path does not exist in the document.
	ErrYAMLPathNotFound = errors.New("YAML path not found")

	// ErrYAMLUpdateFailed is returned when an edit operation fails to produce a valid document.
	ErrYAMLUpdateFailed = errors.New("failed to update YAML")

	// ErrYAMLAnchorAltered is returned when an edit would alter or expand a YAML anchor or alias,
	// which the strict editing contract forbids.
	ErrYAMLAnchorAltered = errors.New("edit would alter or expand a YAML anchor or alias")

	// ErrYAMLDuplicateAnchor is returned when a document defines the same anchor name more than
	// once. Aliases before and after the redefinition resolve to different values, so the anchor
	// guard cannot safely verify an edit; the duplicates must be renamed first.
	ErrYAMLDuplicateAnchor = errors.New("duplicate YAML anchor definition")

	// ErrYAMLMultiDocUnsupported is returned when the editor is given a stream containing more
	// than one YAML document. Edits would silently apply to every document, so multi-document
	// files are rejected explicitly.
	ErrYAMLMultiDocUnsupported = errors.New("multi-document YAML streams are not supported by the YAML editor")

	// ErrParseYAML is returned when YAML content cannot be parsed.
	ErrParseYAML = errors.New("failed to parse YAML")

	// ErrReadFile is returned when a file cannot be read.
	ErrReadFile = errors.New("failed to read file")

	// ErrWriteFile is returned when a file cannot be written.
	ErrWriteFile = errors.New("failed to write file")
)
