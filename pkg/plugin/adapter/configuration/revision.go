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
	revision := &Session{ctx: s.ctx, root: root, command: s.command, loader: s.loader, done: make(chan struct{})}
	for _, request := range s.requests {
		cloned := proto.Clone(request).(*pluginv1.DiscoverComponentsRequest)
		cloned.RootDirectory = root
		revision.requests = append(revision.requests, cloned)
	}
	return revision, nil
}
