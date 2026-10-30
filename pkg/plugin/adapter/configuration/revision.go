package configuration

import (
	"path/filepath"

	"google.golang.org/protobuf/proto"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
	pluginv1 "github.com/cloudposse/atmos/pkg/plugin/proto/atmos/plugin/v1"
	"github.com/cloudposse/atmos/pkg/schema"
)

// ForRevision creates an independent inventory in a different checkout. The host's
// trusted plugin selection, discovery scope, environment, and cancellation remain
// fixed for the invocation; no cached definitions or evaluated HCL are reused.
func (s *Session) ForRevision(root string) (schema.ComponentConfigurationRuntime, error) {
	defer perf.Track(nil, "configurationadapter.ForRevision")()

	if err := s.ctx.Err(); err != nil {
		return nil, err
	}
	if !filepath.IsAbs(root) {
		return nil, errUtils.ErrPluginInvalidConfig
	}
	s.importMu.Lock()
	defer s.importMu.Unlock()
	revision := &Session{ctx: s.ctx, root: root, command: s.command, loader: s.loader, done: make(chan struct{}), imports: make(map[string]bool), importProviders: make(map[string]*pluginv1.DiscoverComponentsRequest)}
	// Explicit imports come from the revision's YAML, not the HEAD inventory.
	for scheme, request := range s.importProviders {
		cloned := proto.Clone(request).(*pluginv1.DiscoverComponentsRequest)
		cloned.RootDirectory = root
		revision.importProviders[scheme] = cloned
	}
	for _, request := range s.requests {
		if len(request.IncludePaths) == 0 {
			continue
		}
		cloned := proto.Clone(request).(*pluginv1.DiscoverComponentsRequest)
		cloned.RootDirectory = root
		cloned.Imports = nil
		revision.requests = append(revision.requests, cloned)
	}
	return revision, nil
}
