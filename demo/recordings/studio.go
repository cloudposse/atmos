package main

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/log"
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
const viewportHeight = 10 // Viewport height for command output

func init() {
	var err error
	// Configure the logger
	log.SetDefault(log.NewWithOptions(os.Stderr, log.Options{
		ReportTimestamp: false, // Disable timestamps
		ReportCaller:    false, // Optional: Disable caller info
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
			return runWithSpinner("Cleaning up generated files...", func(updates chan string) error {
				return Clean() // Just call Clean inside
			})
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

func Clean() error {
	if err := os.RemoveAll(mp4OutDir); err != nil {
		return err
	}
	if err := os.RemoveAll(gifOutDir); err != nil {
		return err
	}
	return nil
}

func getFadeStart(mp4File string) int {
	cmd := exec.Command("ffprobe", "-v", "error", "-show_entries", "format=duration", "-of", "csv=p=0", mp4File)
	output, err := cmd.Output()
	if err != nil {
		log.Error("Failed to get video duration", "file", mp4File, "error", err)
		return 0
	}

	duration, err := strconv.ParseFloat(strings.TrimSpace(string(output)), 64)
	if err != nil {
		log.Error("Failed to parse video duration", "file", mp4File, "error", err)
		return 0
	}

	fadeStart := int(duration) - 5
	if fadeStart < 0 {
		fadeStart = 0
	}

	return fadeStart
}

func Build() error {
	if err := ensureDirs(mp4OutDir, gifOutDir); err != nil {
		return err
	}

	// Convert tapes to mp4
	if err := convertTapes(); err != nil {
		return fmt.Errorf("error converting tapes: %w", err)
	}

	// Process scenes
	if err := processScenes(); err != nil {
		return fmt.Errorf("error processing scenes: %w", err)
	}

	return nil
}

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
		baseName := filepath.Base(tape[:len(tape)-len(filepath.Ext(tape))])
		outputMp4 := filepath.Join(mp4OutDir, baseName+".mp4")
		outputGif := filepath.Join(gifOutDir, baseName+".gif")

		if isUpToDate(outputMp4, tape) {
			log.Info("Skipping", "file", baseName, "reason", "already up to date")
			continue
		}

		// Convert tape to mp4
		err := runWithSpinner(fmt.Sprintf("Converting %s to mp4...", baseName), func(updates chan string) error {
			return runCommandWithOutput(updates, "vhs", tape, "--output", outputMp4)
		})
		if err != nil {
			log.Error("Failed to convert tape", "file", baseName, "error", err)
			continue // Continue processing other tapes
		}

		// Generate GIF
		err = runWithSpinner(fmt.Sprintf("Generating GIF for %s...", baseName), func(updates chan string) error {
			return createGifWithOutput(updates, outputMp4, outputGif)
		})
		if err != nil {
			log.Error("Failed to generate GIF", "file", baseName, "error", err)
		}
	}

	return nil
}

func createGifWithOutput(updates chan string, inputMp4, outputGif string) error {
	palette := filepath.Join(filepath.Dir(outputGif), filepath.Base(outputGif)+".png")

	// Step 1: Generate palette for optimized colors
	err := runCommandWithOutput(updates, "ffmpeg", "-y", "-i", inputMp4, "-vf", "palettegen", palette)
	if err != nil {
		return fmt.Errorf("failed to generate palette for %s: %w", outputGif, err)
	}

	// Step 2: Generate GIF using the optimized palette
	err = runCommandWithOutput(updates, "ffmpeg", "-i", inputMp4, "-i", palette, "-lavfi", "fps=10 [video]; [video][1:v] paletteuse", "-y", outputGif)
	if err != nil {
		return fmt.Errorf("failed to create GIF for %s: %w", outputGif, err)
	}

	return nil
}

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
		outputMp4WithAudio := filepath.Join(mp4OutDir, sceneName+"-with-audio.mp4")
		outputGif := filepath.Join(gifOutDir, sceneName+".gif")

		// Step 1: Concatenate scenes using ffmpeg
		err := runWithSpinner(fmt.Sprintf("Concatenating scenes for %s...", sceneName), func(updates chan string) error {
			return runCommandWithOutput(updates, "ffmpeg", "-f", "concat", "-safe", "0", "-i", sceneFile, "-c", "copy", "-y", outputMp4)
		})
		if err != nil {
			log.Error("Failed to concatenate scenes", "scene", sceneName, "error", err)
			continue // Continue processing other scenes
		}

		// Step 2: Add fade-out audio
		fadeStart := getFadeStart(outputMp4)
		err = runWithSpinner(fmt.Sprintf("Adding fade-out audio for scene %s...", sceneName), func(updates chan string) error {
			return runCommandWithOutput(updates, "ffmpeg", "-i", outputMp4, "-i", audioFile, "-filter_complex", fmt.Sprintf("[1:a]afade=t=out:st=%d:d=5[aout]", fadeStart), "-map", "0:v", "-map", "[aout]", "-c:v", "copy", "-c:a", "aac", "-y", outputMp4WithAudio)
		})
		if err != nil {
			log.Error("Failed to add audio to scene", "scene", sceneName, "error", err)
			continue // Continue processing other scenes
		}

		// Step 3: Generate GIF from final MP4 with audio
		err = runWithSpinner(fmt.Sprintf("Generating GIF for scene %s...", sceneName), func(updates chan string) error {
			return createGifWithOutput(updates, outputMp4WithAudio, outputGif)
		})
		if err != nil {
			log.Error("Failed to generate GIF", "scene", sceneName, "error", err)
		}
	}

	return nil
}

func runVHSWithHeartbeat(input, output string) error {
	ctx, cancel := context.WithTimeout(context.Background(), vhsTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "vhs", input, "--output", output)
	cmd.Dir = repoRoot

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("VHS failed for %s: %w\nStderr: %s\nStdout: %s", input, err, stderr.String(), stdout.String())
	}
	return nil
}

func createGif(inputMp4, outputGif string) error {
	palette := filepath.Join(filepath.Dir(outputGif), filepath.Base(outputGif)+".png")

	if err := exec.Command("ffmpeg", "-y", "-i", inputMp4, "-vf", "palettegen", palette).Run(); err != nil {
		return err
	}

	if err := exec.Command("ffmpeg", "-i", inputMp4, "-i", palette, "-lavfi", "fps=10 [video]; [video][1:v] paletteuse", "-y", outputGif).Run(); err != nil {
		return err
	}

	return nil
}

func ensureDirs(dirs ...string) error {
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, os.ModePerm); err != nil {
			return err
		}
	}
	return nil
}

func isUpToDate(output, input string) bool {
	outputInfo, err := os.Stat(output)
	if err != nil {
		return false
	}
	inputInfo, err := os.Stat(input)
	if err != nil {
		return false
	}
	return outputInfo.ModTime().After(inputInfo.ModTime())
}

// Spinner + Viewport, handles SIGINT (^C) to terminate safely
func runWithSpinner(title string, fn func(chan string) error) error {
	updates := make(chan string)   // Command output channel
	done := make(chan struct{})    // Signal for UI exit
	errChan := make(chan error, 1) // Capture command errors
	sigChan := make(chan os.Signal, 1)
	var once sync.Once // Ensures updates is closed only once

	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM) // Handle Ctrl+C

	model := newSpinnerViewportModel(title, updates, done)
	p := tea.NewProgram(model)

	// Goroutine to run the command
	go func() {
		err := fn(updates)                 // Run the command
		once.Do(func() { close(updates) }) // Ensure updates is closed only once
		errChan <- err
	}()

	// Run the UI in a separate goroutine so we can monitor for `Ctrl+C`
	uiDone := make(chan struct{})
	go func() {
		_, _ = p.Run()
		close(uiDone) // UI is done running
	}()

	// Handle `Ctrl+C` in the main thread
	select {
	case <-sigChan: // 🚨 Wait for `Ctrl+C`
		close(done)                        // Signal UI to quit
		p.Quit()                           // Stop Bubble Tea UI
		once.Do(func() { close(updates) }) // Ensure updates is closed only once
		os.Exit(1)                         // 🚨 Force exit immediately
	case <-uiDone: // UI exited naturally
	}

	once.Do(func() { close(updates) }) // Ensure updates is closed safely
	close(done)                        // Ensure cleanup after UI exits

	return <-errChan // Return command error if any
}

type spinnerViewportModel struct {
	spinner  spinner.Model
	viewport viewport.Model
	title    string
	updates  chan string
	done     chan struct{}
	msgs     chan tea.Msg // Channel for UI updates
}

func (m spinnerViewportModel) Init() tea.Cmd {
	go func() {
		for line := range m.updates {
			m.msgs <- lineMsg(line) // Send UI update message for each line
		}
		close(m.msgs) // Close when done
	}()
	return m.spinner.Tick
}

// Custom message type for updating logs
type lineMsg string

func (m spinnerViewportModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spinner, cmd = m.spinner.Update(msg)
		return m, cmd

	case lineMsg:
		m.viewport.SetContent(m.viewport.View() + "\n" + string(msg)) // Append new log line
		return m, nil

	case tea.KeyMsg:
		return m, tea.Quit
	}

	return m, nil
}

func (m spinnerViewportModel) View() string {
	return fmt.Sprintf("\n%s %s\n%s", m.spinner.View(), m.title, m.viewport.View())
}

func newSpinnerViewportModel(title string, updates chan string, done chan struct{}) spinnerViewportModel {
	s := spinner.New()
	s.Spinner = spinner.Dot
	s.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12")) // Blue Spinner

	vp := viewport.New(80, 10) // Set viewport width and height

	return spinnerViewportModel{
		spinner:  s,
		viewport: vp,
		title:    title,
		updates:  updates,
		done:     done,
		msgs:     make(chan tea.Msg), // Initialize UI update channel
	}
}

func runCommandWithOutput(updates chan string, name string, args ...string) error {
	cmd := exec.Command(name, args...)

	stdoutPipe, _ := cmd.StdoutPipe()
	stderrPipe, _ := cmd.StderrPipe()

	if err := cmd.Start(); err != nil {
		return err
	}

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		streamOutput(stdoutPipe, updates)
	}()

	go func() {
		defer wg.Done()
		streamOutput(stderrPipe, updates)
	}()

	err := cmd.Wait()
	wg.Wait() // Ensure all goroutines complete before returning

	return err
}

func streamOutput(r io.Reader, updates chan string) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		select {
		case updates <- scanner.Text(): // Try sending to the channel
		default: // If the channel is closed, stop writing
			return
		}
	}
}
