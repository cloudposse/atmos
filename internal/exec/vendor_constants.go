package exec

// Constants for vendor operations to avoid magic strings.
const (
	// YAML field names.
	yamlComponentField = "component:"
	yamlVersionField   = "version:"

	// File permissions.
	vendorDefaultFilePermissions = 0o600

	// Log formatting.
	logMessageFormat = "%s\n"

	// Version defaults.
	defaultVersionLatest = "latest"

	// Template markers.
	templateStartMarker = "{{"

	// Component key for logging.
	componentLogKey = "component"
)
