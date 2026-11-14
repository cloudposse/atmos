package engine

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/gruntwork-io/terragrunt/pkg/atmosbridge"
	"github.com/gruntwork-io/terragrunt/pkg/config"
	"github.com/gruntwork-io/terragrunt/pkg/log"

	"github.com/cloudposse/atmos/plugins/terragrunt/internal/model"
)

// registryScheme prefixes Terraform module registry sources.
const registryScheme = "tfr://"

// registryClient is the HTTP client for registry requests. It is nil in production, which selects the
// default transport; tests substitute a client that trusts their registry.
var registryClient *http.Client

// prepareRemoteSource validates a remote module source and resolves a registry source to the getter URL
// that the Atmos source provisioner downloads. Resolution talks to the registry, so it only happens when
// the unit is prepared for a command: discovery and state identity keep the source as written.
func prepareRemoteSource(ctx context.Context, logger log.Logger, request *model.Request, cfg *config.TerragruntConfig, result *model.Resolution) error {
	if err := validateRemoteSource(result.Source); err != nil {
		return err
	}
	if !strings.HasPrefix(result.Source, registryScheme) || request.Phase == "backend" {
		return nil
	}
	constraint := ""
	if cfg.Terraform != nil && cfg.Terraform.Version != nil {
		constraint = *cfg.Terraform.Version
	}
	var auth *model.RegistryAuth
	started := time.Now()
	resolved, err := retryRegistryResolution(ctx, result.Errors, func() (string, error) {
		resolution, err := atmosbridge.ResolveRegistrySourceWithAuth(ctx, logger, atmosbridge.RegistryRequest{
			Client: registryClient, Source: result.Source, Constraint: constraint, Environment: request.Environment, OpenTofu: IsOpenTofu(request.Tofu),
		})
		if err == nil && resolution.Authorization != "" {
			auth = &model.RegistryAuth{Origin: resolution.AuthOrigin, Authorization: resolution.Authorization}
		}
		return resolution.Source, err
	})
	if err != nil {
		return registryProblem(result.Source, err)
	}
	result.RegistryResolutionNanos = uint64(time.Since(started))
	resolved = normalizeGetterURL(resolved)
	if err := validateRemoteSource(resolved); err != nil {
		return err
	}
	result.Source = resolved
	result.RegistryAuth = auth
	return nil
}

// retryRegistryResolution applies the unit's source-fetch retry patterns to
// registry discovery and version lookup, which happen before the host fetch.
func retryRegistryResolution(ctx context.Context, policies *model.Errors, resolve func() (string, error)) (string, error) {
	for attempt := 1; ; attempt++ {
		resolved, err := resolve()
		if err == nil || policies == nil || ctx.Err() != nil {
			return resolved, err
		}
		var retry *model.RetryPolicy
		for i := range policies.Retry {
			if registryErrorMatches(err.Error(), policies.Retry[i].Patterns) {
				retry = &policies.Retry[i]
				break
			}
		}
		if retry == nil || attempt >= retry.MaxAttempts {
			return "", err
		}
		timer := time.NewTimer(time.Duration(retry.SleepIntervalSec) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", ctx.Err()
		case <-timer.C:
		}
	}
}

func registryErrorMatches(message string, patterns []string) bool {
	matched := false
	for _, pattern := range patterns {
		negative := strings.HasPrefix(pattern, "!")
		expression, err := regexp.Compile(strings.TrimPrefix(pattern, "!"))
		if err == nil && expression.MatchString(message) {
			if negative {
				return false
			}
			matched = true
		}
	}
	return matched
}

// IsOpenTofu reports whether the selected executable is OpenTofu. It is the one rule of the host and the
// plugin: any executable whose name contains "tofu", in any case, is OpenTofu, whichever separator its
// path uses. Everything else is Terraform.
func IsOpenTofu(binary string) bool {
	name := binary
	if index := strings.LastIndexAny(name, `/\`); index >= 0 {
		name = name[index+1:]
	}
	return strings.Contains(strings.ToLower(name), "tofu")
}

// normalizeGetterURL spells the host shorthands a registry may return as the explicit Git URLs that
// validation and the source provisioner accept, for example github.com/org/repo?ref=v1.
func normalizeGetterURL(source string) string {
	for _, host := range []string{"github.com/", "bitbucket.org/"} {
		if !strings.HasPrefix(source, host) {
			continue
		}
		location, query, hasQuery := strings.Cut(source, "?")
		repository, subdir, hasSubdir := strings.Cut(location, "//")
		// Like go-getter, a shorthand names the repository with its first three path segments
		// (host/owner/name); anything after them is a subdirectory of that repository.
		if segments := strings.SplitN(repository, "/", 4); !hasSubdir && len(segments) == 4 {
			repository, subdir, hasSubdir = strings.Join(segments[:3], "/"), segments[3], true
		}
		if !strings.HasSuffix(repository, ".git") {
			repository += ".git"
		}
		normalized := "git::https://" + repository
		if hasSubdir {
			normalized += "//" + subdir
		}
		if hasQuery {
			normalized += "?" + query
		}
		return normalized
	}
	return source
}

// registryProblem describes a registry source that could not be resolved. The error text names the
// registry URL and status but never a credential, though a registry body can echo arbitrary text, so
// it is not marked safe.
func registryProblem(source string, err error) *model.Problem {
	return &model.Problem{
		Code: model.CodeInvalidSource, Summary: "The module registry could not resolve the `tfr://` source",
		Detail: fmt.Sprintf("Resolving %q failed: %s", source, err.Error()),
		Hints: []string{
			"Check the module path (`namespace/name/provider`), the `version`, and that the registry host is reachable.",
			"For a private registry, set `TF_TOKEN_<host>` (dots as underscores), add a `credentials` block to the CLI configuration, or set `TG_TF_REGISTRY_TOKEN`.",
		},
		Key: "terraform.source", Err: err,
	}
}
