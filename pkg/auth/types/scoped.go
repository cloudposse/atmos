package types

import (
	"context"

	"github.com/cloudposse/atmos/pkg/schema"
)

//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_scoped.go -package=$GOPACKAGE

// ScopedAuthManager optionally gives a store ownership of an isolated identity
// for one operation. Identity cleanup completes before this method returns;
// provider sessions and the component's authentication context are preserved.
type ScopedAuthManager interface {
	WithIsolatedIdentity(context.Context, string, func(context.Context, *schema.AuthContext) error) error
}
