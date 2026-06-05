package providers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/schema"
)

// sopsProvider implements the Provider interface over a SOPS-encrypted file (track 2). It
// shells out to the `sops` binary because SOPS has no stable public encrypt API in Go; the
// binary is the canonical way to mutate an encrypted file in place.
type sopsProvider struct {
	name string
	kind string
	file string
}

// newSopsProvider builds a SOPS provider. The provider definition is resolved from the
// stack/component `secrets.providers` map first (so providers can be declared in a stack),
// then from the top-level `secrets.providers` in atmos.yaml.
func newSopsProvider(atmosConfig *schema.AtmosConfiguration, name string, sectionProviders map[string]any) (Provider, error) {
	defer perf.Track(atmosConfig, "providers.newSopsProvider")()

	kind, spec, ok := lookupSopsProvider(sectionProviders, name)
	if !ok {
		cfg, found := atmosConfig.Secrets.Providers[name]
		if !found {
			return nil, fmt.Errorf("%w: %q", ErrProviderNotFound, name)
		}
		kind = cfg.Kind
		spec = cfg.Spec
	}

	file, _ := spec["file"].(string)
	if file == "" {
		return nil, fmt.Errorf("%w: provider %q has no `spec.file`", ErrProviderNotFound, name)
	}

	return &sopsProvider{name: name, kind: kind, file: file}, nil
}

// lookupSopsProvider reads a provider definition from a stack/component `secrets.providers`
// map: `{ providers: { <name>: { kind: ..., spec: { ... } } } }`.
func lookupSopsProvider(sectionProviders map[string]any, name string) (kind string, spec map[string]any, ok bool) {
	if sectionProviders == nil {
		return "", nil, false
	}
	raw, found := sectionProviders[name].(map[string]any)
	if !found {
		return "", nil, false
	}
	kind, _ = raw["kind"].(string)
	spec, _ = raw["spec"].(map[string]any)
	if spec == nil {
		spec = map[string]any{}
	}
	return kind, spec, true
}

func (p *sopsProvider) Kind() string {
	defer perf.Track(nil, "providers.sopsProvider.Kind")()

	return p.kind
}

// resolveFile substitutes {stack}/{component} tokens in the configured file path.
func (p *sopsProvider) resolveFile(coord Coordinate) string {
	f := strings.ReplaceAll(p.file, "{stack}", coord.Stack)
	f = strings.ReplaceAll(f, "{component}", coord.Component)
	return f
}

// sopsIndexPath builds a SOPS index path for a single top-level key, e.g. `["NAME"]`.
func sopsIndexPath(key string) string {
	return fmt.Sprintf("[%q]", key)
}

func (p *sopsProvider) Set(coord Coordinate, value any) error {
	defer perf.Track(nil, "providers.sopsProvider.Set")()

	encoded, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrSerialize, err)
	}
	_, err = p.runSops("set", p.resolveFile(coord), sopsIndexPath(coord.Key), string(encoded))
	return err
}

func (p *sopsProvider) Get(coord Coordinate) (any, error) {
	defer perf.Track(nil, "providers.sopsProvider.Get")()

	out, err := p.runSops("decrypt", "--extract", sopsIndexPath(coord.Key), p.resolveFile(coord))
	if err != nil {
		return nil, err
	}
	trimmed := strings.TrimRight(out, "\n")

	// Prefer structured decoding; fall back to the raw string for plain scalars.
	var decoded any
	if jsonErr := json.Unmarshal([]byte(trimmed), &decoded); jsonErr == nil {
		return decoded, nil
	}
	return trimmed, nil
}

func (p *sopsProvider) Delete(coord Coordinate) error {
	defer perf.Track(nil, "providers.sopsProvider.Delete")()

	_, err := p.runSops("unset", p.resolveFile(coord), sopsIndexPath(coord.Key))
	return err
}

func (p *sopsProvider) Status(coord Coordinate) (bool, error) {
	defer perf.Track(nil, "providers.sopsProvider.Status")()

	_, err := p.Get(coord)
	if err != nil {
		// Treat extraction failure as "not initialized" rather than a hard error.
		return false, nil
	}
	return true, nil
}

// runSops invokes the sops binary with the given arguments and returns stdout.
func (p *sopsProvider) runSops(args ...string) (string, error) {
	bin, err := exec.LookPath("sops")
	if err != nil {
		return "", ErrSopsBinaryNotFound
	}

	cmd := exec.Command(bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	log.Debug("Running sops", "args", strings.Join(args, " "))

	if runErr := cmd.Run(); runErr != nil {
		return "", fmt.Errorf("%w: %w: %s", ErrSopsOperation, runErr, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}
