package main

import (
	"fmt"
	"strings"
)

// googleCloudGoModule is the root module of the split cloud.google.com/go
// repository; each submodule (cloud.google.com/go/storage, .../iam, ...) is
// tagged independently, so its license URL needs the submodule's own path
// as both the tag prefix and the in-repo directory.
const googleCloudGoModule = "cloud.google.com/go"

// ModuleVersion is one row of `go list -m -f '{{.Path}} {{.Version}}' all`.
type ModuleVersion struct {
	Path    string
	Version string
}

// applyGoogleCloudGoOverrides rewrites the URL of every entry whose module
// is cloud.google.com/go or one of its submodules, using the generic
// google-cloud-go split-module URL shape (no network access -- the ref is
// derived from the resolved build-list version via listModules).
func applyGoogleCloudGoOverrides(entries []LicenseEntry, listModules func() ([]ModuleVersion, error)) error {
	modules, err := listModules()
	if err != nil {
		return err
	}

	index := make(map[string]int, len(entries))
	for i, e := range entries {
		index[e.Module] = i
	}

	for _, m := range modules {
		idx, ok := index[m.Path]
		if !ok {
			continue
		}
		ref := gitRefFromVersion(m.Version)
		subpath := strings.TrimPrefix(strings.TrimPrefix(m.Path, googleCloudGoModule), "/")

		var url string
		if subpath == "" {
			url = fmt.Sprintf("https://github.com/googleapis/google-cloud-go/blob/%s/LICENSE", ref)
		} else {
			url = fmt.Sprintf("https://github.com/googleapis/google-cloud-go/blob/%s/%s/%s/LICENSE", subpath, ref, subpath)
		}
		entries[idx].URL = url
	}
	return nil
}

// isGoogleCloudGoModule reports whether path is cloud.google.com/go itself
// or one of its submodules.
func isGoogleCloudGoModule(path string) bool {
	return path == googleCloudGoModule || strings.HasPrefix(path, googleCloudGoModule+"/")
}
