package tui

import (
	"bufio"
	"io"
	"os"
	"sync"
	"time"

	"github.com/charmbracelet/bubbles/progress"
	"github.com/charmbracelet/bubbles/spinner"
)

// SubtestStats tracks statistics for subtests of a parent test.
type SubtestStats struct {
	passed  []string // names of passed subtests
	failed  []string // names of failed subtests
	skipped []string // names of skipped subtests
}

// PackageResult stores complete information about a tested package.
type PackageResult struct {
	Package   string
	StartTime time.Time
	EndTime   time.Time
	Status    string // "pass", "fail", "skip", "running"
	Tests     map[string]*TestResult
	TestOrder []string // Maintain test execution order
	Coverage  string   // e.g., "coverage: 75.2% of statements"
	Output    []string // Package-level output (build errors, etc.)
	Elapsed   float64
	HasTests  bool
}

// TestResult stores individual test information.
type TestResult struct {
	Name         string
	FullName     string // Full test name including package
	Status       string // "pass", "fail", "skip", "running"
	Elapsed      float64
	Output       []string // All output lines from this test
	Parent       string   // Parent test name for subtests
	Subtests     map[string]*TestResult
	SubtestOrder []string
	SkipReason   string // Reason for skipping the test
}

// TestModel represents the Bubble Tea model for the interactive TUI.
type TestModel struct {
	// Test execution
	testPackages []string
	testArgs     string
	testCmd      *os.Process // Store the process for killing on abort
	testProc     io.ReadCloser
	scanner      *bufio.Scanner

	// Buffered output
	buffers           map[string][]string
	subtestStats      map[string]*SubtestStats  // Track subtest statistics per parent test
	packageResults    map[string]*PackageResult // Complete package results
	packageOrder      []string                  // Order packages were started
	activePackages    map[string]bool           // Currently running packages
	displayedPackages map[string]bool           // Packages that have been displayed

	// Current state
	currentPackage string
	currentTest    string
	showFilter     string // "all", "failed", "passed", "skipped", "collapsed", "none"
	verbosityLevel string // "minimal", "standard", "with-output", "verbose"
	testFilter     string // Test filter applied via -run flag (if any)

	// Progress tracking
	spinner          spinner.Model
	progress         progress.Model
	progressMessages []string
	estimatedTotal   int // Estimated total test count (from cache or discovery)
	processedTests   int // Number of tests processed so far

	// Test counting
	actualTestCount    int  // Actual test count discovered during execution
	completedTests     int  // Number of tests completed so far
	totalTests         int  // Total tests (actual or estimated)
	estimatedTestCount int  // Original estimate from cache
	usingEstimate      bool // Whether we're using estimate or actual count
	passCount          int  // Number of passed tests
	failCount          int  // Number of failed tests
	skipCount          int  // Number of skipped tests

	// UI state
	width        int
	height       int
	scrollOffset int    // For scrolling through results
	maxScroll    int    // Maximum scroll position
	outputBuffer string // Buffer for gradual output display

	// Statistics
	passed  int
	failed  int
	skipped int

	// Timing
	startTime time.Time
	endTime   time.Time

	// File output
	outputFile   string
	coverProfile string

	// Process state
	done       bool
	err        error
	aborted    bool // Track if the test was aborted
	exitCode   int  // Store the exit code from the test process
	alert      bool // Whether to emit terminal bell on completion
	jsonFile   io.WriteCloser
	jsonWriter *sync.Mutex

	// Legacy compatibility fields
	packagesWithNoTests   map[string]bool // Track packages that have no test files
	packageHasTests       map[string]bool // Track if package had any test run events
	packageNoTestsPrinted map[string]bool // Track if we already printed "No tests" for a package
}
