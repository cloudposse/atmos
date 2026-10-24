package emulator

const (
	testDriverName  = "test/aws"
	testDriverImage = "test/aws:latest"
	testDriverPort  = 4566
)

// testDriver is a minimal in-package driver so the core (Spec/Manager) tests stay
// decoupled from the concrete driver products, which live in the pkg/emulator/driver
// subpackage. Its Profile returns sentinel values (TEST_DRIVER, test_flag) so the
// tests assert the manager→driver wiring, not a specific product's profile content.
type testDriver struct{}

func (testDriver) Name() string   { return testDriverName }
func (testDriver) Target() string { return TargetAWS }

func (testDriver) Defaults() ContainerDefaults {
	return ContainerDefaults{Image: testDriverImage, Ports: []int{testDriverPort}}
}

func (testDriver) Profile(ep *Endpoint) Profile {
	env := map[string]string{"TEST_DRIVER": "1"}
	url := ep.URL("http")
	if url != "" {
		env["AWS_ENDPOINT_URL"] = url
	}
	return Profile{Env: env, ResolverURL: url, Provider: map[string]any{"test_flag": true}}
}

func init() {
	RegisterDriver(testDriver{})
}
