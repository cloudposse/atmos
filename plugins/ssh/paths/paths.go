// Package paths resolves the per-user locations the SSH plugin uses on disk.
//
// The plugin process starts with an empty environment, so nothing here reads environment
// variables. The home directory comes from the operating system's user database. The default root
// mirrors where Atmos puts its XDG data directory when no XDG variable is set (see pkg/xdg), with
// an "ssh" subfolder: ~/.local/share/atmos/ssh on macOS and Linux, and
// ~\AppData\Local\atmos\ssh on Windows.
package paths

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"runtime"
)

// Sentinel errors. Wrap them with fmt.Errorf("%w ...") and test with errors.Is.
var (
	// ErrNoHome means the user's home directory could not be determined.
	ErrNoHome = errors.New("cannot determine the home directory")
	// ErrEmptyRoot means an empty root directory was given.
	ErrEmptyRoot = errors.New("root directory is empty")
	// ErrInvalidIdentity means an identity name cannot be used in a file name.
	ErrInvalidIdentity = errors.New("invalid identity name")
	// ErrSocketPathTooLong means a Unix socket path exceeds the operating system's limit.
	ErrSocketPathTooLong = errors.New("unix socket path is too long")
)

const (
	// DirMode is the permission of every directory the plugin creates.
	DirMode os.FileMode = 0o700

	maxIdentityLen = 64
	socketExt      = ".sock"
)

var identityRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// Paths is the set of per-user locations under one root directory.
type Paths struct {
	root string
}

// New returns Paths rooted at root. Tests use it to keep everything under a temporary directory.
func New(root string) (*Paths, error) {
	if root == "" {
		return nil, ErrEmptyRoot
	}
	return &Paths{root: filepath.Clean(root)}, nil
}

// Default returns Paths rooted at the default location for the current user, without reading any
// environment variable.
func Default() (*Paths, error) {
	home, err := HomeDir()
	if err != nil {
		return nil, err
	}
	return New(defaultRoot(home, runtime.GOOS))
}

// HomeDir returns the current user's home directory from the user database. It does not use
// $HOME, which is absent in the plugin process.
func HomeDir() (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", fmt.Errorf("%w: %w", ErrNoHome, err)
	}
	if u.HomeDir == "" {
		return "", fmt.Errorf("%w: user %q has no home directory", ErrNoHome, u.Username)
	}
	return u.HomeDir, nil
}

// defaultRoot is the Atmos data directory for goos, plus the plugin's "ssh" subfolder.
func defaultRoot(home, goos string) string {
	if goos == "windows" {
		return filepath.Join(home, "AppData", "Local", "atmos", "ssh")
	}
	return filepath.Join(home, ".local", "share", "atmos", "ssh")
}

// Root is the directory that holds everything the plugin stores.
func (p *Paths) Root() string { return p.root }

// KeysDir holds the private key material and its metadata.
func (p *Paths) KeysDir() string { return filepath.Join(p.root, "keys") }

// StoreDir is where the key store of the given kind keeps its key material: a subdirectory of
// KeysDir named after the kind, so stores never share file names.
func (p *Paths) StoreDir(kind string) string { return filepath.Join(p.KeysDir(), kind) }

// SocketsDir holds one agent socket per identity.
func (p *Paths) SocketsDir() string { return filepath.Join(p.root, "sockets") }

// ReceiptsDir holds the enrollment receipts.
func (p *Paths) ReceiptsDir() string { return filepath.Join(p.root, "receipts") }

// BinDir holds the stable, versioned copies of the plugin executable.
func (p *Paths) BinDir() string { return filepath.Join(p.root, "bin") }

// AgentConfigFile is the file that lists the identities the agent serves.
func (p *Paths) AgentConfigFile() string { return filepath.Join(p.root, "agent.json") }

// AuditLogFile is the agent's audit log.
func (p *Paths) AuditLogFile() string { return filepath.Join(p.root, "audit.log") }

// StateFile is the file the running daemon writes its pid and version to.
func (p *Paths) StateFile() string { return filepath.Join(p.root, "daemon.json") }

// ServiceLogFile receives the daemon's standard output and error under launchd or systemd.
func (p *Paths) ServiceLogFile() string { return filepath.Join(p.root, "agent.log") }

// ValidateIdentity reports whether name can be used as an identity name in a file name.
func ValidateIdentity(name string) error {
	if len(name) > maxIdentityLen || !identityRE.MatchString(name) {
		return fmt.Errorf("%w: %q", ErrInvalidIdentity, name)
	}
	return nil
}

// SocketPath returns the socket path for identity. It fails if the name is unsafe or the whole
// path is longer than the operating system allows for a Unix socket.
func (p *Paths) SocketPath(identity string) (string, error) {
	if err := ValidateIdentity(identity); err != nil {
		return "", err
	}
	sock := filepath.Join(p.SocketsDir(), identity+socketExt)
	limit := maxSocketPath(runtime.GOOS)
	if len(sock) >= limit {
		return "", fmt.Errorf("%w: %d bytes, the limit on %s is %d (including the terminating NUL): %s; "+
			"use a shorter data directory", ErrSocketPathTooLong, len(sock), runtime.GOOS, limit, sock)
	}
	return sock, nil
}

// maxSocketPath is the size of sockaddr_un.sun_path on goos. A path must be strictly shorter,
// leaving room for the terminating NUL.
func maxSocketPath(goos string) int {
	switch goos {
	case "linux", "android", "windows":
		return 108
	default:
		return 104
	}
}

// EnsureDirs creates the root and every subdirectory with mode 0700. It tightens the mode of
// directories that already exist.
func (p *Paths) EnsureDirs() error {
	for _, dir := range []string{p.root, p.KeysDir(), p.SocketsDir(), p.ReceiptsDir(), p.BinDir()} {
		if err := os.MkdirAll(dir, DirMode); err != nil {
			return fmt.Errorf("create directory %s: %w", dir, err)
		}
		if err := os.Chmod(dir, DirMode); err != nil {
			return fmt.Errorf("restrict directory %s: %w", dir, err)
		}
	}
	return nil
}
