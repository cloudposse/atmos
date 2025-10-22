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

// BadFunction - missing perf.Track.
func BadFunction() { // want "missing defer perf.Track\\(\\) call at start of public function BadFunction"
	// Function body.
}

type MyType struct{}

// GoodMethod - has perf.Track.
func (m *MyType) GoodMethod() {
	defer perf.Track(nil, "perftrack.MyType.GoodMethod")()

	// Method body.
}

// BadMethod - missing perf.Track.
func (m *MyType) BadMethod() { // want "missing defer perf.Track\\(\\) call at start of public function BadMethod"
	// Method body.
}

// privateFunction should not be checked (unexported).
func privateFunction() {
	// No perf.Track needed for unexported functions.
}
