package registryauth

import (
	"context"
	"errors"
	"fmt"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/schema"
)

// Preparation registers already available auth plugins without downloading packages.
// The returned function removes those temporary registrations after authentication.
type Preparation func(context.Context) (func(), error)

// Option configures lazy authentication dependencies.
type Option func(*Resolver)

// WithPreparation supplies the plugin composition boundary without introducing a dependency cycle.
func WithPreparation(prepare Preparation) Option {
	return func(resolver *Resolver) { resolver.prepare = prepare }
}

func (r *Resolver) manager(ctx context.Context, selected *schema.AuthConfig) (Manager, func(), error) {
	manager, err := r.newManager(selected, r.config.CliConfigPath)
	if err == nil {
		return manager, func() {}, nil
	}
	if r.prepare == nil || (!errors.Is(err, errUtils.ErrInvalidIdentityKind) && !errors.Is(err, errUtils.ErrInvalidProviderKind)) {
		return nil, nil, bootstrapError(err)
	}
	cleanup, err := r.prepare(ctx)
	if err != nil {
		return nil, nil, err
	}
	manager, err = r.newManager(selected, r.config.CliConfigPath)
	if err != nil {
		cleanup()
		return nil, nil, bootstrapError(err)
	}
	return manager, cleanup, nil
}

func bootstrapError(err error) error {
	return fmt.Errorf("%w: registry authentication must be available before plugins are downloaded; use a built-in or already-installed auth identity: %w", errUtils.ErrIdentityAuthFailed, err)
}
