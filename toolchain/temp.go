package toolchain

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// GitHubContent represents a single item from GitHub's contents API
type GitHubContent struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
}

// GetFolderPaths fetches all file paths in a specific folder from a GitHub repository
func GetFolderPaths(user, repo, folderPath string) ([]string, error) {
	// Clean the folder path - remove leading/trailing slashes
	folderPath = strings.Trim(folderPath, "/")

	// Construct the GitHub API URL
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", user, repo, folderPath)

	// Make HTTP request
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check if the response is successful
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Read and parse the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var contents []GitHubContent
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	// Extract paths from the response
	var paths []string
	for _, item := range contents {
		paths = append(paths, item.Path)
	}

	return paths, nil
}

// GetFolderPathsRecursive fetches all file paths recursively in a folder and its subdirectories
func GetFolderPathsRecursive(user, repo, folderPath string) ([]string, error) {
	var allPaths []string

	// Get contents of current folder
	contents, err := getFolderContents(user, repo, folderPath)
	if err != nil {
		return nil, err
	}

	for _, item := range contents {
		if item.Type == "file" {
			allPaths = append(allPaths, item.Path)
		} else if item.Type == "dir" {
			// Recursively get contents of subdirectory
			subPaths, err := GetFolderPathsRecursive(user, repo, item.Path)
			if err != nil {
				return nil, err
			}
			allPaths = append(allPaths, subPaths...)
		}
	}

	return allPaths, nil
}

// Helper function to get folder contents
func getFolderContents(user, repo, folderPath string) ([]GitHubContent, error) {
	folderPath = strings.Trim(folderPath, "/")
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s", user, repo, folderPath)

	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GitHub API returned status %d: %s", resp.StatusCode, string(body))
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response body: %w", err)
	}

	var contents []GitHubContent
	if err := json.Unmarshal(body, &contents); err != nil {
		return nil, fmt.Errorf("failed to parse JSON response: %w", err)
	}

	return contents, nil
}

// Example usage
func main() {
	user := "golang"
	repo := "go"
	folderPath := "src/cmd"

	// Get paths in the specified folder only
	paths, err := GetFolderPaths(user, repo, folderPath)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("Paths in %s/%s/%s:\n", user, repo, folderPath)
	for _, path := range paths {
		fmt.Printf("- %s\n", path)
	}

	// Example of recursive search
	fmt.Printf("\n--- Recursive search ---\n")
	recursivePaths, err := GetFolderPathsRecursive(user, repo, "src/cmd/compile")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	fmt.Printf("All files in %s/%s/src/cmd/compile (recursive):\n", user, repo)
	for _, path := range recursivePaths {
		fmt.Printf("- %s\n", path)
	}
}
