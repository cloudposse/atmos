package azure

// File permissions.
const (
	// DirPermissions is the permission mode for Azure cache directories (owner read/write/execute only).
	DirPermissions = 0o700
	// FilePermissions is the permission mode for Azure credential files (owner read/write only).
	FilePermissions = 0o600
)

// BOM (Byte Order Mark) constants for UTF-8.
const (
	// BomMarker is the first byte of UTF-8 BOM.
	BomMarker = 0xEF
	// BomSecondByte is the second byte of UTF-8 BOM.
	BomSecondByte = 0xBB
	// BomThirdByte is the third byte of UTF-8 BOM.
	BomThirdByte = 0xBF
)

// MSAL cache field names used in azureProfile.json and MSAL cache.
// Exported for use by device_code_cache.go.
const (
	FieldHomeAccountID = "home_account_id"
	FieldEnvironment   = "environment"
	FieldRealm         = "realm"
	FieldUsername      = "username"
	FieldLocalID       = "local_account_id"
	FieldAccessToken   = "AccessToken"
	FieldUser          = "user"
)

// String format and conversion constants.
const (
	IntFormat      = "%d" // Format string for integer output.
	StrconvDecimal = 10   // Decimal base for string conversion.
	Int64BitSize   = 64   // Bit size for int64 conversion.
)

// Logging field names.
const (
	LogFieldIdentity     = "identity"     // Log field for identity name.
	LogFieldSubscription = "subscription" // Log field for subscription ID.
	LogFieldTenantID     = "tenantID"     // Log field for tenant ID.
	LogFieldExpiresOn    = "expiresOn"    // Log field for token expiration.
	LogFieldKey          = "key"          // Log field for cache key.
)
