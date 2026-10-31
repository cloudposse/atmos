package errors

import "errors"

// Native plugin, shared OCI, and extension registration sentinels.
var (
	// ErrPluginReleasePlan identifies invalid or inconsistent committed release inputs.
	ErrPluginReleasePlan = errors.New("plugin release planning failed")
	// ErrPluginBuild identifies a failed native plugin compilation.
	ErrPluginBuild = errors.New("plugin build failed")
	// ErrPluginCatalogConflict prevents replacing newer or inconsistent catalog metadata.
	ErrPluginCatalogConflict = errors.New("plugin catalog changed; reload it before publishing")
	// ErrPluginInUse prevents removal of a package still referenced by auth configuration.
	ErrPluginInUse = errors.New("plugin is still referenced by configuration")
	// ErrPluginImmutableRelease rejects replacing an already published version.
	ErrPluginImmutableRelease = errors.New("plugin version already exists with different bytes; publish a new version")
	// ErrPluginTransactionPending prevents use of a partially committed dependency change.
	ErrPluginTransactionPending = errors.New("plugin transaction is incomplete or conflicts with an external edit")
	// ErrPluginSignatureInvalid rejects unsigned, untrusted, or invalidly signed releases.
	ErrPluginSignatureInvalid = errors.New("plugin signature does not satisfy publisher policy")
	// ErrPluginOfflineArtifact identifies missing bytes during explicit offline installation.
	ErrPluginOfflineArtifact = errors.New("plugin artifact is unavailable offline; provide a verified OCI layout")
	// ErrPluginVersionNotFound identifies an unsatisfied release constraint.
	ErrPluginVersionNotFound = errors.New("no plugin release satisfies the version constraint")
	// ErrPluginUnsupportedPlatform indicates no payload matches the running process.
	ErrPluginUnsupportedPlatform = errors.New("plugin release does not support this platform")
	// ErrPluginFrozenDrift indicates dependency declarations or artifact bytes disagree with the lock.
	ErrPluginFrozenDrift = errors.New("plugin declarations or artifacts differ from the release lock")
	// ErrOCIIntegrity indicates content does not match a verified OCI descriptor.
	ErrOCIIntegrity = errors.New("OCI artifact integrity check failed")
	// ErrOCIRegistry indicates a shared registry transport or credential operation failed.
	ErrOCIRegistry = errors.New("OCI registry operation failed")
	// ErrOCILayout indicates an incomplete or invalid OCI image layout.
	ErrOCILayout                  = errors.New("invalid OCI image layout")
	ErrPluginNotInstalled         = errors.New("plugin is not installed; run atmos plugin install")
	ErrDuplicatePluginExtension   = errors.New("duplicate plugin extension registration")
	ErrPluginUnsupportedExtension = errors.New("unsupported plugin extension")
	ErrPluginInteractionRequired  = errors.New("plugin requires interactive authentication")
	ErrPluginInvalidConfig        = errors.New("invalid plugin configuration")
	ErrPluginAPI                  = errors.New("plugin API operation failed")
	ErrPluginProtocol             = errors.New("plugin protocol error")
	ErrPluginUnauthorized         = errors.New("plugin API invocation is not authorized")
	ErrPluginPayloadTooLarge      = errors.New("plugin payload exceeds size limit")

	// ErrPluginComponentsFailed identifies selected components whose configuration provider reported a failure.
	ErrPluginComponentsFailed = errors.New("selected components failed configuration discovery; no infrastructure command was run")

	// ErrPluginBundlePathForbidden rejects hidden, credential-like, or special files in a plugin bundle.
	ErrPluginBundlePathForbidden = errors.New("plugin bundle contains a forbidden file")
	// ErrPluginBundleExecutableInvalid rejects an executable that is not a native binary for its declared platform.
	ErrPluginBundleExecutableInvalid = errors.New("plugin executable is not a native binary for its declared platform")
	// ErrPluginBundleSecretDetected rejects metadata files that contain secret-looking content.
	ErrPluginBundleSecretDetected = errors.New("plugin bundle metadata contains secret-looking content")
	// ErrPluginCacheCorrupt identifies a cached release that no longer matches its verified digest.
	ErrPluginCacheCorrupt = errors.New("cached plugin release is corrupt")
	// ErrPluginPathPackage rejects a lifecycle change to a package whose path comes from configuration.
	ErrPluginPathPackage = errors.New("plugin package runs from a local path set in configuration")
	// ErrPluginRegistryNotFound identifies an unknown plugin registry alias.
	ErrPluginRegistryNotFound = errors.New("plugin registry is not configured")
	// ErrPluginTestsNotMatched identifies a --run filter that selected no plugin tests.
	ErrPluginTestsNotMatched = errors.New("no plugin tests matched the filter")
	// ErrPluginCredentialStore identifies registry credentials that could not be stored or removed.
	ErrPluginCredentialStore = errors.New("plugin registry credentials could not be stored")
	// ErrPluginCatalogEntryNotFound identifies a catalog that does not list the requested package.
	ErrPluginCatalogEntryNotFound = errors.New("package not found in catalog")
	// ErrPluginCatalogVersionNotFound identifies a catalog entry that does not list the requested version.
	ErrPluginCatalogVersionNotFound = errors.New("version not found in catalog")
	// ErrPluginSourceIsPath identifies a filesystem path passed where an OCI source or selector is required.
	ErrPluginSourceIsPath = errors.New("plugin source looks like a filesystem path")
	// ErrPluginTimeout identifies a plugin call that exceeded its deadline.
	ErrPluginTimeout = errors.New("plugin call exceeded its deadline")
	// ErrPluginConfirmationRequired identifies a destructive operation that needs explicit confirmation.
	ErrPluginConfirmationRequired = errors.New("confirmation required; pass --yes to proceed")
	// ErrPluginUnlockedInCI refuses automatic installation of an unlocked plugin in CI.
	ErrPluginUnlockedInCI = errors.New("plugin is declared but not locked; automatic installation is disabled in CI")
	// ErrPluginFlagConflict identifies flags that cannot be combined.
	ErrPluginFlagConflict = errors.New("plugin flags cannot be combined")
	// ErrConfigurationImportConflict identifies two definitions of one component in a stack: two imported
	// provider units with the same name, or a unit and a YAML component.
	ErrConfigurationImportConflict = errors.New("provider import conflicts with another component definition")
	// ErrConfigurationImportEmpty identifies a provider import that resolved to no components.
	ErrConfigurationImportEmpty = errors.New("provider import matched no components")
	// ErrComponentHookFailed identifies a provider-supplied hook that exited unsuccessfully or could not run.
	ErrComponentHookFailed = errors.New("component hook failed")
	// ErrComponentVarFileMissing identifies a required variable file that does not exist.
	ErrComponentVarFileMissing = errors.New("required variable file does not exist")
	// ErrComponentHookWorkingDirectory identifies a hook whose working directory does not exist.
	ErrComponentHookWorkingDirectory = errors.New("hook working directory does not exist")

	ErrInvalidCredentialCodec    = errors.New("invalid credential codec")
	ErrDuplicateCredentialCodec  = errors.New("duplicate credential codec")
	ErrInvalidAuthRegistration   = errors.New("invalid auth registration")
	ErrDuplicateAuthRegistration = errors.New("duplicate auth registration")
	// ErrDuplicateCommandRegistration prevents extension code from replacing built-in commands.
	ErrDuplicateCommandRegistration = errors.New("command provider already registered")
	// ErrDuplicateManifestRegistration prevents changing the contract of a registered manifest kind.
	ErrDuplicateManifestRegistration = errors.New("manifest kind already registered with a different contract")
)
