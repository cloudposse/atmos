package docker

// dockerConfig is a typed view of the auths section of config.json, used by
// tests to assert on WriteAuth/RemoveAuth output.
type dockerConfig struct {
	Auths map[string]authEntry `json:"auths,omitempty"`
}

// authEntry represents a single auth entry in Docker config.
type authEntry struct {
	Auth string `json:"auth,omitempty"`
}
