package profiler

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	_ "net/http/pprof" // #nosec G108 - Profiling endpoint is intentionally exposed for debugging
	"os"
	"runtime"
	"runtime/pprof"
	"runtime/trace"
	"strings"
	"sync"
	"time"

	log "github.com/charmbracelet/log"
)

// Profiler-specific errors.
var (
	// ErrUnsupportedProfileType is returned when an unsupported profile type is used.
	ErrUnsupportedProfileType = errors.New("unsupported profile type")
	// ErrStartCPUProfile is returned when CPU profiling fails to start.
	ErrStartCPUProfile = errors.New("failed to start CPU profile")
	// ErrStartTraceProfile is returned when trace profiling fails to start.
	ErrStartTraceProfile = errors.New("failed to start trace profile")
	// ErrCreateProfileFile is returned when profile file creation fails.
	ErrCreateProfileFile = errors.New("failed to create profile file")
)

// ProfileType represents the type of profile to collect.
type ProfileType string

const (
	// DefaultProfilerPort is the default port for the profiler server.
	DefaultProfilerPort = 6060
	// DefaultReadHeaderTimeout is the default timeout for reading request headers.
	DefaultReadHeaderTimeout = 10 * time.Second

	// ProfileTypeCPU collects CPU profile data.
	ProfileTypeCPU ProfileType = "cpu"
	// ProfileTypeHeap collects heap memory profile data.
	ProfileTypeHeap ProfileType = "heap"
	// ProfileTypeAllocs collects allocation profile data.
	ProfileTypeAllocs ProfileType = "allocs"
	// ProfileTypeGoroutine collects goroutine profile data.
	ProfileTypeGoroutine ProfileType = "goroutine"
	// ProfileTypeBlock collects blocking profile data.
	ProfileTypeBlock ProfileType = "block"
	// ProfileTypeMutex collects mutex contention profile data.
	ProfileTypeMutex ProfileType = "mutex"
	// ProfileTypeThreadCreate collects thread creation profile data.
	ProfileTypeThreadCreate ProfileType = "threadcreate"
	// ProfileTypeTrace collects execution trace data.
	ProfileTypeTrace ProfileType = "trace"
)

// Config holds the configuration for the profiler.
type Config struct {
	Enabled     bool        `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	Port        int         `json:"port" yaml:"port" mapstructure:"port"`
	Host        string      `json:"host" yaml:"host" mapstructure:"host"`
	File        string      `json:"file" yaml:"file" mapstructure:"file"`
	ProfileType ProfileType `json:"profile_type" yaml:"profile_type" mapstructure:"profile_type"`
}

// DefaultConfig returns the default profiler configuration.
func DefaultConfig() Config {
	return Config{
		Enabled:     false,
		Port:        DefaultProfilerPort,
		Host:        "localhost",
		ProfileType: ProfileTypeCPU,
	}
}

// Server represents a pprof profiling server.
type Server struct {
	config      Config
	server      *http.Server
	mu          sync.Mutex
	ctx         context.Context
	cancel      context.CancelFunc
	profFile    *os.File
	isFileBased bool
	profileType ProfileType
}

// New creates a new profiler server with the given configuration.
func New(config Config) *Server {
	ctx, cancel := context.WithCancel(context.Background())
	return &Server{
		config: config,
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start starts the profiling server or file-based profiling if enabled.
func (p *Server) Start() error {
	if !p.config.Enabled {
		log.Debug("Profiler is disabled")
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	// Check if file-based profiling is requested
	if p.config.File != "" {
		p.profileType = p.config.ProfileType
		if p.profileType == "" {
			p.profileType = ProfileTypeCPU
		}
		if !IsValidProfileType(p.profileType) {
			return fmt.Errorf("%w: %s. Supported types: %v",
				ErrUnsupportedProfileType, p.profileType, GetSupportedProfileTypes())
		}
		return p.startFileBasedProfiling()
	}

	// Start server-based profiling
	return p.startServerBasedProfiling()
}

// startFileBasedProfiling starts profiling to a file based on the profile type.
func (p *Server) startFileBasedProfiling() error {
	if p.profFile != nil {
		log.Warn("File-based profiler is already running")
		return nil
	}

	var err error
	p.profFile, err = os.Create(p.config.File)
	if err != nil {
		return errors.Join(ErrCreateProfileFile, fmt.Errorf("%s: %w", p.config.File, err))
	}

	if err := p.startProfileByType(); err != nil {
		return err
	}

	p.isFileBased = true
	log.Info("Profiling started", "type", p.profileType, "file", p.config.File)
	return nil
}

// startProfileByType starts profiling based on the configured profile type.
func (p *Server) startProfileByType() error {
	switch p.profileType {
	case ProfileTypeCPU:
		if err := pprof.StartCPUProfile(p.profFile); err != nil {
			p.profFile.Close()
			p.profFile = nil
			return fmt.Errorf("%w: %v", ErrStartCPUProfile, err)
		}
	case ProfileTypeTrace:
		if err := trace.Start(p.profFile); err != nil {
			p.profFile.Close()
			p.profFile = nil
			return fmt.Errorf("%w: %v", ErrStartTraceProfile, err)
		}
	case ProfileTypeHeap, ProfileTypeAllocs, ProfileTypeGoroutine,
		ProfileTypeBlock, ProfileTypeMutex, ProfileTypeThreadCreate:
		// These profiles are collected on-demand when stopping, so we just keep the file open
		// Enable runtime profiling for block and mutex if needed
		if p.profileType == ProfileTypeBlock {
			runtime.SetBlockProfileRate(1)
		}
		if p.profileType == ProfileTypeMutex {
			runtime.SetMutexProfileFraction(1)
		}
	default:
		p.profFile.Close()
		p.profFile = nil
		return fmt.Errorf("%w: %s", ErrUnsupportedProfileType, p.profileType)
	}
	return nil
}

// startServerBasedProfiling starts the HTTP profiling server.
func (p *Server) startServerBasedProfiling() error {
	if p.server != nil {
		log.Warn("Profiler server is already running")
		return nil
	}

	addr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	p.server = &http.Server{
		Addr:              addr,
		Handler:           http.DefaultServeMux,
		ReadHeaderTimeout: DefaultReadHeaderTimeout, // #nosec G112 - Prevent Slowloris attacks
	}

	// Channel to receive startup errors
	errChan := make(chan error, 1)

	go func() {
		if err := p.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("Profiler server error", "error", err)
			errChan <- err
		}
		close(errChan)
	}()

	// Give the server a moment to start and check for immediate errors
	select {
	case err := <-errChan:
		if err != nil {
			p.server = nil
			return fmt.Errorf("failed to start profiler server: %w", err)
		}
	case <-time.After(100 * time.Millisecond):
		// Server started successfully (no immediate error)
	}

	log.Debug("Profiler server available at:", "url", fmt.Sprintf("http://%s/debug/pprof/", addr))
	log.Debug("Profiler server started successfully")
	return nil
}

// Stop stops the profiling server or file-based profiling.
func (p *Server) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	// Stop file-based profiling if active
	if p.isFileBased && p.profFile != nil {
		return p.stopFileBasedProfiling()
	}

	// Stop server-based profiling if active
	if p.server != nil {
		return p.stopServerBasedProfiling()
	}

	return nil
}

// stopFileBasedProfiling stops profiling and writes/closes the file based on the profile type.
func (p *Server) stopFileBasedProfiling() error {
	writeErr := p.stopProfileAndWrite()

	if writeErr != nil {
		log.Error("Error writing profile data", "error", writeErr, "type", p.profileType, "file", p.config.File)
	}

	var closeErr error
	if err := p.profFile.Close(); err != nil {
		log.Error("Error closing profile file", "error", err, "file", p.config.File)
		closeErr = err
	}

	// Always reset profiler state, even on errors
	log.Info("Profiling completed", "type", p.profileType, "file", p.config.File)
	p.profFile = nil
	p.isFileBased = false
	p.cancel()

	// Return the first error encountered, prioritizing close errors over write errors
	if closeErr != nil {
		return closeErr
	}
	if writeErr != nil {
		return writeErr
	}
	return nil
}

// stopProfileAndWrite stops profiling and writes profile data based on the profile type.
func (p *Server) stopProfileAndWrite() error {
	switch p.profileType {
	case ProfileTypeCPU:
		pprof.StopCPUProfile()
		return nil
	case ProfileTypeTrace:
		trace.Stop()
		return nil
	case ProfileTypeHeap:
		return p.writeProfile("heap")
	case ProfileTypeAllocs:
		return p.writeProfile("allocs")
	case ProfileTypeGoroutine:
		return p.writeProfile("goroutine")
	case ProfileTypeBlock:
		err := p.writeProfile("block")
		runtime.SetBlockProfileRate(0) // Disable block profiling
		return err
	case ProfileTypeMutex:
		err := p.writeProfile("mutex")
		runtime.SetMutexProfileFraction(0) // Disable mutex profiling
		return err
	case ProfileTypeThreadCreate:
		return p.writeProfile("threadcreate")
	default:
		return fmt.Errorf("%w: %s", ErrUnsupportedProfileType, p.profileType)
	}
}

// writeProfile writes a profile to the file using pprof.Lookup.
func (p *Server) writeProfile(profileName string) error {
	if prof := pprof.Lookup(profileName); prof != nil {
		return prof.WriteTo(p.profFile, 0)
	}
	return nil
}

// stopServerBasedProfiling stops the HTTP profiling server.
func (p *Server) stopServerBasedProfiling() error {
	log.Debug("Stopping profiler server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		log.Error("Error shutting down profiler server", "error", err)
		return err
	}

	p.server = nil
	p.cancel()

	log.Debug("Profiler server stopped")
	return nil
}

// IsRunning returns true if the profiler (server or file-based) is currently running.
func (p *Server) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.server != nil || (p.isFileBased && p.profFile != nil)
}

// GetAddress returns the address the profiler server is listening on.
func (p *Server) GetAddress() string {
	if !p.config.Enabled {
		return ""
	}
	return fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
}

// GetURL returns the full URL to access the pprof web interface.
func (p *Server) GetURL() string {
	if !p.config.Enabled {
		return ""
	}
	return fmt.Sprintf("http://%s/debug/pprof/", p.GetAddress())
}

// IsValidProfileType checks if the given profile type is supported.
func IsValidProfileType(profileType ProfileType) bool {
	switch profileType {
	case ProfileTypeCPU, ProfileTypeHeap, ProfileTypeAllocs, ProfileTypeGoroutine,
		ProfileTypeBlock, ProfileTypeMutex, ProfileTypeThreadCreate, ProfileTypeTrace:
		return true
	default:
		return false
	}
}

// GetSupportedProfileTypes returns a list of all supported profile types.
func GetSupportedProfileTypes() []ProfileType {
	return []ProfileType{
		ProfileTypeCPU,
		ProfileTypeHeap,
		ProfileTypeAllocs,
		ProfileTypeGoroutine,
		ProfileTypeBlock,
		ProfileTypeMutex,
		ProfileTypeThreadCreate,
		ProfileTypeTrace,
	}
}

// ParseProfileType converts a string to ProfileType, with case-insensitive matching.
func ParseProfileType(s string) (ProfileType, error) {
	profileType := ProfileType(strings.ToLower(s))
	if IsValidProfileType(profileType) {
		return profileType, nil
	}
	return "", fmt.Errorf("%w: %s. Supported types: %v",
		ErrUnsupportedProfileType, s, GetSupportedProfileTypes())
}
