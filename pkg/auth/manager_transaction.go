package auth

import (
	"context"

	"github.com/cloudposse/atmos/pkg/auth/types"
)

func (m *manager) credentialTransaction(ctx context.Context, providerName string, operation func(context.Context) error) error {
	if transaction, ok := m.providers[providerName].(types.CredentialTransaction); ok {
		return transaction.WithCredentialTransaction(ctx, operation)
	}
	return operation(ctx)
}
