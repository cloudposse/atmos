//go:build windows

package agent

import "context"

// Serve is not implemented on Windows; named pipes are a later phase.
func Serve(context.Context, Options) error { return ErrUnsupportedPlatform }
