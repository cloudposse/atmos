package resolution

import "errors"

// ErrCircularDependency is returned when a circular dependency is detected.
var ErrCircularDependency = errors.New("circular dependency detected")
