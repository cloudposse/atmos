package security

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/cloudposse/atmos/pkg/perf"
)

// defaultCacheTTL is the default time-to-live for cached entries.
const defaultCacheTTL = 5 * time.Minute

// cacheEntry holds a cached value along with its expiration time.
type cacheEntry[T any] struct {
	value     T
	expiresAt time.Time
}

// isExpired returns true if the entry has passed its expiration time.
func (e *cacheEntry[T]) isExpired() bool {
	return time.Now().After(e.expiresAt)
}

// findingsCache provides a thread-safe TTL cache for security findings and compliance reports.
// It reduces redundant AWS API calls when the same query is repeated within the TTL window.
type findingsCache struct {
	mu         sync.RWMutex
	ttl        time.Duration
	findings   map[string]*cacheEntry[[]Finding]
	compliance map[string]*cacheEntry[*ComplianceReport]
}

// FindingsCacheOption is a functional option for configuring the findings cache.
type FindingsCacheOption func(*findingsCache)

// WithCacheTTL sets a custom TTL for cache entries.
func WithCacheTTL(ttl time.Duration) FindingsCacheOption {
	return func(c *findingsCache) {
		c.ttl = ttl
	}
}

// NewFindingsCache creates a new findings cache with the given options.
func NewFindingsCache(opts ...FindingsCacheOption) *findingsCache {
	defer perf.Track(nil, "security.NewFindingsCache")()

	c := &findingsCache{
		ttl:        defaultCacheTTL,
		findings:   make(map[string]*cacheEntry[[]Finding]),
		compliance: make(map[string]*cacheEntry[*ComplianceReport]),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// GetFindings retrieves cached findings for the given query options.
// Returns the findings and true on a cache hit, or nil and false on a miss.
func (c *findingsCache) GetFindings(opts *QueryOptions) ([]Finding, bool) {
	defer perf.Track(nil, "security.findingsCache.GetFindings")()

	key := buildFindingsKey(opts)

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.findings[key]
	if !ok || entry.isExpired() {
		return nil, false
	}
	// Return a copy to prevent callers from mutating cached state.
	result := make([]Finding, len(entry.value))
	copy(result, entry.value)
	return result, true
}

// SetFindings stores findings in the cache for the given query options.
func (c *findingsCache) SetFindings(opts *QueryOptions, findings []Finding) {
	defer perf.Track(nil, "security.findingsCache.SetFindings")()

	key := buildFindingsKey(opts)

	// Store a copy to prevent callers from mutating cached state.
	stored := make([]Finding, len(findings))
	copy(stored, findings)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.findings[key] = &cacheEntry[[]Finding]{
		value:     stored,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// GetCompliance retrieves a cached compliance report for the given framework and stack.
// Returns the report and true on a cache hit, or nil and false on a miss.
func (c *findingsCache) GetCompliance(framework, stack string) (*ComplianceReport, bool) {
	defer perf.Track(nil, "security.findingsCache.GetCompliance")()

	key := buildComplianceKey(framework, stack)

	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.compliance[key]
	if !ok || entry.isExpired() {
		return nil, false
	}
	return entry.value, true
}

// SetCompliance stores a compliance report in the cache for the given framework and stack.
func (c *findingsCache) SetCompliance(framework, stack string, report *ComplianceReport) {
	defer perf.Track(nil, "security.findingsCache.SetCompliance")()

	key := buildComplianceKey(framework, stack)

	c.mu.Lock()
	defer c.mu.Unlock()

	c.compliance[key] = &cacheEntry[*ComplianceReport]{
		value:     report,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// Invalidate removes all entries from the cache.
func (c *findingsCache) Invalidate() {
	defer perf.Track(nil, "security.findingsCache.Invalidate")()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.findings = make(map[string]*cacheEntry[[]Finding])
	c.compliance = make(map[string]*cacheEntry[*ComplianceReport])
}

// buildFindingsKey constructs a composite cache key from all query dimensions.
func buildFindingsKey(opts *QueryOptions) string {
	if opts == nil {
		return "findings:<nil>"
	}

	// Sort severities for consistent key generation.
	sevs := make([]string, len(opts.Severity))
	for i, s := range opts.Severity {
		sevs[i] = string(s)
	}
	sort.Strings(sevs)

	return fmt.Sprintf("findings:%s:%s:%s:%s:%s:%s:%d",
		opts.Region,
		strings.Join(sevs, ","),
		string(opts.Source),
		opts.Framework,
		opts.Stack,
		opts.Component,
		opts.MaxFindings,
	)
}

// buildComplianceKey constructs a cache key for compliance reports.
func buildComplianceKey(framework, stack string) string {
	return fmt.Sprintf("%s:%s", framework, stack)
}
