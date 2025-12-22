package ffmpeg

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	errUtils "github.com/cloudposse/atmos/errors"
	"github.com/cloudposse/atmos/pkg/perf"
)

const (
	// Float64BitSize is the bitsize for ParseFloat (64-bit floating point).
	Float64BitSize = 64
)

// CheckInstalled verifies that FFmpeg and FFprobe are available in the system PATH.
func CheckInstalled() error {
	defer perf.Track(nil, "ffmpeg.CheckInstalled")()

	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("%w (install: brew install ffmpeg)", errUtils.ErrFFmpegNotFound)
	}
	if _, err := exec.LookPath("ffprobe"); err != nil {
		return fmt.Errorf("%w (install: brew install ffmpeg)", errUtils.ErrFFprobeNotFound)
	}
	return nil
}

// GetVideoDuration returns the duration of a video file in seconds.
func GetVideoDuration(ctx context.Context, videoPath string) (float64, error) {
	defer perf.Track(nil, "ffmpeg.GetVideoDuration")()

	cmd := exec.CommandContext(ctx, "ffprobe",
		"-v", "error",
		"-show_entries", "format=duration",
		"-of", "default=noprint_wrappers=1:nokey=1",
		videoPath,
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %w", err)
	}

	durationStr := strings.TrimSpace(string(output))
	duration, err := strconv.ParseFloat(durationStr, Float64BitSize)
	if err != nil {
		return 0, fmt.Errorf("failed to parse duration '%s': %w", durationStr, err)
	}

	return duration, nil
}

// AudioConfig contains audio processing configuration.
type AudioConfig struct {
	Volume  float64 // Volume level (0.0-1.0)
	FadeOut float64 // Fade-out duration in seconds
	Loop    bool    // Whether to loop audio if shorter than video
}

// ConvertGIFToMP4 converts a GIF file to MP4 format suitable for web playback.
// Uses H.264 codec with settings optimized for browser compatibility.
func ConvertGIFToMP4(ctx context.Context, gifPath, mp4Path string) error {
	defer perf.Track(nil, "ffmpeg.ConvertGIFToMP4")()

	// FFmpeg command to convert GIF to MP4 with web-friendly settings.
	// -movflags faststart: Enables progressive download/streaming
	// -pix_fmt yuv420p: Required for browser compatibility
	// -vf "scale=trunc(iw/2)*2:trunc(ih/2)*2": Ensures even dimensions (required for H.264)
	args := []string{
		"-i", gifPath,
		"-movflags", "faststart",
		"-pix_fmt", "yuv420p",
		"-vf", "scale=trunc(iw/2)*2:trunc(ih/2)*2",
		"-c:v", "libx264",
		"-preset", "medium",
		"-crf", "23",
		"-y", // Overwrite output
		mp4Path,
	}

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg GIF to MP4 conversion failed: %w", err)
	}

	return nil
}

// MergeAudioWithVideo adds an audio track to a video file.
// If config.Loop is true and audio is shorter than video, audio will loop seamlessly.
// Audio will fade out over the last config.FadeOut seconds.
func MergeAudioWithVideo(ctx context.Context, videoPath, audioPath, outputPath string, config AudioConfig) error {
	defer perf.Track(nil, "ffmpeg.MergeAudioWithVideo")()

	// Get video duration to calculate fade-out timing.
	duration, err := GetVideoDuration(ctx, videoPath)
	if err != nil {
		return err
	}

	// Build filter for volume and fade-out.
	fadeStart := duration - config.FadeOut
	if fadeStart < 0 {
		fadeStart = 0
	}

	audioFilter := fmt.Sprintf("[1:a]afade=t=out:st=%.2f:d=%.2f,volume=%.2f[a]", fadeStart, config.FadeOut, config.Volume)

	// Build FFmpeg command.
	args := []string{
		"-i", videoPath,
	}

	// Add audio looping if enabled.
	if config.Loop {
		args = append(args, "-stream_loop", "-1")
	}

	args = append(args,
		"-i", audioPath,
		"-filter_complex", audioFilter,
		"-map", "0:v:0", // Map video from first input
		"-map", "[a]", // Map filtered audio
		"-shortest",    // Stop when shortest stream ends (video)
		"-c:v", "copy", // Copy video codec (no re-encoding)
		"-c:a", "aac", // Encode audio as AAC
		"-f", "mp4", // Explicitly specify output format (for .tmp extension)
		"-y", // Overwrite output file
		outputPath,
	)

	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffmpeg audio merge failed: %w", err)
	}

	return nil
}

// FrameColorInfo holds color analysis data for a video frame.
type FrameColorInfo struct {
	Timestamp  float64 // Time in seconds.
	Saturation float64 // Average saturation (0-255).
	UniqueHues int     // Number of unique hue buckets.
	ColorScore float64 // Combined score for ranking.
}

// FindMostColorfulFrame analyzes a video and finds the frame with the most colors.
// It samples frames at regular intervals and returns the timestamp of the most colorful one.
// sampleCount determines how many frames to analyze (more = slower but more accurate).
func FindMostColorfulFrame(ctx context.Context, videoPath string, sampleCount int) (float64, error) {
	defer perf.Track(nil, "ffmpeg.FindMostColorfulFrame")()

	if sampleCount <= 0 {
		sampleCount = 20 // Default to 20 samples.
	}

	// Get video duration.
	duration, err := GetVideoDuration(ctx, videoPath)
	if err != nil {
		return 0, err
	}

	// Create temp directory for frame extraction.
	tempDir, err := os.MkdirTemp("", "colorframe-*")
	if err != nil {
		return 0, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Calculate frame timestamps to sample.
	interval := duration / float64(sampleCount+1)
	frames := make([]FrameColorInfo, 0, sampleCount)

	for i := 1; i <= sampleCount; i++ {
		timestamp := interval * float64(i)
		framePath := filepath.Join(tempDir, fmt.Sprintf("frame_%03d.png", i))

		// Extract frame at this timestamp.
		extractCmd := exec.CommandContext(ctx, "ffmpeg",
			"-ss", fmt.Sprintf("%.3f", timestamp),
			"-i", videoPath,
			"-vframes", "1",
			"-y",
			framePath,
		)
		if err := extractCmd.Run(); err != nil {
			continue // Skip frames we can't extract.
		}

		// Analyze frame colors using ImageMagick's identify with histogram.
		score, err := analyzeFrameColors(ctx, framePath)
		if err != nil {
			continue
		}

		frames = append(frames, FrameColorInfo{
			Timestamp:  timestamp,
			ColorScore: score,
		})
	}

	if len(frames) == 0 {
		// Fallback to 25% into the video if we couldn't analyze any frames.
		return duration * 0.25, nil
	}

	// Sort by color score (highest first).
	sort.Slice(frames, func(i, j int) bool {
		return frames[i].ColorScore > frames[j].ColorScore
	})

	return frames[0].Timestamp, nil
}

// analyzeFrameColors analyzes a frame image and returns a color richness score.
// Uses ffprobe to get color statistics without requiring ImageMagick.
func analyzeFrameColors(ctx context.Context, framePath string) (float64, error) {
	// Use ffprobe to get frame info with signalstats filter for color analysis.
	// We'll use the entropy of the histogram as a proxy for color richness.
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-f", "lavfi",
		"-i", fmt.Sprintf("movie=%s,signalstats=stat=tout+vrep+brng", framePath),
		"-show_entries", "frame_tags",
		"-of", "default=noprint_wrappers=1",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// Fallback: use simpler histogram approach.
		return analyzeFrameHistogram(ctx, framePath)
	}

	// Parse SATAVG (average saturation) from output.
	lines := strings.Split(string(output), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "tag:lavfi.signalstats.SATAVG=") {
			valStr := strings.TrimPrefix(line, "tag:lavfi.signalstats.SATAVG=")
			val, err := strconv.ParseFloat(strings.TrimSpace(valStr), Float64BitSize)
			if err == nil {
				return val, nil
			}
		}
	}

	// Fallback if parsing failed.
	return analyzeFrameHistogram(ctx, framePath)
}

// analyzeFrameHistogram uses ffmpeg's histogram filter to estimate color richness.
func analyzeFrameHistogram(ctx context.Context, framePath string) (float64, error) {
	// Use ffmpeg to generate histogram data.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", framePath,
		"-vf", "format=rgb24,histogram=display_mode=overlay:level_height=50",
		"-f", "null",
		"-",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		// As last resort, estimate based on file size (larger = likely more detail).
		info, statErr := os.Stat(framePath)
		if statErr != nil {
			return 0, fmt.Errorf("failed to analyze frame: %w", err)
		}
		// Normalize file size to a score (larger files often have more color detail).
		return float64(info.Size()) / 1000.0, nil
	}

	// Count unique color mentions in output (rough heuristic).
	colorPattern := regexp.MustCompile(`\b[0-9]{1,3}\b`)
	matches := colorPattern.FindAllString(string(output), -1)

	return float64(len(matches)), nil
}

// GetFrameSaturation returns the average saturation of a specific frame timestamp.
func GetFrameSaturation(ctx context.Context, videoPath string, timestamp float64) (float64, error) {
	defer perf.Track(nil, "ffmpeg.GetFrameSaturation")()

	// Use ffprobe with select filter to analyze specific frame.
	cmd := exec.CommandContext(ctx, "ffprobe",
		"-f", "lavfi",
		"-i", fmt.Sprintf("movie=%s:seek_point=%.3f,signalstats", videoPath, timestamp),
		"-show_entries", "frame_tags=lavfi.signalstats.SATAVG",
		"-of", "default=noprint_wrappers=1:nokey=1",
		"-read_intervals", "%+#1", // Only read 1 frame.
	)

	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get saturation: %w", err)
	}

	satStr := strings.TrimSpace(string(output))
	if satStr == "" {
		return 0, nil
	}

	sat, err := strconv.ParseFloat(satStr, Float64BitSize)
	if err != nil {
		return 0, fmt.Errorf("failed to parse saturation '%s': %w", satStr, err)
	}

	return sat, nil
}

// FindBestThumbnailTime analyzes video and finds the best thumbnail timestamp.
// For terminal recordings, this looks for frames with the most visual content
// (colored text, progress bars, formatted output) rather than empty prompts.
func FindBestThumbnailTime(ctx context.Context, videoPath string) (float64, error) {
	defer perf.Track(nil, "ffmpeg.FindBestThumbnailTime")()

	duration, err := GetVideoDuration(ctx, videoPath)
	if err != nil {
		return 0, err
	}

	// For short videos (< 30s), sample more frames.
	sampleCount := 30
	if duration < 15 {
		sampleCount = 15
	}

	// Sample frames evenly across the video, avoiding first and last 10%.
	startPct := 0.10
	endPct := 0.90
	startTime := duration * startPct
	endTime := duration * endPct
	sampleRange := endTime - startTime

	type scoredFrame struct {
		timestamp float64
		score     float64
	}
	frames := make([]scoredFrame, 0, sampleCount)

	// Create temp directory for frame extraction.
	tempDir, err := os.MkdirTemp("", "thumbnail-*")
	if err != nil {
		return duration * 0.5, nil // Fallback to middle.
	}
	defer os.RemoveAll(tempDir)

	for i := 0; i < sampleCount; i++ {
		// Calculate timestamp (evenly spaced in middle 80%).
		pct := float64(i) / float64(sampleCount-1)
		timestamp := startTime + (sampleRange * pct)

		framePath := filepath.Join(tempDir, fmt.Sprintf("frame_%03d.png", i))

		// Extract frame at this timestamp.
		extractCmd := exec.CommandContext(ctx, "ffmpeg",
			"-ss", fmt.Sprintf("%.3f", timestamp),
			"-i", videoPath,
			"-vframes", "1",
			"-y",
			framePath,
		)
		if err := extractCmd.Run(); err != nil {
			continue
		}

		// Get file size as a proxy for visual complexity.
		// Terminal frames with more content (text, colors) are larger.
		info, err := os.Stat(framePath)
		if err != nil {
			continue
		}

		// Score based on file size (larger = more content).
		score := float64(info.Size())

		frames = append(frames, scoredFrame{
			timestamp: timestamp,
			score:     score,
		})
	}

	if len(frames) == 0 {
		return duration * 0.5, nil // Fallback to middle.
	}

	// Sort by score (highest first).
	sort.Slice(frames, func(i, j int) bool {
		return frames[i].score > frames[j].score
	})

	// Return the frame with most content.
	return frames[0].timestamp, nil
}

// detectSceneChanges finds timestamps where significant scene changes occur.
func detectSceneChanges(ctx context.Context, videoPath string) ([]float64, error) {
	// Use ffmpeg's scene detection filter.
	cmd := exec.CommandContext(ctx, "ffmpeg",
		"-i", videoPath,
		"-vf", "select='gt(scene,0.3)',showinfo",
		"-f", "null",
		"-",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, err
	}

	// Parse timestamps from showinfo output.
	timestamps := make([]float64, 0)
	scanner := bufio.NewScanner(strings.NewReader(string(output)))
	ptsTimePattern := regexp.MustCompile(`pts_time:([0-9.]+)`)

	for scanner.Scan() {
		line := scanner.Text()
		if matches := ptsTimePattern.FindStringSubmatch(line); len(matches) > 1 {
			ts, err := strconv.ParseFloat(matches[1], Float64BitSize)
			if err == nil {
				timestamps = append(timestamps, ts)
			}
		}
	}

	return timestamps, nil
}
