package config

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/opencontainers/go-digest"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/artifact"
	"github.com/cloudposse/atmos/pkg/schema"
)

// validateCatalog checks the optional catalog reference. A catalog belongs directly to its
// repository: it must be an oci:// reference with a tag or digest (there is no implicit latest),
// carry no credentials, and name the repository's own registry host, because the repository's
// identity must never authenticate a different host.
func validateCatalog(desc *artifact.KindDescriptor, repo *schema.ArtifactRepositoryConfig) error {
	if repo.Catalog == "" {
		return nil
	}
	if !desc.AllowsCatalog {
		return fmt.Errorf("%w: kind %q does not support a catalog; direct named operations and managed-record enumeration work without one", errUtils.ErrArtifactConfigInvalid, desc.Kind)
	}
	parsed, err := parseCatalogURI(repo.Catalog)
	if err != nil {
		return err
	}
	registry, _ := repo.Options["registry"].(string)
	if registry != "" && !strings.EqualFold(registry, parsed.Host) {
		return fmt.Errorf("%w: catalog host %q does not match the repository registry %q; a catalog cannot direct the repository identity to another host", errUtils.ErrArtifactConfigInvalid, parsed.Host, registry)
	}
	return nil
}

// parseCatalogURI parses and checks the shape of an oci:// catalog reference.
func parseCatalogURI(raw string) (*url.URL, error) {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "oci" || parsed.Host == "" {
		return nil, fmt.Errorf("%w: catalog %q must be an oci://<registry>/<path>:<tag> reference", errUtils.ErrArtifactConfigInvalid, sanitizedCatalog(raw))
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("%w: catalog %q must not contain credentials, a query, or a fragment", errUtils.ErrArtifactConfigInvalid, sanitizedCatalog(raw))
	}
	if err := validateCatalogReference(strings.TrimPrefix(parsed.Path, "/")); err != nil {
		return nil, fmt.Errorf("%w: catalog %q: %w", errUtils.ErrArtifactConfigInvalid, raw, err)
	}
	return parsed, nil
}

// validateCatalogReference checks "<repository path>:<tag>" or "<repository path>@sha256:<hex>".
func validateCatalogReference(reference string) error {
	path, tag, dgst := reference, "", ""
	if before, after, found := strings.Cut(reference, "@"); found {
		path, dgst = before, after
	} else if index := strings.LastIndex(reference, ":"); index >= 0 {
		path, tag = reference[:index], reference[index+1:]
	}
	if tag == "" && dgst == "" {
		return fmt.Errorf("%w: a tag or digest is required (there is no implicit latest)", errUtils.ErrArtifactRefInvalid)
	}
	if err := artifact.ValidateName(path); err != nil {
		return err
	}
	if dgst != "" {
		return artifact.ValidateDigest(digest.Digest(dgst))
	}
	return artifact.ValidateVersion(tag)
}

// sanitizedCatalog hides userinfo so a mistaken credential is not echoed back in the error.
func sanitizedCatalog(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.User == nil {
		return raw
	}
	parsed.User = nil
	return parsed.String()
}
