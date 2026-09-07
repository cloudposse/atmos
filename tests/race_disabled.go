//go:build !race

package tests

// RaceEnabled is true when the test binary was built with -race. Used to
// skip tests that are safe under normal execution but incompatible with the
// race detector's instrumented binary layout, such as gomonkey's runtime
// machine-code patching (see SkipIfGomonkeyUnsafe).
const RaceEnabled = false
