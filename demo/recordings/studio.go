package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/viewport"
	"github.com/dustin/go-humanize"
	"github.com/go-git/go-git/v5"
	"github.com/spf13/cobra"
)

// Paths
var (
	repoRoot  string
	tapesDir  string
	scenesDir string
	mp4OutDir string
	gifOutDir string
	audioFile string
)

// Timeout for VHS processing
const vhsTimeout = 10 * time.Minute

// Styles
var (
	successMark = lipgloss.NewStyle().Foreground(lipgloss.Color("#00FF00")).Bold(false).Render("✓") // Bright green
	errorMark   = lipgloss.NewStyle().Foreground(lipgloss.Color("#FF0000")).Bold(false).Render("x") // Bright red
	neutralMark = lipgloss.NewStyle().Foreground(lipgloss.Color("#555555")).Bold(false).Render("-") // Dark gray
)

func init() {
	var err error

	log.SetDefault(log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false,
		ReportCaller:    false,
	}))

	log.SetLevel(log.DebugLevel)

	repoRoot, err = getGitRoot()
	if err != nil {
		log.Fatal("Error detecting Git root", "error", err)
	}

	tapesDir = filepath.Join(repoRoot, "demo", "recordings", "tapes")
	scenesDir = filepath.Join(repoRoot, "demo", "recordings", "scenes")
	mp4OutDir = filepath.Join(repoRoot, "demo", "recordings", "mp4")
	gifOutDir = filepath.Join(repoRoot, "demo", "recordings", "gif")
	audioFile = filepath.Join(repoRoot, "demo", "recordings", "background.mp3")

	log.Info("Initialized", "repoRoot", ConvertToRelativeFromCWD(repoRoot), "tapesDir", ConvertToRelativeFromCWD(tapesDir), "scenesDir", ConvertToRelativeFromCWD(scenesDir), "mp4OutDir", ConvertToRelativeFromCWD(mp4OutDir), "gifOutDir", ConvertToRelativeFromCWD(gifOutDir), "audioFile", ConvertToRelativeFromCWD(audioFile))
}

func main() {
	rootCmd := &cobra.Command{Use: "studio"}

	rootCmd.AddCommand(&cobra.Command{
		Use:   "clean",
		Short: "Clean up generated files",
		RunE: func(cmd *cobra.Command, args []string) error {
			_, err := RunCmdWithSpinner("Cleaning up generated files...", exec.Command("rm", "-rf", mp4OutDir, gifOutDir))
			return err
		},
	})

	rootCmd.AddCommand(&cobra.Command{
		Use:   "build",
		Short: "Convert tapes, process scenes, and generate GIFs",
		RunE: func(cmd *cobra.Command, args []string) error {
			return Build()
		},
	})

	if err := rootCmd.Execute(); err != nil {
		log.Fatal("Command execution failed", "error", err)
	}
}

// Run a Command with a Spinner
func RunCmdWithSpinner(title string, cmd *exec.Cmd) (int, error) {
	// Ensure we run commands from the repo root
	repoRoot, err := getGitRoot()
	if err != nil {
		return -1, fmt.Errorf("failed to get repo root: %w", err)
	}

	cmd.Stdin = strings.NewReader("") // Explicitly detach input
	cmd.Dir = repoRoot                // Change working directory

	m, err := viewport.RunWithSpinner(title, func(output chan string, logLines *[]string) (int, error) {
		return viewport.RunCommand(output, logLines, cmd)
	})

	exitCode := m.ExitCode // Extract correct exit code

	elapsed := time.Since(m.Start).Round(time.Second)
	timer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("8")).
		Render(fmt.Sprintf("(%s)", elapsed))

	if exitCode == -1 {
		fmt.Printf("%s %s. Aborted by user. %s\n", neutralMark, title, timer)
		os.Exit(130)
	} else if err != nil || exitCode != 0 {
		fmt.Printf("%s %s. Error encountered. Command exited with code %d. %s\n", errorMark, title, exitCode, timer)
		fmt.Println("=== Full Log Dump ===")
		fmt.Println(strings.Join(*m.LogLines, "\n"))
		return exitCode, err
	} else {
		fmt.Printf("%s %s %s\n", successMark, title, timer)
	}
	return exitCode, nil
}

// ConvertToRelativeFromCWD converts an absolute file path to a relative path based on the current working directory.
func ConvertToRelativeFromCWD(absPath string) string {
	// Get the current working directory
	cwd, err := os.Getwd()
	if err != nil {
		log.Error("Error getting working directory", "error", err)
		return absPath
	}

	// Get absolute versions of both paths for consistency
	absFile, err := filepath.Abs(absPath)
	if err != nil {
		log.Error("failed to get absolute file path", "error", err)
		return absPath
	}

	// Convert absolute path to relative path
	relPath, err := filepath.Rel(cwd, absFile)
	if err != nil {
		log.Error("failed to compute relative path", "error", err)
		return absPath
	}

	return relPath
}

// Git Root Detection
func getGitRoot() (string, error) {
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %w", err)
	}

	repo, err := git.PlainOpenWithOptions(currentDir, &git.PlainOpenOptions{DetectDotGit: true})
	if err != nil {
		return "", fmt.Errorf("failed to open git repository: %w", err)
	}

	worktree, err := repo.Worktree()
	if err != nil {
		return "", fmt.Errorf("failed to get worktree: %w", err)
	}

	return worktree.Filesystem.Root(), nil
}

// Ensure Directories Exist
func ensureDirs(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

// Build Process
func Build() error {
	if err := ensureDirs(mp4OutDir, gifOutDir); err != nil {
		return err
	}

	if err := convertTapes(); err != nil {
		return fmt.Errorf("error converting tapes: %w", err)
	}

	if err := processScenes(); err != nil {
		return fmt.Errorf("error processing scenes: %w", err)
	}

	return nil
}

// Convert Tapes
func convertTapes() error {
	files, err := filepath.Glob(filepath.Join(tapesDir, "*.tape"))
	if err != nil {
		return fmt.Errorf("failed to list tape files: %w", err)
	}
	if len(files) == 0 {
		log.Info("No .tape files found. Skipping conversion.")
		return nil
	}

	for _, tape := range files {
		var exitCode int
		var err error

		baseName := filepath.Base(tape[:len(tape)-len(filepath.Ext(tape))])
		outputMp4 := filepath.Join(mp4OutDir, baseName+".mp4")
		outputGif := filepath.Join(gifOutDir, baseName+".gif")

		if isUpToDate(outputMp4, tape) {
			log.Info("Skipping tape recording", "tape", ConvertToRelativeFromCWD(tape), "reason", "already up-to-date")
		} else {
			if exitCode, err = RunCmdWithSpinner(fmt.Sprintf("Recording %s to mp4...", baseName), exec.Command("vhs", tape, "--output", outputMp4)); err != nil || exitCode != 0 {
				log.Error("Failed to record tape", "tape", ConvertToRelativeFromCWD(tape), "file", ConvertToRelativeFromCWD(outputMp4), "error", err)
				os.Exit(exitCode)
			} else {
				log.Info("Recorded tape to mp4", "file", ConvertToRelativeFromCWD(outputMp4))
			}
		}

		if isUpToDate(outputGif, outputMp4) {
			log.Info("Skipping GIF generation", "file", ConvertToRelativeFromCWD(outputGif), "reason", "already up-to-date")
			continue
		} else {
			if exitCode, err = RunCmdWithSpinner(fmt.Sprintf("Generating GIF for %s...", baseName), exec.Command("ffmpeg", "-i", outputMp4, "-y", outputGif)); err != nil || exitCode != 0 {
				log.Error("Failed to generate GIF", "file", baseName, "error", err)
				os.Exit(exitCode)
			} else {
				log.Info("Generated GIF", "file", ConvertToRelativeFromCWD(outputGif))
			}
		}
	}

	return nil
}

// Process Scenes
func processScenes() error {
	files, err := filepath.Glob(filepath.Join(scenesDir, "*.txt"))
	if err != nil {
		return fmt.Errorf("failed to list scene files: %w", err)
	}

	if len(files) == 0 {
		log.Info("No scene files found. Skipping processing.")
		return nil
	}

	log.Info("Processing scenes...", "scenes", len(files))

	for _, sceneFile := range files {
		sceneName := filepath.Base(sceneFile[:len(sceneFile)-len(filepath.Ext(sceneFile))])
		outputMp4 := filepath.Join(mp4OutDir, sceneName+".mp4")
		outputMp4WithAudio := filepath.Join(mp4OutDir, sceneName+"-with-audio.mp4")
		outputGif := filepath.Join(gifOutDir, sceneName+".gif")

		// Concatenate scene files into MP4
		if isSceneUpToDate(outputMp4, sceneFile) {
			log.Info("Skipping concatenation", "scene", sceneName, "reason", "already up-to-date")
		} else {
			exitCode, err := RunCmdWithSpinner(fmt.Sprintf("Concatenating scenes for %s...", sceneName),
				exec.Command("ffmpeg", "-f", "concat", "-safe", "0", "-i", sceneFile, "-c", "copy", "-y", outputMp4))
			if err != nil || exitCode != 0 {
				log.Error("Failed to concatenate scenes", "scene", sceneName, "error", err)
				os.Exit(exitCode)
			}
			log.Info("Concatenated scenes", "scene", sceneName, "file", ConvertToRelativeFromCWD(outputMp4), "duration", FormatDuration(GetMP4Duration(outputMp4)), "size", humanize.Bytes(uint64(GetFileSize(outputMp4))))
		}

		// Skip if the audio-enhanced MP4 is already up-to-date
		if isUpToDate(outputMp4WithAudio, outputMp4) {
			log.Info("Skipping audio fade", "scene", sceneName, "reason", "already up-to-date")
		} else {

			// Get fade start time
			fadeStart, err := getFadeStart(outputMp4)
			if err != nil {
				log.Warn("Failed to determine fade-out time, using default", "scene", sceneName, "error", err)
				fadeStart = 0
			}

			// Apply audio fade effect
			exitCode, err := RunCmdWithSpinner(fmt.Sprintf("Adding fade-out audio for scene %s...", sceneName),
				exec.Command("ffmpeg", "-i", outputMp4, "-i", audioFile,
					"-filter_complex", fmt.Sprintf("[1:a]afade=t=out:st=%d:d=5[aout]", fadeStart),
					"-map", "0:v", "-map", "[aout]", "-c:v", "copy", "-c:a", "aac", "-y", outputMp4WithAudio))
			if err != nil || exitCode != 0 {
				log.Error("Failed to add audio to scene", "scene", sceneName, "error", err)
				os.Exit(exitCode)
			} else {
				log.Info("Added audio to scene", "scene", sceneName, "file", ConvertToRelativeFromCWD(outputMp4WithAudio), "duration", FormatDuration(GetMP4Duration(outputMp4WithAudio)), "size", humanize.Bytes(uint64(GetFileSize(outputMp4WithAudio))))
			}
		}

		// Skip if the GIF is already up-to-date
		if isUpToDate(outputGif, outputMp4WithAudio) {
			log.Info("Skipping GIF generation", "file", ConvertToRelativeFromCWD(outputGif), "reason", "already up-to-date")
			return nil
		} else {
			// Generate GIF from the scene with mp4
			if err := createGif(outputMp4WithAudio, outputGif); err != nil {
				log.Error("Failed to generate GIF", "scene", sceneName, "error", err)
				os.Exit(1)
			} else {
				log.Info("Generated GIF", "scene", sceneName, "file", ConvertToRelativeFromCWD(outputGif))
			}
		}
	}

	return nil
}

// FormatDuration converts seconds to hh:mm:ss format
func FormatDuration(seconds float64) string {
	duration := time.Duration(seconds) * time.Second
	hours := int(duration.Hours())
	minutes := int(duration.Minutes()) % 60
	secondsInt := int(duration.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, secondsInt)
}

// GetMP4Duration returns the duration of an MP4 file in seconds
func GetMP4Duration(mp4File string) float64 {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", mp4File)
	output, err := cmd.Output()
	if err != nil {
		log.Debug("Failed to get video duration", "file", ConvertToRelativeFromCWD(mp4File), "error", err)
		return -1
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		log.Debug("Failed to parse video duration", "file", ConvertToRelativeFromCWD(mp4File), "error", err)
		return -1
	}

	return duration
}

// GetFileSize returns the size of an MP4 file in bytes
func GetFileSize(mp4File string) int64 {
	fileInfo, err := os.Stat(mp4File)
	if err != nil {
		log.Debug("Failed to get file size", "file", ConvertToRelativeFromCWD(mp4File), "error", err)
		return -1
	}

	return fileInfo.Size()
}

func isUpToDate(output, input string) bool {
	outputInfo, err := os.Stat(output)
	if err != nil {
		return false // Output file doesn't exist
	}
	inputInfo, err := os.Stat(input)
	if err != nil {
		return false // Input file doesn't exist (should not happen)
	}
	return outputInfo.ModTime().After(inputInfo.ModTime())
}

// isSceneUpToDate checks if outputMp4 is newer than both the sceneFile and all referenced mp4 files inside it
func isSceneUpToDate(outputMp4, sceneFile string) bool {
	// If outputMp4 is outdated compared to sceneFile, return false
	if !isUpToDate(outputMp4, sceneFile) {
		return false
	}

	// Regex to match "file '<filename>.mp4'"
	sceneFileRegex := regexp.MustCompile(`file '([^']+\.mp4)'`)

	// Read sceneFile contents
	data, err := os.ReadFile(sceneFile)
	if err != nil {
		log.Warn("Failed to read scene file", "file", ConvertToRelativeFromCWD(sceneFile), "error", err)
		return false
	}

	// Find all matching mp4 files in the scene file
	matches := sceneFileRegex.FindAllStringSubmatch(string(data), -1)

	for _, match := range matches {
		if len(match) < 2 {
			continue // Skip invalid matches
		}

		mp4File := match[1] // Extracted filename
		mp4FilePath := filepath.Join(filepath.Dir(sceneFile), mp4File)

		// Check if outputMp4 is up-to-date with this mp4 file
		if !isUpToDate(outputMp4, mp4FilePath) {
			log.Info("Scene is outdated because an input mp4 is newer",
				"scene", ConvertToRelativeFromCWD(sceneFile),
				"mp4", ConvertToRelativeFromCWD(mp4FilePath))
			return false
		}
	}

	// If we got here, outputMp4 is newer than everything
	return true
}

// Get fade start time from video duration
func getFadeStart(mp4File string) (int, error) {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", mp4File)
	output, err := cmd.Output()
	if err != nil {
		return 0, fmt.Errorf("failed to get video duration: %w", err)
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		return 0, fmt.Errorf("failed to parse video duration: %w", err)
	}

	fadeStart := int(duration) - 5
	if fadeStart < 0 {
		fadeStart = 0
	}
	return fadeStart, nil
}

func createGif(inputMp4, outputGif string) error {
	palette := outputGif + "-palette.png"

	// Generate palette for better GIF quality, only if outdated
	if !isUpToDate(palette, inputMp4) {
		exitCode, err := RunCmdWithSpinner("Generating palette for GIF...", exec.Command(
			"ffmpeg", "-y", "-i", inputMp4, "-vf", "palettegen", palette))
		if err != nil || exitCode != 0 {
			return fmt.Errorf("failed to generate GIF palette: %w", err)
		}
	}

	// Create GIF using the palette
	exitCode, err := RunCmdWithSpinner("Creating GIF...", exec.Command(
		"ffmpeg", "-i", inputMp4, "-i", palette, "-lavfi", "fps=10 [video]; [video][1:v] paletteuse", "-y", outputGif))
	if err != nil || exitCode != 0 {
		return fmt.Errorf("failed to create GIF: %w", err)
	}

	return nil
}
