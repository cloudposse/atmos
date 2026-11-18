package artifact

import (
	"fmt"
	"sort"
	"sync"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

// ProducerRegistry maps artifact kinds to the [Producer] that prepares them. The generic files
// producer is registered by a composition root; kind-specific producers (scaffold, component, and
// so on) register the same way and add their own validation and exclusions.
type ProducerRegistry struct {
	mu        sync.RWMutex
	producers map[string]Producer
}

// NewProducerRegistry returns an empty registry.
func NewProducerRegistry() *ProducerRegistry {
	defer perf.Track(nil, "artifact.NewProducerRegistry")()

	return &ProducerRegistry{producers: make(map[string]Producer)}
}

// Register adds a producer under its Kind. A duplicate kind or an invalid kind is an error.
func (r *ProducerRegistry) Register(producer Producer) error {
	defer perf.Track(nil, "artifact.ProducerRegistry.Register")()

	if producer == nil {
		return fmt.Errorf("%w: producer must not be nil", errUtils.ErrArtifactConfigInvalid)
	}
	kind := producer.Kind()
	if err := ValidateKind(kind); err != nil {
		return fmt.Errorf("%w: producer kind %q: %w", errUtils.ErrArtifactConfigInvalid, kind, err)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.producers[kind]; exists {
		return fmt.Errorf("%w: producer for kind %q is already registered", errUtils.ErrArtifactConflict, kind)
	}
	r.producers[kind] = producer
	return nil
}

// Lookup returns the producer for kind; an empty kind means [KindFiles].
func (r *ProducerRegistry) Lookup(kind string) (Producer, bool) {
	defer perf.Track(nil, "artifact.ProducerRegistry.Lookup")()

	if kind == "" {
		kind = KindFiles
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	producer, ok := r.producers[kind]
	return producer, ok
}

// Kinds returns the registered kinds, sorted.
func (r *ProducerRegistry) Kinds() []string {
	defer perf.Track(nil, "artifact.ProducerRegistry.Kinds")()

	r.mu.RLock()
	defer r.mu.RUnlock()
	kinds := make([]string, 0, len(r.producers))
	for kind := range r.producers {
		kinds = append(kinds, kind)
	}
	sort.Strings(kinds)
	return kinds
}
