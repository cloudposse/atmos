package filematch

import "fmt"

// ErrInvalidPattern represents an error due to an invalid glob pattern.
type ErrInvalidPattern struct {
	Pattern string
	Err     error
}

func (e ErrInvalidPattern) Error() string {
	return fmt.Sprintf("invalid pattern %q: %v", e.Pattern, e.Err)
}
