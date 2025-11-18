package ffmpeg

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
