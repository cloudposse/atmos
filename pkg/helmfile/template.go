package helmfile

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/cloudposse/atmos/pkg/manifest"
	"github.com/cloudposse/atmos/pkg/perf"
	"github.com/cloudposse/atmos/pkg/provisioner/target"
	"github.com/cloudposse/atmos/pkg/schema"
)

// targetFlag is the Atmos-specific flag used to select a provision target for
// `atmos helmfile template`. It is intercepted by Atmos and never forwarded to
// the helmfile binary.
const targetFlag = "--target"

// ExtractTargetFlag removes the Atmos `--target[=value]` flag from a helmfile
// argument list and returns the selected target plus the remaining args. Both
// `--target X` and `--target=X` forms are supported.
func ExtractTargetFlag(args []string) (string, []string) {
	defer perf.Track(nil, "helmfile.ExtractTargetFlag")()

	out := make([]string, 0, len(args))
	value := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == targetFlag:
			if i+1 < len(args) {
				value = args[i+1]
				i++
			}
		case strings.HasPrefix(arg, targetFlag+"="):
			value = strings.TrimPrefix(arg, targetFlag+"=")
		default:
			out = append(out, arg)
		}
	}
	return value, out
}

// RenderDeliverInput carries everything needed to render a helmfile to manifests
// and deliver them to a provision target.
type RenderDeliverInput struct {
	AtmosConfig      *schema.AtmosConfiguration
	Info             *schema.ConfigAndStacksInfo
	Command          string         // Resolved helmfile binary.
	Args             []string       // Full helmfile args (including the `template` subcommand).
	WorkingDir       string         // Component working directory.
	EnvVars          []string       // Environment for the helmfile process.
	ProvisionSection map[string]any // The component's `provision:` section.
	FlagTarget       string         // The selected `--target` value (may be empty).
	EnvProvider      target.IdentityEnvironmentProvider
}

// RenderAndDeliver runs `helmfile template`, captures the rendered manifests, and
// delivers them to the selected provision target. When the selected target is the
// implicit cluster (no external target), the rendered manifests are written to
// stdout instead.
func RenderAndDeliver(ctx context.Context, in *RenderDeliverInput) (string, error) {
	defer perf.Track(in.AtmosConfig, "helmfile.RenderAndDeliver")()

	selected, err := target.SelectTarget(in.ProvisionSection, in.FlagTarget)
	if err != nil {
		return "", err
	}

	rendered, err := captureCommand(ctx, in)
	if err != nil {
		return rendered, err
	}

	// Cluster (implicit) target: helmfile template has no cluster delivery, so
	// emit the rendered manifests to stdout (equivalent to `helmfile template`).
	if selected.Kind == target.KindKubernetes {
		_, err := os.Stdout.WriteString(rendered)
		return rendered, err
	}

	objects, err := manifest.DecodeObjects([]byte(rendered))
	if err != nil {
		return rendered, err
	}
	files, err := manifest.ArtifactFiles(objects)
	if err != nil {
		return rendered, err
	}

	artifact := target.ProvisionArtifact{
		Kind:   target.ArtifactKindKubernetesManifests,
		Format: target.FormatYAML,
		Files:  files,
		Metadata: target.ArtifactMetadata{
			Component: in.Info.ComponentFromArg,
			Stack:     in.Info.Stack,
			Target:    selected.Name,
		},
	}

	return rendered, target.Deliver(ctx, selected.Kind, &target.DeliverInput{
		AtmosConfig:  in.AtmosConfig,
		TargetName:   selected.Name,
		TargetConfig: selected.Config,
		Artifact:     artifact,
		EnvProvider:  in.EnvProvider,
	})
}

// captureCommand runs the helmfile command and returns its stdout.
func captureCommand(ctx context.Context, in *RenderDeliverInput) (string, error) {
	cmd := exec.CommandContext(ctx, in.Command, in.Args...) //nolint:gosec // Command is a resolved toolchain binary.
	cmd.Dir = in.WorkingDir
	cmd.Env = append(os.Environ(), in.EnvVars...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("helmfile template failed: %w: %s", err, stderr.String())
	}
	return stdout.String(), nil
}
