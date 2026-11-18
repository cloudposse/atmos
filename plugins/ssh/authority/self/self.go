// Package self is the authority for keys that nobody vouches for. Nothing is called: the receipt
// only records that the key was used without an upstream.
package self

import (
	"context"

	"github.com/cloudposse/atmos/plugins/ssh/authority"
)

// Name is the registry key of the self authority.
const Name = "self"

// Factory builds the self authority for a registry.
var Factory = authority.Factory{
	Name: Name,
	New:  func(authority.Options) (authority.Authority, error) { return New(), nil },
}

// Authority is the no-authority case.
type Authority struct{}

// New returns the self authority.
func New() *Authority { return &Authority{} }

// Name implements authority.Authority.
func (*Authority) Name() string { return Name }

// Supports implements authority.Authority. Self-asserted keys can be used for anything.
func (*Authority) Supports() []authority.Use {
	return []authority.Use{authority.UseAuthentication, authority.UseSigning}
}

// Ensure implements authority.Authority. It makes no call and ignores the credentials.
func (a *Authority) Ensure(_ context.Context, _ map[string]string, request authority.Request) (authority.Receipt, error) {
	if err := authority.ValidateRequest(a, request); err != nil {
		return authority.Receipt{}, err
	}
	receipt := authority.NewReceipt(Name, request)
	receipt.Uses = authority.NormalizeUses(request.Uses)
	return receipt, nil
}

// Revoke implements authority.Authority. There is nothing to withdraw.
func (*Authority) Revoke(context.Context, map[string]string, authority.Receipt) error { return nil }
