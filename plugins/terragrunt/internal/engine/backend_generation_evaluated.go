package engine

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// errUnverifiedGeneration marks a generate declaration whose backend identity the static check could not
// prove, so a state read may still verify it by evaluating the declaration.
var errUnverifiedGeneration = errors.New("generate declaration not statically verified")

// verifyBackendGeneration proves, for a state read, that no generate declaration of the unit or its
// includes writes a backend. Literal declarations are checked without evaluation. When one is dynamic,
// such as a parent's generate block that interpolates a local, the unit is evaluated once more in the
// state read's read-only environment, with inputs projected away and every dependency replaced by an
// unknown placeholder, and each rendered file is checked. Anything that cannot be rendered there (a
// command, a secret, a dependency output) or that renders a backend still fails closed. Automatic
// includes keep failing closed: the evaluation does not merge them the way a run does.
func (e *unitEvaluator) verifyBackendGeneration(ctx context.Context, environment *atmosbridge.Venv, paths []string) error {
	err := checkBackendGeneration(environment.FS, paths)
	if err == nil || !errors.Is(err, errUnverifiedGeneration) {
		return err
	}
	expanded, expandErr := withAutomaticIncludes(environment.FS, paths)
	if expandErr != nil || len(expanded) != len(paths) {
		return err
	}
	if !evaluatedGenerationDeclaresNoBackend(ctx, e.logger, environment, e.pctx) {
		return err
	}
	return nil
}

// evaluatedGenerationDeclaresNoBackend renders every enabled generate declaration of the unit and reports
// whether all of them were rendered and none declares a backend or cloud block.
func evaluatedGenerationDeclaresNoBackend(ctx context.Context, logger log.Logger, environment *atmosbridge.Venv, pctx *config.ParsingContext) bool {
	placeholder := cty.DynamicVal
	probe := pctx.WithSkipOutputsResolution()
	probe.DecodedDependencies = &placeholder
	// Fresh caches keep the projected files apart from the partial parse's cached ones.
	full, err := config.ParseConfigFile(config.WithCaches(ctx), logger, environment, probe, pctx.TerragruntConfigPath, nil)
	if err != nil {
		return false
	}
	for name := range full.GenerateConfigs {
		generated := full.GenerateConfigs[name]
		if generated.Disable {
			// A disabled declaration writes nothing; at most its policy removes an earlier file.
			continue
		}
		path := filepath.Clean(generated.Path)
		if !filepath.IsLocal(path) || path == "." {
			return false
		}
		content, err := atmosbridge.RenderGenerated(ctx, logger, &generated)
		if err != nil || !provablyWithoutBackend(path, content) {
			return false
		}
	}
	return true
}

// provablyWithoutBackend reports whether a rendered file certainly declares no backend or cloud block. A
// Terraform file that does not parse cannot be proven either way, so it fails closed.
func provablyWithoutBackend(path string, content []byte) bool {
	if !terraformConfigurationFile(path) {
		return true
	}
	if strings.HasSuffix(strings.ToLower(path), ".json") {
		return json.Valid(content) && !jsonDeclaresBackend(content)
	}
	if _, diagnostics := hclsyntax.ParseConfig(content, path, hcl.InitialPos); diagnostics.HasErrors() {
		return false
	}
	return !hclDeclaresBackend(content, path)
}
