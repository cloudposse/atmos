// Package customcommand holds the argument and flag semantics of custom commands declared in
// atmos.yaml: how positional arguments are resolved against their defaults, how they travel from
// the pre-run hook to the executor, and which flag types are supported.
package customcommand

import (
	"encoding/json"
	"fmt"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ResolveArguments merges the positional values supplied on the command line with the declared
// arguments. The result has one entry per declared argument, in declaration order.
//
// A declared argument that was not supplied takes its default. An argument with no default that
// is not required resolves to an empty string; a required argument with no default is an error.
func ResolveArguments(declared []schema.CommandArgument, supplied []string) ([]string, error) {
	defer perf.Track(nil, "customcommand.ResolveArguments")()

	resolved := make([]string, len(declared))
	for i := range declared {
		arg := &declared[i]
		switch {
		case i < len(supplied):
			resolved[i] = supplied[i]
		case arg.Default != "":
			resolved[i] = arg.Default
		case arg.Required:
			return nil, errUtils.Build(errUtils.ErrCustomCommandArgumentMissing).
				WithExplanationf("Missing required argument '%s' with no default.", arg.Name).
				WithHintf("Pass a value for '%s', or give the argument a `default` or `required: false` in atmos.yaml.", arg.Name).
				WithContext("argument", arg.Name).
				Err()
		}
	}
	return resolved, nil
}

// EncodeArguments serializes resolved arguments losslessly so they can travel through a
// string-valued map (such as cobra command annotations) without being split on any character.
func EncodeArguments(args []string) (string, error) {
	defer perf.Track(nil, "customcommand.EncodeArguments")()

	if args == nil {
		args = []string{}
	}
	encoded, err := json.Marshal(args)
	if err != nil {
		return "", fmt.Errorf("%w: %w", errUtils.ErrFailedToProcessArgs, err)
	}
	return string(encoded), nil
}

// DecodeArguments is the inverse of EncodeArguments. It returns ok=false when encoded is empty,
// which means no arguments were ever recorded.
func DecodeArguments(encoded string) (args []string, ok bool, err error) {
	defer perf.Track(nil, "customcommand.DecodeArguments")()

	if strings.TrimSpace(encoded) == "" {
		return nil, false, nil
	}
	if err := json.Unmarshal([]byte(encoded), &args); err != nil {
		return nil, false, fmt.Errorf("%w: %w", errUtils.ErrFailedToProcessArgs, err)
	}
	return args, true, nil
}
