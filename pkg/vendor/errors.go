package vendor

import "errors"

// Vendor command errors.
var (
	ErrVendorConfigNotExist       = errors.New("the '--everything' flag is set, but vendor config file does not exist")
	ErrExecuteVendorDiffCmd       = errors.New("'atmos vendor diff' is not implemented yet")
	ErrValidateComponentFlag      = errors.New("either '--component' or '--tags' flag can be provided, but not both")
	ErrValidateComponentStackFlag = errors.New("either '--component' or '--stack' flag can be provided, but not both")
	ErrValidateEverythingFlag     = errors.New("'--everything' flag cannot be combined with '--component', '--stack', or '--tags' flags")
	ErrMissingComponent           = errors.New("to vendor a component, the '--component' (shorthand '-c') flag needs to be specified.\n" +
		"Example: atmos vendor pull -c <component>")
)

// Vendor config errors.
var (
	ErrVendorComponents              = errors.New("failed to vendor components")
	ErrSourceMissing                 = errors.New("'source' must be specified in 'sources' in the vendor config file")
	ErrTargetsMissing                = errors.New("'targets' must be specified for the source in the vendor config file")
	ErrVendorConfigSelfImport        = errors.New("vendor config file imports itself in 'spec.imports'")
	ErrMissingVendorConfigDefinition = errors.New("either 'spec.sources' or 'spec.imports' (or both) must be defined in the vendor config file")
	// ErrVendoringNotConfigured is deprecated - use errUtils.ErrVendoringNotConfigured instead.
	// Kept for backwards compatibility.
	ErrVendoringNotConfigured   = errors.New("Vendoring is not configured")
	ErrPermissionDenied         = errors.New("permission denied when accessing")
	ErrEmptySources             = errors.New("'spec.sources' is empty in the vendor config file and the imports")
	ErrNoComponentsWithTags     = errors.New("there are no components in the vendor config file")
	ErrNoYAMLConfigFiles        = errors.New("no YAML configuration files found in directory")
	ErrDuplicateComponents      = errors.New("duplicate component names")
	ErrDuplicateImport          = errors.New("duplicate import")
	ErrDuplicateComponentsFound = errors.New("duplicate component")
	ErrComponentNotDefined      = errors.New("the flag '--component' is passed, but the component is not defined in any of the 'sources' in the vendor config file and the imports")
)

// Component vendor errors.
var (
	ErrMissingMixinURI             = errors.New("'uri' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrMissingMixinFilename        = errors.New("'filename' must be specified for each 'mixin' in the 'component.yaml' file")
	ErrMixinEmpty                  = errors.New("mixin URI cannot be empty")
	ErrMixinNotImplemented         = errors.New("local mixin installation not implemented")
	ErrStackPullNotSupported       = errors.New("command 'atmos vendor pull --stack <stack>' is not supported yet")
	ErrComponentConfigFileNotFound = errors.New("component vendoring config file does not exist in the folder")
	ErrFolderNotFound              = errors.New("folder does not exist")
	ErrInvalidComponentKind        = errors.New("invalid 'kind' in the component vendoring config file. Supported kinds: 'ComponentVendorConfig'")
	ErrUriMustSpecified            = errors.New("'uri' must be specified in 'source.uri' in the component vendoring config file")
)
