package engine

import (
	"context"
	"errors"
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
	constraint := registryVersionConstraint(cfg)
	var auth *model.RegistryAuth
	started := time.Now()
	resolved, ignored, err := retryRegistryResolution(ctx, result.Errors, func() (string, error) {
		resolution, err := atmosbridge.ResolveRegistrySourceWithAuth(ctx, logger, atmosbridge.RegistryRequest{
			Client: registryClient, Source: result.Source, Constraint: constraint, Environment: request.Environment, OpenTofu: IsOpenTofu(request.Tofu),
		})
		if err == nil && resolution.Authorization != "" {
			auth = &model.RegistryAuth{Origin: resolution.AuthOrigin, Authorization: resolution.Authorization}
		}
		return resolution.Source, err
	})
	if ignored != nil {
		// No source was resolved, so the unit keeps the source it was written with and the host ends its
		// run without executing it, as it does for an ignored source fetch.
		result.RunIgnored = &model.IgnoredRun{Policy: ignored.Name, Message: ignored.Message}
		return nil
	}
	if err != nil {
		return registryProblem(result.Source, err)
	}
	if elapsed := time.Since(started); elapsed > 0 {
		result.RegistryResolutionNanos = uint64(elapsed)
	}
	original := result.Source
	resolved = normalizeGetterURL(resolved)
	if err := validateRemoteSource(resolved); err != nil {
		return rejectedResolvedSource(original, resolved, err)
	}
	result.Source = resolved
	result.RegistryAuth = auth
	return nil
}

// retryRegistryResolution applies the unit's source-fetch policies to registry discovery and version
// lookup, which happen before the host fetch. As for a source fetch, ignore policies are read before
// retry policies: a failure an ignore policy matches is not retried, and the matching policy is
// returned with the failure so the caller can end the unit's run.
func retryRegistryResolution(ctx context.Context, policies *model.Errors, resolve func() (string, error)) (string, *model.IgnorePolicy, error) {
	for attempt := 1; ; attempt++ {
		resolved, err := resolve()
		if err == nil || policies == nil || ctx.Err() != nil {
			return resolved, nil, err
		}
		text := cleanRegistryError(err.Error())
		if ignore := matchingRegistryIgnore(policies.Ignore, text); ignore != nil {
			return "", ignore, err
		}
		retry := matchingRegistryRetry(policies.Retry, text)
		if retry == nil || attempt >= retry.MaxAttempts {
			return "", nil, err
		}
		timer := time.NewTimer(time.Duration(retry.SleepIntervalSec) * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return "", nil, ctx.Err()
		case <-timer.C:
		}
	}
}

// registryErrorNoise matches everything that is not part of a readable error message. The host cleans
// the text of a failed source fetch the same way before matching, so one pattern behaves alike in both.
var registryErrorNoise = regexp.MustCompile(`[^a-zA-Z0-9./'"():=\- ]+`)

// cleanRegistryError reduces a failure to the readable text that ignore and retry patterns match. The
// cleaning is idempotent, so text that is already clean passes through unchanged.
func cleanRegistryError(message string) string {
	return strings.Join(strings.Fields(registryErrorNoise.ReplaceAllString(message, " ")), " ")
}

// matchingRegistryIgnore returns the first ignore policy, in declaration order, that matches the failure.
func matchingRegistryIgnore(policies []model.IgnorePolicy, message string) *model.IgnorePolicy {
	text := cleanRegistryError(message)
	for i := range policies {
		if registryErrorMatches(text, policies[i].Patterns) {
			return &policies[i]
		}
	}
	return nil
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
//
//nolint:lintroller // Standalone SDK-only plugin has no host perf runtime dependency.
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

// urlUserinfo matches the credentials in front of the host of a URL, also inside a getter-prefixed URL such as
// git::https://user:token@host/repo.
var urlUserinfo = regexp.MustCompile(`(://)[^/@\s]*@`)

// redactURLCredentials removes the userinfo from every URL in text, so a credential a registry embedded in a
// getter URL cannot reach a diagnostic that is shown to the user.
func redactURLCredentials(text string) string {
	return urlUserinfo.ReplaceAllString(text, "$1")
}

// rejectedResolvedSource names both the registry source a unit was written with and the getter URL the
// registry returned for it, so a rejected result can be traced to the source that produced it. The detail may be
// shown as safe, so credentials in the resolved URL are removed first.
func rejectedResolvedSource(original, resolved string, err error) error {
	var problem *model.Problem
	if !errors.As(err, &problem) {
		return err
	}
	annotated := *problem
	annotated.Detail = fmt.Sprintf("%s The registry source %q resolved to %q.", problem.Detail, original, redactURLCredentials(resolved))
	return &annotated
}

func registryVersionConstraint(cfg *config.TerragruntConfig) string {
	if cfg.Terraform != nil && cfg.Terraform.Version != nil {
		return *cfg.Terraform.Version
	}
	return ""
}

func matchingRegistryRetry(policies []model.RetryPolicy, text string) *model.RetryPolicy {
	for i := range policies {
		if registryErrorMatches(text, policies[i].Patterns) {
			return &policies[i]
		}
	}
	return nil
}
