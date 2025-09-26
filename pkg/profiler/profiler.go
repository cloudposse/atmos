package profiler

import (
	"context"
	"fmt"
	"net/http"
	_ "net/http/pprof"
	"sync"
	"time"

	"github.com/charmbracelet/log"
)

// Config holds the configuration for the profiler.
type Config struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	Port    int    `json:"port" yaml:"port"`
	Host    string `json:"host" yaml:"host"`
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
	config Config
	server *http.Server
	mu     sync.Mutex
	ctx    context.Context
	cancel context.CancelFunc
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

// Start starts the profiling server if enabled.
func (p *Server) Start() error {
	if !p.config.Enabled {
		log.Debug("Profiler is disabled")
		return nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

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
		log.Info("Starting pprof server", "address", addr)
		log.Info("pprof endpoints available at:", "url", fmt.Sprintf("http://%s/debug/pprof/", addr))

		if err := p.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Error("pprof server error", "error", err)
		}
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	log.Debug("pprof server started successfully")
	return nil
}

// Stop stops the profiling server.
func (p *Server) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.server == nil {
		return nil
	}

	log.Debug("Stopping pprof server")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := p.server.Shutdown(ctx); err != nil {
		log.Error("Error shutting down pprof server", "error", err)
		return err
	}

	p.server = nil
	p.cancel()

	log.Debug("pprof server stopped")
	return nil
}

// IsRunning returns true if the profiler server is currently running.
func (p *Server) IsRunning() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.server != nil
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
