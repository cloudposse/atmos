package security

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindingsCache_GetSetFindings(t *testing.T) {
	tests := []struct {
		name      string
		setOpts   *QueryOptions
		getOpts   *QueryOptions
		findings  []Finding
		wantHit   bool
		wantCount int
	}{
		{
			name: "cache hit with matching options",
			setOpts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityCritical, SeverityHigh},
				Source:      SourceSecurityHub,
				MaxFindings: 50,
			},
			getOpts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityCritical, SeverityHigh},
				Source:      SourceSecurityHub,
				MaxFindings: 50,
			},
			findings: []Finding{
				{ID: "f-1", Severity: SeverityCritical},
				{ID: "f-2", Severity: SeverityHigh},
			},
			wantHit:   true,
			wantCount: 2,
		},
		{
			name: "cache miss with different region",
			setOpts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityHigh},
				MaxFindings: 50,
			},
			getOpts: &QueryOptions{
				Region:      "us-west-2",
				Severity:    []Severity{SeverityHigh},
				MaxFindings: 50,
			},
			findings:  []Finding{{ID: "f-1"}},
			wantHit:   false,
			wantCount: 0,
		},
		{
			name: "cache miss with different severity",
			setOpts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityHigh},
				MaxFindings: 50,
			},
			getOpts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityCritical},
				MaxFindings: 50,
			},
			findings:  []Finding{{ID: "f-1"}},
			wantHit:   false,
			wantCount: 0,
		},
		{
			name: "cache miss with different max findings",
			setOpts: &QueryOptions{
				Region:      "us-east-1",
				MaxFindings: 50,
			},
			getOpts: &QueryOptions{
				Region:      "us-east-1",
				MaxFindings: 100,
			},
			findings:  []Finding{{ID: "f-1"}},
			wantHit:   false,
			wantCount: 0,
		},
		{
			name: "severity order does not affect cache key",
			setOpts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityHigh, SeverityCritical},
				MaxFindings: 50,
			},
			getOpts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityCritical, SeverityHigh},
				MaxFindings: 50,
			},
			findings:  []Finding{{ID: "f-1"}},
			wantHit:   true,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewFindingsCache()

			cache.SetFindings(tt.setOpts, tt.findings)

			got, hit := cache.GetFindings(tt.getOpts)
			assert.Equal(t, tt.wantHit, hit)
			if tt.wantHit {
				require.NotNil(t, got)
				assert.Len(t, got, tt.wantCount)
			}
		})
	}
}

func TestFindingsCache_GetSetCompliance(t *testing.T) {
	tests := []struct {
		name         string
		setFramework string
		setStack     string
		getFramework string
		getStack     string
		wantHit      bool
	}{
		{
			name:         "cache hit with matching framework and stack",
			setFramework: "cis-aws",
			setStack:     "prod-us-east-1",
			getFramework: "cis-aws",
			getStack:     "prod-us-east-1",
			wantHit:      true,
		},
		{
			name:         "cache miss with different framework",
			setFramework: "cis-aws",
			setStack:     "prod-us-east-1",
			getFramework: "pci-dss",
			getStack:     "prod-us-east-1",
			wantHit:      false,
		},
		{
			name:         "cache miss with different stack",
			setFramework: "cis-aws",
			setStack:     "prod-us-east-1",
			getFramework: "cis-aws",
			getStack:     "staging-us-east-1",
			wantHit:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cache := NewFindingsCache()
			report := &ComplianceReport{
				Framework:       tt.setFramework,
				Stack:           tt.setStack,
				FailingControls: 3,
			}

			cache.SetCompliance(tt.setFramework, tt.setStack, report)

			got, hit := cache.GetCompliance(tt.getFramework, tt.getStack)
			assert.Equal(t, tt.wantHit, hit)
			if tt.wantHit {
				require.NotNil(t, got)
				assert.Equal(t, tt.setFramework, got.Framework)
			}
		})
	}
}

func TestFindingsCache_TTLExpiration(t *testing.T) {
	// Use a very short TTL for testing expiration.
	cache := NewFindingsCache(WithCacheTTL(50 * time.Millisecond))

	opts := &QueryOptions{
		Region:      "us-east-1",
		MaxFindings: 10,
	}
	findings := []Finding{{ID: "f-1"}}

	cache.SetFindings(opts, findings)

	// Immediately should be a hit.
	got, hit := cache.GetFindings(opts)
	assert.True(t, hit)
	assert.Len(t, got, 1)

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	// Should now be a miss.
	got, hit = cache.GetFindings(opts)
	assert.False(t, hit)
	assert.Nil(t, got)
}

func TestFindingsCache_ComplianceTTLExpiration(t *testing.T) {
	cache := NewFindingsCache(WithCacheTTL(50 * time.Millisecond))

	report := &ComplianceReport{Framework: "cis-aws", Stack: "prod"}
	cache.SetCompliance("cis-aws", "prod", report)

	// Immediately should be a hit.
	got, hit := cache.GetCompliance("cis-aws", "prod")
	assert.True(t, hit)
	assert.NotNil(t, got)

	// Wait for TTL to expire.
	time.Sleep(100 * time.Millisecond)

	got, hit = cache.GetCompliance("cis-aws", "prod")
	assert.False(t, hit)
	assert.Nil(t, got)
}

func TestFindingsCache_Invalidate(t *testing.T) {
	cache := NewFindingsCache()

	opts := &QueryOptions{Region: "us-east-1", MaxFindings: 10}
	cache.SetFindings(opts, []Finding{{ID: "f-1"}})
	cache.SetCompliance("cis-aws", "prod", &ComplianceReport{Framework: "cis-aws"})

	// Verify entries exist.
	_, hit := cache.GetFindings(opts)
	assert.True(t, hit)
	_, hit = cache.GetCompliance("cis-aws", "prod")
	assert.True(t, hit)

	// Invalidate all.
	cache.Invalidate()

	// Verify entries are gone.
	_, hit = cache.GetFindings(opts)
	assert.False(t, hit)
	_, hit = cache.GetCompliance("cis-aws", "prod")
	assert.False(t, hit)
}

func TestFindingsCache_ConcurrentAccess(t *testing.T) {
	cache := NewFindingsCache()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent writers.
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			opts := &QueryOptions{
				Region:      "us-east-1",
				MaxFindings: idx,
			}
			cache.SetFindings(opts, []Finding{{ID: "f-concurrent"}})
		}(i)
	}

	// Concurrent readers.
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			opts := &QueryOptions{
				Region:      "us-east-1",
				MaxFindings: idx,
			}
			// We don't assert on hit/miss since writers and readers race.
			// The goal is to verify no panics or data races occur.
			cache.GetFindings(opts)
		}(i)
	}

	wg.Wait()

	// If we got here without a panic or race detector failure, the test passes.
}

func TestFindingsCache_ConcurrentComplianceAccess(t *testing.T) {
	cache := NewFindingsCache()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Concurrent compliance writers.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			cache.SetCompliance("cis-aws", "prod", &ComplianceReport{Framework: "cis-aws"})
		}()
	}

	// Concurrent compliance readers.
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			cache.GetCompliance("cis-aws", "prod")
		}()
	}

	wg.Wait()
}

func TestBuildFindingsKey(t *testing.T) {
	tests := []struct {
		name    string
		opts    *QueryOptions
		wantKey string
	}{
		{
			name: "full options",
			opts: &QueryOptions{
				Region:      "us-east-1",
				Severity:    []Severity{SeverityCritical, SeverityHigh},
				Source:      SourceSecurityHub,
				MaxFindings: 50,
			},
			wantKey: "findings:us-east-1:CRITICAL,HIGH:security-hub:50",
		},
		{
			name:    "empty options",
			opts:    &QueryOptions{},
			wantKey: "findings::::0",
		},
		{
			name: "severity ordering is normalized",
			opts: &QueryOptions{
				Severity: []Severity{SeverityLow, SeverityCritical, SeverityHigh},
			},
			wantKey: "findings::CRITICAL,HIGH,LOW::0",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := buildFindingsKey(tt.opts)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}

func TestBuildComplianceKey(t *testing.T) {
	tests := []struct {
		name      string
		framework string
		stack     string
		wantKey   string
	}{
		{
			name:      "framework and stack",
			framework: "cis-aws",
			stack:     "prod-us-east-1",
			wantKey:   "cis-aws:prod-us-east-1",
		},
		{
			name:      "framework only",
			framework: "pci-dss",
			stack:     "",
			wantKey:   "pci-dss:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			key := buildComplianceKey(tt.framework, tt.stack)
			assert.Equal(t, tt.wantKey, key)
		})
	}
}

func TestWithCacheTTL(t *testing.T) {
	customTTL := 10 * time.Minute
	cache := NewFindingsCache(WithCacheTTL(customTTL))
	assert.Equal(t, customTTL, cache.ttl)
}
