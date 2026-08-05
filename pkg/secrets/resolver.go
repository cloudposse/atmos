package secrets

import (
	"fmt"
	"strings"

	fnparser "github.com/cloudposse/atmos/pkg/function/parser"
	"github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/secrets/providers"
	"github.com/cloudposse/atmos/pkg/utils"
)

// secretTag is the YAML tag (with leading bang) for the secret function.
const secretTag = "!secret"

// Resolve resolves a `!secret NAME [| path ...] [| raw] [| default ...]` expression to a value.
//
// Behavior (in order):
//  1. It validates that the secret is declared in the component section.
//  2. If the processing scope is an inspection command with masking enabled
//     (stackInfo.SecretsMaskOnly), it returns the mask replacement WITHOUT retrieving the
//     value from the backend — no provider call, no credentials required.
//  3. Otherwise it resolves the backend
//     provider, retrieves the value, applies the optional path/default modifiers, registers
//     the value (recursively) with the I/O masker, and returns it.
func Resolve(atmosConfig *schema.AtmosConfiguration, input, currentStack string, stackInfo *schema.ConfigAndStacksInfo) (any, error) {
	defer perf.Track(atmosConfig, "secrets.Resolve")()

	name, opts, err := parseSecretArgs(input)
	if err != nil {
		return nil, err
	}

	component := componentName(stackInfo)

	var componentSection map[string]any
	if stackInfo != nil {
		componentSection = stackInfo.ComponentSection
	}

	decl, ok := LookupDeclaration(componentSection, name)
	if !ok {
		return nil, fmt.Errorf("%w: %q (declare it under the component's secrets.vars)", ErrSecretNotDeclared, name)
	}

	// Mask-without-retrieval fast path for inspection commands. Declaration lookup happens
	// first so masked inspection still catches misspelled or malformed secret references.
	if stackInfo != nil && stackInfo.SecretsMaskOnly {
		return io.GetContext().Masker().Replacement(), nil
	}

	provider, err := providerFor(atmosConfig, &decl, componentSection)
	if err != nil {
		return nil, fmt.Errorf("%w (secret %q)", err, name)
	}

	coord := coordinateForDeclaration(&decl, currentStack, component)
	return retrieveAndMask(atmosConfig, provider, coord, name, opts)
}

// retrieveAndMask gets the value from the provider, applies path/default modifiers, registers
// it with the masker, and returns it.
func retrieveAndMask(atmosConfig *schema.AtmosConfiguration, provider providers.Provider, coord providers.Coordinate, name string, opts ResolveOptions) (any, error) {
	defer perf.Track(atmosConfig, "secrets.retrieveAndMask")()

	var value any
	var err error
	if opts.Raw {
		if rawProvider, ok := provider.(providers.RawGetter); ok {
			value, err = rawProvider.GetRaw(coord)
		} else {
			value, err = provider.Get(coord)
			if err == nil {
				if _, ok := value.(string); !ok {
					err = providers.ErrRawNotSupported
				}
			}
		}
	} else {
		value, err = provider.Get(coord)
	}
	if err != nil {
		if opts.Default != nil {
			return *opts.Default, nil
		}
		return nil, fmt.Errorf("%w: %q: %w", ErrSecretMissing, name, err)
	}

	if opts.Path != "" {
		value, err = utils.EvaluateYqExpression(atmosConfig, value, opts.Path)
		if err != nil {
			return nil, err
		}
	}

	// Register every secret-bearing representation so all output is masked.
	io.RegisterSecretValue(value)

	return value, nil
}

// componentName returns the component name for the current resolution scope.
func componentName(stackInfo *schema.ConfigAndStacksInfo) string {
	if stackInfo == nil {
		return ""
	}
	if stackInfo.Component != "" {
		return stackInfo.Component
	}
	if stackInfo.ComponentFromArg != "" {
		return stackInfo.ComponentFromArg
	}
	return stackInfo.FinalComponent
}

// parseSecretArgs parses `!secret NAME [| path "x"] [| raw] [| default "y"]` (or the same
// without the leading tag) into the secret name and modifiers.
func parseSecretArgs(input string) (string, ResolveOptions, error) {
	defer perf.Track(nil, "secrets.parseSecretArgs")()

	s := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(input), secretTag))
	parsed, err := fnparser.ParseSecret(s)
	if err != nil {
		if strings.TrimSpace(s) == "" {
			return "", ResolveOptions{}, ErrEmptyName
		}
		return "", ResolveOptions{}, fmt.Errorf("%w: %w", ErrInvalidSecretArgs, err)
	}
	return parsed.Name, ResolveOptions{Path: parsed.Path, Raw: parsed.Raw, Default: parsed.Default}, nil
}
