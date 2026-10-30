package configuration

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
)

// RegisterImport records an explicit root without starting a plugin. The normal
// discovery pass evaluates the combined scope once after YAML imports are read.
func (s *Session) RegisterImport(ctx context.Context, uri string) (bool, error) {
	defer perf.Track(nil, "configuration.Session.RegisterImport")()

	name, path, found := strings.Cut(uri, "://")
	if !found {
		return false, nil
	}
	s.importMu.Lock()
	defer s.importMu.Unlock()
	request := s.importProviders[name]
	if request == nil {
		return false, nil
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	if err := s.ctx.Err(); err != nil {
		return true, err
	}
	if !filepath.IsLocal(path) || strings.ContainsAny(path, "\\?#*") {
		return true, fmt.Errorf("%w: provider import must name a project-relative file or directory", errUtils.ErrPluginInvalidConfig)
	}
	path = filepath.ToSlash(filepath.Clean(path))
	key := name + "://" + path
	if s.imports[key] {
		return true, nil
	}
	if s.started {
		return true, fmt.Errorf("%w: cannot add an import after configuration discovery", errUtils.ErrPluginInvalidConfig)
	}
	selected := s.discoveryRequest(request)
	selected.Imports = append(selected.Imports, key)
	s.imports[key] = true
	return true, nil
}

// discoveryRequest returns the pending discovery request for the template's provider,
// cloning the template into the session when none exists yet. The caller holds importMu.
func (s *Session) discoveryRequest(template *pluginv1.DiscoverComponentsRequest) *pluginv1.DiscoverComponentsRequest {
	var selected *pluginv1.DiscoverComponentsRequest
	for _, existing := range s.requests {
		if existing.Provider == template.Provider {
			selected = existing
		}
	}
	if selected == nil {
		selected = proto.Clone(template).(*pluginv1.DiscoverComponentsRequest)
		s.requests = append(s.requests, selected)
	}
	return selected
}
