package errors

import "errors"

// ErrAutomation identifies invalid automation requests independently of the calling language.
var ErrAutomation = errors.New("automation request failed")
