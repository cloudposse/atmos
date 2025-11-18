package vhs

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

// CacheMetadata stores rendering metadata for scenes.
type CacheMetadata struct {
	Version string               `json:"version"`
	Scenes  map[string]SceneHash `json:"scenes"`
}

// SceneHash stores the SHA256 hash and render time for a scene.
type SceneHash struct {
	TapeHash   string    `json:"tape_hash"`
	AudioHash  string    `json:"audio_hash,omitempty"`
	RenderedAt time.Time `json:"rendered_at"`
	Outputs    []string  `json:"outputs"`
}

// LoadCache loads the cache metadata from disk.
func LoadCache(cacheDir string) (*CacheMetadata, error) {
	metadataFile := filepath.Join(cacheDir, "metadata.json")

	data, err := os.ReadFile(metadataFile)
	if os.IsNotExist(err) {
		// Cache doesn't exist yet - return empty cache.
		return &CacheMetadata{
			Version: "1.0",
			Scenes:  make(map[string]SceneHash),
		}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read cache metadata: %w", err)
	}

	var cache CacheMetadata
	if err := json.Unmarshal(data, &cache); err != nil {
		return nil, fmt.Errorf("failed to parse cache metadata: %w", err)
	}

	if cache.Scenes == nil {
		cache.Scenes = make(map[string]SceneHash)
	}

	return &cache, nil
}

// SaveCache saves the cache metadata to disk.
func (c *CacheMetadata) SaveCache(cacheDir string) error {
	metadataFile := filepath.Join(cacheDir, "metadata.json")

	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal cache metadata: %w", err)
	}

	if err := os.WriteFile(metadataFile, data, 0o644); err != nil {
		return fmt.Errorf("failed to write cache metadata: %w", err)
	}

	return nil
}

// CalculateSHA256 calculates the SHA256 hash of a file.
func CalculateSHA256(filePath string) (string, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("failed to open file: %w", err)
	}
	defer file.Close()

	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", fmt.Errorf("failed to hash file: %w", err)
	}

	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

// NeedsRender checks if a scene needs to be rendered.
// Returns true if the tape file has changed, audio file has changed, or outputs are missing.
func (c *CacheMetadata) NeedsRender(sceneName, tapeFile, audioFile string, outputs []string, force bool) (bool, error) {
	// Force flag always triggers render.
	if force {
		return true, nil
	}

	// Calculate current tape file hash.
	currentHash, err := CalculateSHA256(tapeFile)
	if err != nil {
		return false, fmt.Errorf("failed to calculate tape hash: %w", err)
	}

	// Calculate audio file hash if provided.
	var currentAudioHash string
	if audioFile != "" {
		currentAudioHash, err = CalculateSHA256(audioFile)
		if err != nil {
			return false, fmt.Errorf("failed to calculate audio hash: %w", err)
		}
	}

	// Check if scene exists in cache.
	sceneHash, exists := c.Scenes[sceneName]
	if !exists {
		// Never rendered before.
		return true, nil
	}

	// Check if tape hash changed.
	if sceneHash.TapeHash != currentHash {
		return true, nil
	}

	// Check if audio hash changed.
	if currentAudioHash != sceneHash.AudioHash {
		return true, nil
	}

	// Check if all outputs exist.
	for _, output := range outputs {
		if _, err := os.Stat(output); os.IsNotExist(err) {
			return true, nil
		}
	}

	// Cache hit - no render needed.
	return false, nil
}

// UpdateScene updates the cache for a rendered scene.
func (c *CacheMetadata) UpdateScene(sceneName, tapeFile, audioFile string, outputs []string) error {
	hash, err := CalculateSHA256(tapeFile)
	if err != nil {
		return fmt.Errorf("failed to calculate tape hash: %w", err)
	}

	var audioHash string
	if audioFile != "" {
		audioHash, err = CalculateSHA256(audioFile)
		if err != nil {
			return fmt.Errorf("failed to calculate audio hash: %w", err)
		}
	}

	c.Scenes[sceneName] = SceneHash{
		TapeHash:   hash,
		AudioHash:  audioHash,
		RenderedAt: time.Now(),
		Outputs:    outputs,
	}

	return nil
}
