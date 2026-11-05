package sign

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"golang.org/x/mod/semver"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/dependencies"
	iolib "github.com/cloudposse/atmos/pkg/io"
	"github.com/cloudposse/atmos/pkg/oci/signature"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
	"github.com/cloudposse/atmos/pkg/toolchain"
)

// DefaultVersion pins the managed signer to a release supporting modern bundles.
const (
	DefaultVersion  = "3.1.3"
	cosignTool      = "sigstore/cosign"
	privateFileMode = 0o600
)

// Cosign uses a managed executable and the public Sigstore keyless signing flow.
// Resolve and Run are injectable subprocess boundaries for isolated tests.
type Cosign struct {
	Config  *schema.AtmosConfiguration
	Version string
	Yes     bool
	Resolve func(context.Context, *schema.AtmosConfiguration, string) (string, error)
	Run     func(context.Context, string, []string) error
}

// Sign creates a modern Sigstore bundle without giving Cosign registry credentials.
func (c Cosign) Sign(ctx context.Context, raw []byte) ([]byte, error) {
	defer perf.Track(c.Config, "pluginsign.Cosign.Sign")()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	binary, err := c.binary(ctx)
	if err != nil {
		return nil, err
	}
	directory, err := os.MkdirTemp("", "atmos-plugin-sign-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(directory)
	payload, output := filepath.Join(directory, "index.json"), filepath.Join(directory, "signature.sigstore.json")
	if err := os.WriteFile(payload, raw, privateFileMode); err != nil {
		return nil, err
	}
	args := []string{"sign-blob", "--new-bundle-format=true", "--bundle", output}
	if c.Yes {
		args = append(args, "--yes")
	}
	args = append(args, payload)
	run := c.Run
	if run == nil {
		run = runCosign
	}
	if err := run(ctx, binary, args); err != nil {
		return nil, fmt.Errorf("%w: cosign signing failed: %w", errUtils.ErrPluginSignatureInvalid, err)
	}
	return readBundle(output)
}

func (c Cosign) binary(ctx context.Context) (string, error) {
	version := c.Version
	if version == "" {
		version = DefaultVersion
	}
	version = strings.TrimPrefix(version, "v")
	if !semver.IsValid("v"+version) || semver.Major("v"+version) != "v3" {
		return "", fmt.Errorf("%w: cosign-version must be an exact Cosign 3 version", errUtils.ErrPluginInvalidConfig)
	}
	resolve := c.Resolve
	if resolve == nil {
		resolve = resolveCosign
	}
	return resolve(ctx, c.Config, version)
}

func resolveCosign(ctx context.Context, config *schema.AtmosConfiguration, version string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	allowed, err := dependencies.InstallsAutomatically(config)
	if err != nil {
		return "", err
	}
	if allowed {
		if err := dependencies.NewInstaller(config).EnsureTools(map[string]string{cosignTool: version}); err != nil {
			return "", err
		}
	}
	name := "cosign"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary, err := toolchain.NewInstaller().FindBinaryPath("sigstore", "cosign", version, name)
	if err != nil {
		return "", errUtils.Build(errUtils.ErrToolNotInstalled).WithCause(err).WithHintf("Run `atmos toolchain install %s@%s` or enable automatic toolchain installation", cosignTool, version).Err()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	return filepath.Abs(binary)
}

func runCosign(ctx context.Context, binary string, args []string) error {
	// Executable is resolved from the managed toolchain; arguments are generated locally.
	command := exec.CommandContext(ctx, binary, args...)
	command.Stdin = iolib.GetContext().Input()
	// Cosign emits human signing progress, not the command's structured release output.
	command.Stdout, command.Stderr = iolib.GetContext().UI(), iolib.GetContext().UI()
	return command.Run()
}

func readBundle(path string) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, errors.Join(errUtils.ErrPluginSignatureInvalid, err)
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, signature.MaxBundleSize+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > signature.MaxBundleSize {
		return nil, errUtils.ErrPluginPayloadTooLarge
	}
	return raw, nil
}
