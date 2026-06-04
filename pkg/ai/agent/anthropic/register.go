package anthropic

import (
	"context"

	"github.com/cloudposse/atmos/pkg/ai/registry"
	"github.com/cloudposse/atmos/pkg/schema"
)

func init() {
	registry.Register(ProviderName, func(_ context.Context, atmosConfig *schema.AtmosConfiguration) (registry.Client, error) {
		return NewSimpleClient(atmosConfig)
	})
}
