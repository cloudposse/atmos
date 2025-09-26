package profiler

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"os"
	"runtime/pprof"
	"sync"
	"time"

	log "github.com/charmbracelet/log"
)

// Config holds the configuration for the profiler.
type Config struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Port    int    `json:"port" yaml:"port"`
	Host    string `json:"host" yaml:"host"`
	File    string `json:"file" yaml:"file"`
}

// DefaultConfig returns the default profiler configuration.
func DefaultConfig() Config {
	return Config{
		Enabled: false,
		Port:    6060,
		Host:    "localhost",
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
		return p.startFileBasedProfiling()
	}

	// Start server-based profiling
	return p.startServerBasedProfiling()
}

// startFileBasedProfiling starts CPU profiling to a file.
func (p *Server) startFileBasedProfiling() error {
	if p.profFile != nil {
		log.Warn("File-based profiler is already running")
		return nil
	}

	var err error
	p.profFile, err = os.Create(p.config.File)
	if err != nil {
		return fmt.Errorf("failed to create profile file %s: %w", p.config.File, err)
	}

	if err := pprof.StartCPUProfile(p.profFile); err != nil {
		p.profFile.Close()
		p.profFile = nil
		return fmt.Errorf("failed to start CPU profile: %w", err)
	}

	p.isFileBased = true
	log.Info("CPU profiling started", "file", p.config.File)
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
		Addr:    addr,
		Handler: http.DefaultServeMux,
	}

	go func() {
		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("Profiler server error", "error", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	log.Info("Profiler server available at:", "url", fmt.Sprintf("http://%s/debug/pprof/", addr))
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

// stopFileBasedProfiling stops CPU profiling and closes the file.
func (p *Server) stopFileBasedProfiling() error {
	pprof.StopCPUProfile()

	if err := p.profFile.Close(); err != nil {
		log.Error("Error closing profile file", "error", err, "file", p.config.File)
		return err
	}

	log.Info("CPU profiling completed", "file", p.config.File)
	p.profFile = nil
	p.isFileBased = false
	p.cancel()
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
