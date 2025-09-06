package cache

import (
	"time"
)

// CacheFile represents the root structure of the cache YAML file.
type CacheFile struct {
	Version     string            `yaml:"version"`
	Metadata    CacheMetadata     `yaml:"metadata"`
	Discovery   DiscoveryCache    `yaml:"discovery"`
	Performance *PerformanceCache `yaml:"performance,omitempty"`
	History     *HistoryCache     `yaml:"history,omitempty"`
	Preferences *PreferencesCache `yaml:"preferences,omitempty"`
	VCS         *VCSCache         `yaml:"vcs,omitempty"`
}

// CacheMetadata contains metadata about the cache file itself.
type CacheMetadata struct {
	LastUpdated    time.Time `yaml:"last_updated"`
	GotchaVersion  string    `yaml:"gotcha_version"`
	SchemaVersion  string    `yaml:"schema_version"`
}

// DiscoveryCache contains test discovery and counting information.
type DiscoveryCache struct {
	TestCounts     map[string]TestCountEntry  `yaml:"test_counts"`
	TestLists      map[string]TestListEntry   `yaml:"test_lists,omitempty"`
	PackageDetails map[string]PackageDetail   `yaml:"package_details,omitempty"`
}

// TestCountEntry represents cached test count for a specific pattern.
type TestCountEntry struct {
	Count           int       `yaml:"count"`
	Timestamp       time.Time `yaml:"timestamp"`
	GoModTime       time.Time `yaml:"go_mod_time"`
	PackagesScanned int       `yaml:"packages_scanned"`
}

// TestListEntry represents cached test names for a specific pattern.
type TestListEntry struct {
	Tests           []string  `yaml:"tests"`           // List of test names discovered
	Timestamp       time.Time `yaml:"timestamp"`
	GoModTime       time.Time `yaml:"go_mod_time"`
	PackagesScanned int       `yaml:"packages_scanned"`
}

// PackageDetail contains detailed information about a specific package.
type PackageDetail struct {
	TestCount    int       `yaml:"test_count"`
	LastModified time.Time `yaml:"last_modified"`
	FileHash     string    `yaml:"file_hash,omitempty"`
}

// PerformanceCache contains performance metrics and analysis.
type PerformanceCache struct {
	SlowestTests    []TestPerformance    `yaml:"slowest_tests,omitempty"`
	SlowestPackages []PackagePerformance `yaml:"slowest_packages,omitempty"`
}

// TestPerformance tracks performance of individual tests.
type TestPerformance struct {
	Name          string    `yaml:"name"`
	Package       string    `yaml:"package"`
	AvgDurationMs int64     `yaml:"avg_duration_ms"`
	LastRun       time.Time `yaml:"last_run"`
}

// PackagePerformance tracks performance of packages.
type PackagePerformance struct {
	Package       string `yaml:"package"`
	AvgDurationMs int64  `yaml:"avg_duration_ms"`
	TestCount     int    `yaml:"test_count"`
}

// HistoryCache contains execution history.
type HistoryCache struct {
	Runs       []RunHistory `yaml:"runs"`
	MaxEntries int          `yaml:"max_entries"`
}

// RunHistory represents a single test run.
type RunHistory struct {
	ID          string    `yaml:"id"`
	Timestamp   time.Time `yaml:"timestamp"`
	Pattern     string    `yaml:"pattern"`
	Total       int       `yaml:"total"`
	Passed      int       `yaml:"passed"`
	Failed      int       `yaml:"failed"`
	Skipped     int       `yaml:"skipped"`
	DurationMs  int64     `yaml:"duration_ms"`
	Flags       []string  `yaml:"flags,omitempty"`
}

// PreferencesCache stores user preferences.
type PreferencesCache struct {
	DefaultTimeout    string `yaml:"default_timeout,omitempty"`
	DefaultShowFilter string `yaml:"default_show_filter,omitempty"`
	PreferredVerbosity string `yaml:"preferred_verbosity,omitempty"`
}

// VCSCache stores VCS integration metadata.
type VCSCache struct {
	GitHub *GitHubCache `yaml:"github,omitempty"`
}

// GitHubCache stores GitHub-specific cache data.
type GitHubCache struct {
	LastPRCommentID string `yaml:"last_pr_comment_id,omitempty"`
	LastPRNumber    int    `yaml:"last_pr_number,omitempty"`
	LastCommentUUID string `yaml:"last_comment_uuid,omitempty"`
}