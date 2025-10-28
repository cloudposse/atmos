package perftrack

// Mock perf package for testing.
type mockPerf struct{}

func (mockPerf) Track(atmosConfig interface{}, name string) func() {
	return func() {}
}

var perf = mockPerf{}

// Good function - has perf.Track.
func GoodFunction() {
	defer perf.Track(nil, "perftrack.GoodFunction")()

	// Function body.
}

// BadFunction - missing perf.Track (no atmosConfig param).
func BadFunction() { // want "missing defer perf.Track\\(\\) call at start of public function BadFunction; add: defer perf.Track\\(nil, \"perftrack.BadFunction\"\\)\\(\\)"
	// Function body.
}

// BadFunctionWithConfig - missing perf.Track (has atmosConfig param).
func BadFunctionWithConfig(atmosConfig interface{}) { // want "missing defer perf.Track\\(\\) call at start of public function BadFunctionWithConfig; add: defer perf.Track\\(atmosConfig, \"perftrack.BadFunctionWithConfig\"\\)\\(\\)"
	// Function body.
}

type MyType struct{}

// GoodMethod - has perf.Track.
func (m *MyType) GoodMethod() {
	defer perf.Track(nil, "perftrack.MyType.GoodMethod")()

	// Method body.
}

// BadMethod - missing perf.Track.
func (m *MyType) BadMethod() { // want "missing defer perf.Track\\(\\) call at start of public function BadMethod; add: defer perf.Track\\(nil, \"perftrack.MyType.BadMethod\"\\)\\(\\)"
	// Method body.
}

// privateFunction should not be checked (unexported).
func privateFunction() {
	// No perf.Track needed for unexported functions.
}
