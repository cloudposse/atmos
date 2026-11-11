package types

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

// ScopedAuthManager optionally gives a store ownership of an isolated identity
// for one operation. Identity cleanup completes before this method returns;
// provider sessions and the component's authentication context are preserved.
type ScopedAuthManager interface {
	WithIsolatedIdentity(context.Context, string, func(context.Context, *schema.AuthContext) error) error
}
