package toolchain

import (
	_ "github.com/cloudposse/atmos/toolchain/registry/atmos" // Import for init() registration.
)

// Blank import ensures atmos package init() runs to register the inline registry factory.
