package vhs

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/cloudposse/atmos/tools/director/internal/ffmpeg"
	"github.com/cloudposse/atmos/tools/director/internal/scene"
)

// Renderer renders VHS tape files to GIF/PNG.
type Renderer struct {
	demosDir     string
	cacheDir     string
	cache        *CacheMetadata
	force        bool
	formatFilter []string // If set, only render these formats.
	skipSVGFix   bool     // If true, skip SVG line-height post-processing.
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

// SetFormatFilter sets the format filter to only render specific formats.
func (r *Renderer) SetFormatFilter(formats []string) {
	r.formatFilter = formats
}

// SetSkipSVGFix sets whether to skip SVG line-height post-processing.
func (r *Renderer) SetSkipSVGFix(skip bool) {
	r.skipSVGFix = skip
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

	// Filter outputs if format filter is set.
	effectiveOutputs := sc.Outputs
	if len(r.formatFilter) > 0 {
		effectiveOutputs = r.filterOutputs(sc.Outputs, r.formatFilter)
		if len(effectiveOutputs) == 0 {
			// No matching formats to render.
			return &RenderResult{
				Cached:      true,
				OutputPaths: nil,
			}, nil
		}
	}

	// Build output file paths based on effective outputs.
	outputFiles := r.buildOutputPathsForFormats(sc.Name, effectiveOutputs)

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

	// Run setup commands before VHS.
	if len(sc.Setup) > 0 {
		if err := r.runSetupCommands(ctx, sc, workdir); err != nil {
			return nil, fmt.Errorf("setup commands failed: %w", err)
		}
	}

	// Defer cleanup commands to run after VHS (even on error).
	if len(sc.Cleanup) > 0 {
		defer func() {
			if cleanupErr := r.runCleanupCommands(ctx, sc, workdir); cleanupErr != nil {
				fmt.Printf("  Warning: cleanup commands failed: %v\n", cleanupErr)
			}
		}()
	}

	// Always preprocess the tape to inline Source directives.
	// This allows VHS to run from workdir since the tape becomes self-contained.
	tempTape, err := r.createFilteredTape(tapeFile, sc.Name, effectiveOutputs)
	if err != nil {
		return nil, fmt.Errorf("failed to preprocess tape: %w", err)
	}
	defer os.Remove(tempTape)
	tapeToRender := tempTape

	if err := Render(ctx, tapeToRender, workdir, r.cacheDir); err != nil {
		return nil, err
	}

	// Move output files from workdir to cacheDir.
	// VHS writes outputs relative to workdir based on the Output directive.
	if err := r.moveOutputsToCacheForFormats(sc.Name, effectiveOutputs, workdir); err != nil {
		return nil, fmt.Errorf("failed to move outputs to cache: %w", err)
	}

	// TODO: Optimize GIF with gifsicle if enabled in defaults.yaml.

	// Process audio for MP4 outputs if configured.
	if sc.Audio != nil && r.containsFormat(effectiveOutputs, "mp4") {
		if err := r.processAudioForMP4(ctx, sc); err != nil {
			return nil, fmt.Errorf("audio processing failed: %w", err)
		}
	}

	// Calculate scene duration.
	// For SVG-only scenes, estimate from tape Sleep commands.
	// For MP4 scenes, the actual duration will be updated when publishing.
	duration, err := scene.CalculateTapeDuration(tapeFile)
	if err != nil {
		// Non-fatal - just log and continue with zero duration.
		fmt.Printf("  Warning: could not calculate tape duration: %v\n", err)
		duration = 0
	}

	// Update cache with new hash and duration.
	if err := r.cache.UpdateScene(sc.Name, tapeFile, audioFile, outputFiles, duration); err != nil {
		return nil, fmt.Errorf("failed to update cache: %w", err)
	}

	// Extract and store SVG metadata if SVG was rendered.
	if r.containsFormat(effectiveOutputs, "svg") {
		svgPath := filepath.Join(r.cacheDir, fmt.Sprintf("%s.svg", sc.Name))
		if svgMeta, err := findBestSVGThumbnailTime(svgPath); err == nil && svgMeta != nil {
			// Update cache with SVG metadata.
			if sceneHash, exists := r.cache.Scenes[sc.Name]; exists {
				sceneHash.SVGDuration = svgMeta.Duration
				sceneHash.SVGThumbnailTime = svgMeta.ThumbnailTime
				r.cache.Scenes[sc.Name] = sceneHash
				fmt.Printf("  SVG thumbnail time: %.1fs (duration: %.1fs)\n", svgMeta.ThumbnailTime, svgMeta.Duration)
			}
		}
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
	return r.buildOutputPathsForFormats(sc.Name, sc.Outputs)
}

// buildOutputPathsForFormats builds output file paths for specific formats.
func (r *Renderer) buildOutputPathsForFormats(sceneName string, formats []string) []string {
	var paths []string

	for _, format := range formats {
		filename := fmt.Sprintf("%s.%s", sceneName, format)
		paths = append(paths, filepath.Join(r.cacheDir, filename))
	}

	return paths
}

// filterOutputs returns only the outputs that match the filter.
func (r *Renderer) filterOutputs(outputs, filter []string) []string {
	filterSet := make(map[string]bool)
	for _, f := range filter {
		filterSet[f] = true
	}

	var result []string
	for _, output := range outputs {
		if filterSet[output] {
			result = append(result, output)
		}
	}
	return result
}

// containsFormat checks if the outputs array contains a specific format.
func (r *Renderer) containsFormat(outputs []string, format string) bool {
	for _, output := range outputs {
		if output == format {
			return true
		}
	}
	return false
}

// containsMP4 checks if the outputs array contains "mp4".
func (r *Renderer) containsMP4(outputs []string) bool {
	return r.containsFormat(outputs, "mp4")
}

// createFilteredTape creates a temporary tape file with only the specified output formats.
// It reads the original tape, removes Output directives for formats not in the filter,
// inlines Source directives (since VHS doesn't support absolute paths and we may run from different workdir),
// and writes the result to a temp file.
func (r *Renderer) createFilteredTape(originalTape, sceneName string, formats []string) (string, error) {
	// Create set of allowed formats.
	allowedFormats := make(map[string]bool)
	for _, f := range formats {
		allowedFormats[f] = true
	}

	// Create temp file in the same directory to preserve relative paths.
	tapeDir := filepath.Dir(originalTape)
	tempFile, err := os.CreateTemp(tapeDir, "filtered-*.tape")
	if err != nil {
		return "", err
	}
	defer tempFile.Close()

	// Process the tape file, inlining Source directives.
	if err := r.processFilteredTape(originalTape, tapeDir, sceneName, allowedFormats, tempFile); err != nil {
		os.Remove(tempFile.Name())
		return "", err
	}

	return tempFile.Name(), nil
}

// processFilteredTape processes a tape file, filtering outputs and inlining sources.
func (r *Renderer) processFilteredTape(tapeFile, baseDir, sceneName string, allowedFormats map[string]bool, out *os.File) error {
	// Read tape file.
	file, err := os.Open(tapeFile)
	if err != nil {
		return err
	}
	defer file.Close()

	// Regex to match Output directives: "Output scenename.format".
	outputRegex := regexp.MustCompile(`^Output\s+` + regexp.QuoteMeta(sceneName) + `\.(\w+)\s*$`)
	// Regex to match Source directives: "Source path/to/file.tape" or Source "path/to/file.tape".
	sourceRegex := regexp.MustCompile(`^Source\s+"?([^"]+)"?\s*$`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check if this is an Output directive.
		if matches := outputRegex.FindStringSubmatch(line); matches != nil {
			format := matches[1]
			// Only include if format is in the allowed list.
			if !allowedFormats[format] {
				continue
			}
		}

		// Check if this is a Source directive - inline its content.
		if matches := sourceRegex.FindStringSubmatch(line); matches != nil {
			sourcePath := matches[1]
			// Resolve relative to tape file directory.
			if !filepath.IsAbs(sourcePath) {
				sourcePath = filepath.Join(filepath.Dir(tapeFile), sourcePath)
			}
			// Read and inline the source file content.
			if _, err := os.Stat(sourcePath); err == nil {
				// Write comment indicating where the content came from.
				if _, err := fmt.Fprintf(out, "# Inlined from: %s\n", filepath.Base(sourcePath)); err != nil {
					return err
				}
				// Recursively process the source file (it might have its own Source directives).
				if err := r.inlineSourceFile(sourcePath, filepath.Dir(sourcePath), out); err != nil {
					return fmt.Errorf("failed to inline source %s: %w", sourcePath, err)
				}
				continue // Don't write the original Source line.
			}
			// If file doesn't exist, keep the original line (VHS will error on it anyway).
		}

		// Write line to temp file.
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}

	return scanner.Err()
}

// inlineSourceFile reads a source file and writes its content to the output, processing nested Sources.
func (r *Renderer) inlineSourceFile(sourcePath, baseDir string, out *os.File) error {
	file, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer file.Close()

	// Regex to match Source directives (handles both quoted and unquoted paths).
	sourceRegex := regexp.MustCompile(`^Source\s+"?([^"]+)"?\s*$`)

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()

		// Check if this is a nested Source directive.
		if matches := sourceRegex.FindStringSubmatch(line); matches != nil {
			nestedPath := matches[1]
			if !filepath.IsAbs(nestedPath) {
				nestedPath = filepath.Join(baseDir, nestedPath)
			}
			if _, err := os.Stat(nestedPath); err == nil {
				if _, err := fmt.Fprintf(out, "# Inlined from: %s\n", filepath.Base(nestedPath)); err != nil {
					return err
				}
				if err := r.inlineSourceFile(nestedPath, filepath.Dir(nestedPath), out); err != nil {
					return err
				}
				continue
			}
		}

		// Write line.
		if _, err := fmt.Fprintln(out, line); err != nil {
			return err
		}
	}

	return scanner.Err()
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

	// Default loop to true unless explicitly set to false.
	loop := true
	if sc.Audio.Loop != nil {
		loop = *sc.Audio.Loop
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
	return r.moveOutputsToCacheForFormats(sc.Name, sc.Outputs, workdir)
}

// moveOutputsToCacheForFormats moves specific format outputs from workdir to cacheDir.
func (r *Renderer) moveOutputsToCacheForFormats(sceneName string, formats []string, workdir string) error {
	// Get the expected output filenames based on the tape file Output directives.
	// The Output directive uses the scene name as prefix.
	for _, format := range formats {
		filename := fmt.Sprintf("%s.%s", sceneName, format)
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

		// Post-process SVG files to fix line height (unless skipped).
		if format == "svg" && !r.skipSVGFix {
			if err := r.postProcessSVG(dstPath); err != nil {
				return fmt.Errorf("failed to post-process SVG %s: %w", dstPath, err)
			}
		}
	}

	// Also handle Screenshot outputs (png files with scene name prefix) if png is in formats.
	if r.containsFormat(formats, "png") {
		pngFilename := fmt.Sprintf("%s.png", sceneName)
		pngSrcPath := filepath.Join(workdir, pngFilename)
		pngDstPath := filepath.Join(r.cacheDir, pngFilename)

		if _, err := os.Stat(pngSrcPath); err == nil {
			if err := moveFile(pngSrcPath, pngDstPath); err != nil {
				return fmt.Errorf("failed to move screenshot %s to %s: %w", pngSrcPath, pngDstPath, err)
			}
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

// runSetupCommands runs setup shell commands before VHS rendering.
func (r *Renderer) runSetupCommands(ctx context.Context, sc *scene.Scene, workdir string) error {
	for i, cmdStr := range sc.Setup {
		fmt.Printf("  Running setup command %d/%d: %s\n", i+1, len(sc.Setup), cmdStr)

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		cmd.Dir = workdir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("setup command %d failed: %s: %w", i+1, cmdStr, err)
		}
	}
	return nil
}

// runCleanupCommands runs cleanup shell commands after VHS rendering.
func (r *Renderer) runCleanupCommands(ctx context.Context, sc *scene.Scene, workdir string) error {
	for i, cmdStr := range sc.Cleanup {
		fmt.Printf("  Running cleanup command %d/%d: %s\n", i+1, len(sc.Cleanup), cmdStr)

		cmd := exec.CommandContext(ctx, "sh", "-c", cmdStr)
		cmd.Dir = workdir
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cleanup command %d failed: %s: %w", i+1, cmdStr, err)
		}
	}
	return nil
}

// fixSVGLineHeight fixes the line height in an SVG file generated by VHS.
// VHS uses a fixed charHeight of 34px regardless of font-size, resulting in
// excessive line spacing. This function scales Y coordinates of text elements
// to achieve proper line height based on the configured font size and line height.
func fixSVGLineHeight(svgPath string, fontSize, lineHeight float64) error {
	// VHS uses charHeight=34 internally for SVG rendering (measured from actual output).
	// Each text line is placed at y = lineNumber * 34 (e.g., 27.2, 61.2, 95.2...).
	// We want lines at y = lineNumber * (fontSize * lineHeight).
	// Scale = (fontSize * lineHeight) / charHeight
	// Example: (14 * 1.0) / 34 = 14 / 34 = 0.41
	vhsCharHeight := 34.0
	targetLineSpacing := fontSize * lineHeight
	scale := targetLineSpacing / vhsCharHeight

	// Read the SVG file.
	content, err := os.ReadFile(svgPath)
	if err != nil {
		return fmt.Errorf("failed to read SVG: %w", err)
	}

	svgContent := string(content)

	// Track the maximum Y coordinate we encounter to calculate new height.
	var maxY float64

	// Regular expression to match <text> elements with y="N" attributes.
	// We only want to scale Y coordinates inside <text> elements, not the viewBox/height.
	// VHS SVG structure: <text ... y="N">...</text>
	textYRegex := regexp.MustCompile(`(<text[^>]*\s)y="([0-9.]+)"`)

	// Replace Y coordinates in text elements with scaled values.
	fixed := textYRegex.ReplaceAllStringFunc(svgContent, func(match string) string {
		matches := textYRegex.FindStringSubmatch(match)
		if len(matches) < 3 {
			return match
		}
		yVal, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return match
		}
		// Scale the Y value.
		newY := yVal * scale
		if newY > maxY {
			maxY = newY
		}
		return fmt.Sprintf(`%sy="%.1f"`, matches[1], newY)
	})

	// Also handle <tspan> elements which may have their own y coordinates.
	tspanYRegex := regexp.MustCompile(`(<tspan[^>]*\s)y="([0-9.]+)"`)
	fixed = tspanYRegex.ReplaceAllStringFunc(fixed, func(match string) string {
		matches := tspanYRegex.FindStringSubmatch(match)
		if len(matches) < 3 {
			return match
		}
		yVal, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return match
		}
		newY := yVal * scale
		if newY > maxY {
			maxY = newY
		}
		return fmt.Sprintf(`%sy="%.1f"`, matches[1], newY)
	})

	// Scale Y coordinates in rect elements (background boxes for highlights).
	// Only matches rects that have a y attribute (positioned elements, not outer background).
	rectYRegex := regexp.MustCompile(`(<rect[^>]*\s)y="([0-9.]+)"`)
	fixed = rectYRegex.ReplaceAllStringFunc(fixed, func(match string) string {
		matches := rectYRegex.FindStringSubmatch(match)
		if len(matches) < 3 {
			return match
		}
		yVal, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return match
		}
		newY := yVal * scale
		if newY > maxY {
			maxY = newY
		}
		return fmt.Sprintf(`%sy="%.1f"`, matches[1], newY)
	})

	// Scale heights in rect elements that have a y attribute (positioned elements only).
	// Matches: <rect ... y="..." ... height="..." ...> (y must appear before height).
	// This excludes outer background rects like <rect width="1400" height="800"/>.
	rectHeightRegex := regexp.MustCompile(`(<rect[^>]*y="[^"]*"[^>]*\s)height="([0-9.]+)"`)
	fixed = rectHeightRegex.ReplaceAllStringFunc(fixed, func(match string) string {
		matches := rectHeightRegex.FindStringSubmatch(match)
		if len(matches) < 3 {
			return match
		}
		heightVal, err := strconv.ParseFloat(matches[2], 64)
		if err != nil {
			return match
		}
		newHeight := heightVal * scale
		return fmt.Sprintf(`%sheight="%.1f"`, matches[1], newHeight)
	})

	// Write the fixed SVG back.
	if err := os.WriteFile(svgPath, []byte(fixed), 0o644); err != nil {
		return fmt.Errorf("failed to write fixed SVG: %w", err)
	}

	return nil
}

// fixSVGBackgroundColor replaces the terminal background color in SVG files.
// VHS renders with #1a1a1a but video encoding shifts this to #1e1d2e.
// This post-processing ensures SVGs match the MP4 appearance.
func fixSVGBackgroundColor(svgPath string) error {
	content, err := os.ReadFile(svgPath)
	if err != nil {
		return fmt.Errorf("failed to read SVG: %w", err)
	}

	// Replace VHS theme background with video-matching color.
	fixed := strings.ReplaceAll(string(content), "#1a1a1a", "#1e1d2e")

	return os.WriteFile(svgPath, []byte(fixed), 0o644)
}

// SVGMetadata contains extracted metadata from an SVG animation.
type SVGMetadata struct {
	Duration      float64 // Animation duration in seconds.
	ThumbnailTime float64 // Best frame time for thumbnail (color + content score).
}

// findBestSVGThumbnailTime analyzes SVG animation frames to find the best thumbnail frame.
// It scores frames based on color diversity (60%) and content density (40%).
// Returns the best timestamp and the total animation duration.
func findBestSVGThumbnailTime(svgPath string) (*SVGMetadata, error) {
	content, err := os.ReadFile(svgPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read SVG: %w", err)
	}

	svg := string(content)

	// Extract animation duration from CSS: animation: slide Xs step-end ...
	durationRe := regexp.MustCompile(`animation:\s*slide\s+([\d.]+)s`)
	durationMatch := durationRe.FindStringSubmatch(svg)
	duration := 0.0
	if len(durationMatch) > 1 {
		duration, _ = strconv.ParseFloat(durationMatch[1], 64)
	}

	if duration == 0 {
		return &SVGMetadata{Duration: 0, ThumbnailTime: 0}, nil
	}

	// Find all frame groups: <g transform="translate(X,0)">...</g>
	// Each frame represents a different point in time.
	frameRe := regexp.MustCompile(`<g transform="translate\((\d+),0\)">(.*?)</g>`)
	frames := frameRe.FindAllStringSubmatch(svg, -1)

	if len(frames) == 0 {
		// Fallback to 80% of duration.
		return &SVGMetadata{Duration: duration, ThumbnailTime: duration * 0.8}, nil
	}

	// Color classes used in VHS SVGs (from theme).
	colorClasses := []string{"r", "g", "y", "b", "m", "c", "p", "w"}

	// Regex to count tspan elements (each represents a text segment).
	tspanRe := regexp.MustCompile(`<tspan[^>]*>`)

	type scoredFrame struct {
		index        int
		colorCount   int
		contentCount int // Number of tspan elements (text density).
	}
	scoredFrames := make([]scoredFrame, 0, len(frames))

	for i, frame := range frames {
		if len(frame) < 3 {
			continue
		}
		content := frame[2]

		// Count unique color classes in this frame.
		colorCount := 0
		for _, cls := range colorClasses {
			// Check for class="X" or class="t X" patterns.
			if strings.Contains(content, fmt.Sprintf(`class="%s"`, cls)) ||
				strings.Contains(content, fmt.Sprintf(`class="t %s"`, cls)) ||
				strings.Contains(content, fmt.Sprintf(` %s"`, cls)) {
				colorCount++
			}
		}

		// Count tspan elements as a proxy for content density.
		contentCount := len(tspanRe.FindAllString(content, -1))

		scoredFrames = append(scoredFrames, scoredFrame{index: i, colorCount: colorCount, contentCount: contentCount})
	}

	if len(scoredFrames) == 0 {
		return &SVGMetadata{Duration: duration, ThumbnailTime: duration * 0.8}, nil
	}

	// Find max values for normalization.
	maxColors := 0
	maxContent := 0
	for _, f := range scoredFrames {
		if f.colorCount > maxColors {
			maxColors = f.colorCount
		}
		if f.contentCount > maxContent {
			maxContent = f.contentCount
		}
	}

	// Score function: combines color diversity and content density.
	// Color diversity is weighted slightly higher (0.6) since colorful frames are visually appealing.
	// Content density (0.4) ensures we pick frames with substantial visible output.
	scoreFrame := func(f scoredFrame) float64 {
		colorScore := 0.0
		if maxColors > 0 {
			colorScore = float64(f.colorCount) / float64(maxColors)
		}
		contentScore := 0.0
		if maxContent > 0 {
			contentScore = float64(f.contentCount) / float64(maxContent)
		}
		return colorScore*0.6 + contentScore*0.4
	}

	// Find frame with highest combined score.
	bestFrame := scoredFrames[0]
	bestScore := scoreFrame(bestFrame)
	for _, f := range scoredFrames {
		score := scoreFrame(f)
		if score > bestScore {
			bestScore = score
			bestFrame = f
		}
	}

	// If best frame is at the beginning (probably just prompt) or has low content,
	// prefer a frame around 75-85% of the animation where output is typically visible.
	// This handles demos with monochrome output (like list-stacks) or slow-typing demos.
	if bestFrame.index < len(scoredFrames)/4 || bestFrame.contentCount < maxContent/2 {
		// Find frames in the 75-85% range with at least as good a score.
		targetStart := int(float64(len(scoredFrames)) * 0.75)
		targetEnd := int(float64(len(scoredFrames)) * 0.85)
		for _, f := range scoredFrames {
			if f.index >= targetStart && f.index <= targetEnd && scoreFrame(f) >= bestScore*0.8 {
				bestFrame = f
				break
			}
		}
	}

	// Calculate timestamp for best frame.
	thumbnailTime := float64(bestFrame.index) / float64(len(frames)) * duration

	return &SVGMetadata{
		Duration:      duration,
		ThumbnailTime: thumbnailTime,
	}, nil
}

// postProcessSVG applies post-processing fixes to SVG files.
func (r *Renderer) postProcessSVG(svgPath string) error {
	// Default values matching the tape theme settings.
	// Note: LineHeight should match the tape's "Set LineHeight" value.
	// Using 1.0 ensures terminal row count matches visual rendering for TUI apps.
	fontSize := 14.0
	lineHeight := 1.0

	// TODO: Parse font-size and line-height from the SVG or tape file.
	// For now, use the defaults from tape settings.

	if !strings.HasSuffix(svgPath, ".svg") {
		return nil
	}

	if _, err := os.Stat(svgPath); os.IsNotExist(err) {
		return nil // File doesn't exist, skip.
	}

	fmt.Printf("  Post-processing SVG line height: %s\n", filepath.Base(svgPath))
	if err := fixSVGLineHeight(svgPath, fontSize, lineHeight); err != nil {
		return err
	}

	fmt.Printf("  Post-processing SVG background color: %s\n", filepath.Base(svgPath))
	return fixSVGBackgroundColor(svgPath)
}
