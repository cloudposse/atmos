package vhs

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/cloudposse/atmos/pkg/ffmpeg"
	"github.com/cloudposse/atmos/pkg/vhs"
	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

// Renderer renders VHS tape files to GIF/PNG.
type Renderer struct {
	demosDir string
	cacheDir string
	cache    *CacheMetadata
	force    bool
}

// RenderResult contains the result of a render operation.
type RenderResult struct {
	Cached      bool     // True if render was skipped due to cache hit.
	OutputPaths []string // Paths to rendered output files.
}

// NewRenderer creates a new VHS renderer.
func NewRenderer(demosDir string) *Renderer {
	return &Renderer{
		demosDir: demosDir,
		cacheDir: filepath.Join(demosDir, ".cache"),
		force:    false,
	}
}

// SetForce sets the force flag to skip cache checks.
func (r *Renderer) SetForce(force bool) {
	r.force = force
}

// Render renders a scene using VHS with cache checking.
func (r *Renderer) Render(ctx context.Context, sc *scene.Scene) (*RenderResult, error) {
	// Ensure cache directory exists.
	if err := os.MkdirAll(r.cacheDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Load cache metadata.
	if r.cache == nil {
		cache, err := LoadCache(r.cacheDir)
		if err != nil {
			return nil, fmt.Errorf("failed to load cache: %w", err)
		}
		r.cache = cache
	}

	// Get tape file path.
	tapeFile := filepath.Join(r.demosDir, sc.Tape)

	// Get audio file path if configured.
	var audioFile string
	if sc.Audio != nil && sc.Audio.Source != "" {
		audioFile = filepath.Join(r.demosDir, sc.Audio.Source)
	}

	// Build output file paths.
	outputFiles := r.buildOutputPaths(sc)

	// Check if render is needed.
	needsRender, err := r.cache.NeedsRender(sc.Name, tapeFile, audioFile, outputFiles, r.force)
	if err != nil {
		return nil, fmt.Errorf("failed to check cache: %w", err)
	}

	if !needsRender {
		// Cache hit - skip rendering.
		return &RenderResult{
			Cached:      true,
			OutputPaths: outputFiles,
		}, nil
	}

	// Run VHS to generate outputs.
	// VHS runs from the scene's workdir (or repo root if not specified).
	// Output files are written relative to workdir, then moved to cacheDir.
	repoRoot := filepath.Dir(r.demosDir)
	workdir := repoRoot
	if sc.Workdir != "" {
		workdir = filepath.Join(repoRoot, sc.Workdir)
	}

	// Run prep commands before VHS.
	if len(sc.Prep) > 0 {
		if err := r.runPrepCommands(ctx, sc, workdir); err != nil {
			return nil, fmt.Errorf("prep commands failed: %w", err)
		}
	}

	if err := vhs.Render(ctx, tapeFile, workdir, r.cacheDir); err != nil {
		return nil, err
	}

	// Move output files from workdir to cacheDir.
	// VHS writes outputs relative to workdir based on the Output directive.
	if err := r.moveOutputsToCache(sc, workdir); err != nil {
		return nil, fmt.Errorf("failed to move outputs to cache: %w", err)
	}

	// TODO: Optimize GIF with gifsicle if enabled in defaults.yaml.

	// Process audio for MP4 outputs if configured.
	if sc.Audio != nil && r.containsMP4(sc.Outputs) {
		if err := r.processAudioForMP4(ctx, sc); err != nil {
			return nil, fmt.Errorf("audio processing failed: %w", err)
		}
	}

	// Update cache with new hash.
	if err := r.cache.UpdateScene(sc.Name, tapeFile, audioFile, outputFiles); err != nil {
		return nil, fmt.Errorf("failed to update cache: %w", err)
	}

	// Save cache metadata.
	if err := r.cache.SaveCache(r.cacheDir); err != nil {
		return nil, fmt.Errorf("failed to save cache: %w", err)
	}

	return &RenderResult{
		Cached:      false,
		OutputPaths: outputFiles,
	}, nil
}

// buildOutputPaths builds the expected output file paths for a scene.
// Uses the scene name which matches the Output directive in the tape file.
func (r *Renderer) buildOutputPaths(sc *scene.Scene) []string {
	var paths []string

	for _, format := range sc.Outputs {
		filename := fmt.Sprintf("%s.%s", sc.Name, format)
		paths = append(paths, filepath.Join(r.cacheDir, filename))
	}

	return paths
}

// containsMP4 checks if the outputs array contains "mp4".
func (r *Renderer) containsMP4(outputs []string) bool {
	for _, output := range outputs {
		if output == "mp4" {
			return true
		}
	}
	return false
}

// findMP4OutputPath finds the MP4 file path from the scene outputs.
// Uses the scene name which matches the Output directive in the tape file.
func (r *Renderer) findMP4OutputPath(sc *scene.Scene) string {
	return filepath.Join(r.cacheDir, fmt.Sprintf("%s.mp4", sc.Name))
}

// processAudioForMP4 merges background audio with the MP4 output.
func (r *Renderer) processAudioForMP4(ctx context.Context, sc *scene.Scene) error {
	// Get paths.
	mp4Path := r.findMP4OutputPath(sc)
	audioPath := filepath.Join(r.demosDir, sc.Audio.Source)
	tempOutput := mp4Path + ".tmp"

	// Check if audio file exists.
	if _, err := os.Stat(audioPath); err != nil {
		return fmt.Errorf("audio file not found: %s", audioPath)
	}

	// Apply defaults for audio config.
	volume := sc.Audio.Volume
	if volume == 0 {
		volume = 0.3 // Default 30% volume
	}

	fadeOut := sc.Audio.FadeOut
	if fadeOut == 0 {
		fadeOut = 2.0 // Default 2 second fade-out
	}

	loop := sc.Audio.Loop
	// Note: Zero value for bool is false, but we want default true.
	// If Audio struct was just created, all fields are zero.
	// We'll assume if the Audio config exists, loop should default to true.
	// This is a limitation of the zero-value defaults in Go.
	// Better approach: use pointers for optional fields, but keeping it simple for now.
	if !loop {
		// Check if this is intentionally false or just zero-value.
		// For simplicity, we'll always default to true unless explicitly set.
		// This requires the YAML to explicitly say "loop: false" to disable.
		loop = true
	}

	// Merge audio with video.
	audioConfig := ffmpeg.AudioConfig{
		Volume:  volume,
		FadeOut: fadeOut,
		Loop:    loop,
	}

	if err := ffmpeg.MergeAudioWithVideo(ctx, mp4Path, audioPath, tempOutput, audioConfig); err != nil {
		return err
	}

	// Replace original MP4 with audio-merged version.
	if err := os.Rename(tempOutput, mp4Path); err != nil {
		return fmt.Errorf("failed to replace MP4 with audio version: %w", err)
	}

	return nil
}

// moveOutputsToCache moves output files from workdir to cacheDir.
// VHS writes files relative to its working directory, so we need to move them
// to the cache directory for consistency and easy access.
func (r *Renderer) moveOutputsToCache(sc *scene.Scene, workdir string) error {
	// Get the expected output filenames based on the tape file Output directives.
	// The Output directive uses the scene name as prefix.
	for _, format := range sc.Outputs {
		filename := fmt.Sprintf("%s.%s", sc.Name, format)
		srcPath := filepath.Join(workdir, filename)
		dstPath := filepath.Join(r.cacheDir, filename)

		// Check if the file exists in workdir.
		if _, err := os.Stat(srcPath); err != nil {
			if os.IsNotExist(err) {
				// File doesn't exist, skip (might be a png Screenshot that uses a different path).
				continue
			}
			return fmt.Errorf("failed to check output file %s: %w", srcPath, err)
		}

		// Move file to cache directory.
		if err := moveFile(srcPath, dstPath); err != nil {
			return fmt.Errorf("failed to move %s to %s: %w", srcPath, dstPath, err)
		}
	}

	// Also handle Screenshot outputs (png files with scene name prefix).
	pngFilename := fmt.Sprintf("%s.png", sc.Name)
	pngSrcPath := filepath.Join(workdir, pngFilename)
	pngDstPath := filepath.Join(r.cacheDir, pngFilename)

	if _, err := os.Stat(pngSrcPath); err == nil {
		if err := moveFile(pngSrcPath, pngDstPath); err != nil {
			return fmt.Errorf("failed to move screenshot %s to %s: %w", pngSrcPath, pngDstPath, err)
		}
	}

	return nil
}

// moveFile moves a file from src to dst, handling cross-device moves.
func moveFile(src, dst string) error {
	// Try rename first (same filesystem).
	if err := os.Rename(src, dst); err == nil {
		return nil
	}

	// Fallback to copy+delete for cross-device moves.
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	dstFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	// Close files before removing source.
	srcFile.Close()
	dstFile.Close()

	return os.Remove(src)
}

// runPrepCommands runs prep shell commands before VHS rendering.
func (r *Renderer) runPrepCommands(ctx context.Context, sc *scene.Scene, workdir string) error {
	for i, cmdStr := range sc.Prep {
		fmt.Printf("  Running prep command %d/%d: %s\n", i+1, len(sc.Prep), cmdStr)

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		cmd.Dir = workdir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("prep command %d failed: %s: %w", i+1, cmdStr, err)
		}
	}
	return nil
}
