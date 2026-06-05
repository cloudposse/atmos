package secret

import "errors"

// ErrUnsupportedFormat indicates an unsupported file format was requested.
var ErrUnsupportedFormat = errors.New("unsupported format (use env or json)")
