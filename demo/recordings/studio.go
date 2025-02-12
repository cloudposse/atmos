package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
	"github.com/cloudposse/atmos/internal/tui/viewport"
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

	repoRoot, err = getGitRoot()
	if err != nil {
		log.Fatal("Error detecting Git root", "error", err)
	}

	tapesDir = filepath.Join(repoRoot, "demo", "recordings", "tapes")
	scenesDir = filepath.Join(repoRoot, "demo", "recordings", "scenes")
	mp4OutDir = filepath.Join(repoRoot, "demo", "recordings", "mp4")
	gifOutDir = filepath.Join(repoRoot, "demo", "recordings", "gif")
	audioFile = filepath.Join(repoRoot, "demo", "recordings", "background.mp3")
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

// **Run a Command with a Spinner**
func RunCmdWithSpinner(title string, cmd *exec.Cmd) (int, error) {
	// Ensure we run commands from the repo root
	repoRoot, err := getGitRoot()
	if err != nil {
		return -1, fmt.Errorf("failed to get repo root: %w", err)
	}
	cmd.Dir = repoRoot // Change working directory

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

// **Git Root Detection**
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

// **Ensure Directories Exist**
func ensureDirs(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

// **Build Process**
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

// **Convert Tapes**
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

		if exitCode, err = RunCmdWithSpinner(fmt.Sprintf("Converting %s to mp4...", baseName), exec.Command("vhs", tape, "--output", outputMp4)); err != nil || exitCode != 0 {
			log.Error("Failed to convert tape", "tape", ConvertToRelativeFromCWD(tape), "file", ConvertToRelativeFromCWD(outputMp4), "error", err)
			os.Exit(exitCode)
		} else {
			log.Info("Converted tape to mp4", "file", outputMp4)
		}

		if exitCode, err = RunCmdWithSpinner(fmt.Sprintf("Generating GIF for %s...", baseName), exec.Command("ffmpeg", "-i", outputMp4, "-y", outputGif)); err != nil || exitCode != 0 {
			log.Error("Failed to generate GIF", "file", baseName, "error", err)
			os.Exit(exitCode)
		} else {
			log.Info("Generated GIF", "file", outputGif)
		}
	}

	return nil
}

// **Process Scenes**
func processScenes() error {
	files, err := filepath.Glob(filepath.Join(scenesDir, "*.txt"))
	if err != nil {
		return fmt.Errorf("failed to list scene files: %w", err)
	}
	if len(files) == 0 {
		log.Info("No scene files found. Skipping processing.")
		return nil
	}

	for _, sceneFile := range files {
		sceneName := filepath.Base(sceneFile[:len(sceneFile)-len(filepath.Ext(sceneFile))])
		outputMp4 := filepath.Join(mp4OutDir, sceneName+".mp4")

		if exitCode, err := RunCmdWithSpinner(fmt.Sprintf("Concatenating scenes for %s...", sceneName), exec.Command("ffmpeg", "-f", "concat", "-safe", "0", "-i", sceneFile, "-c", "copy", "-y", outputMp4)); err != nil || exitCode != 0 {
			log.Error("Failed to concatenate scenes", "scene", sceneName, "error", err)
			os.Exit(exitCode)
		}
		log.Info("Concatenated scenes", "scene", sceneName, "file", outputMp4)
	}

	return nil
}
