package toolchain

import "github.com/cloudposse/atmos/toolchain/registry"

// Tool is a type alias for registry.Tool for backward compatibility.
// New code should import and use toolchain/registry.Tool directly.
type Tool = registry.Tool

// File is a type alias for registry.File for backward compatibility.
type File = registry.File

// Override is a type alias for registry.Override for backward compatibility.
type Override = registry.Override
