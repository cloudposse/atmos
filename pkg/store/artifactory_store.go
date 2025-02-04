package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	"github.com/jfrog/jfrog-client-go/config"
	al "github.com/jfrog/jfrog-client-go/utils/log"
)

type ArtifactoryStore struct {
	prefix         string
	repoName       string
	rtManager      ArtifactoryClient
	stackDelimiter *string
}

type ArtifactoryStoreOptions struct {
	AccessToken    *string `mapstructure:"access_token"`
	Prefix         *string `mapstructure:"prefix"`
	RepoName       string  `mapstructure:"repo_name"`
	StackDelimiter *string `mapstructure:"stack_delimiter"`
	URL            string  `mapstructure:"url"`
}

// ArtifactoryClient interface allows us to mock the Artifactory Services Managernager in test with only the methods
// we are using in the ArtifactoryStore.
type ArtifactoryClient interface {
	DownloadFiles(...services.DownloadParams) (int, int, error)
	UploadFiles(artifactory.UploadServiceOptions, ...services.UploadParams) (int, int, error)
}

// Ensure SSMStore implements the store.Store interface.
var _ Store = (*ArtifactoryStore)(nil)

func getAccessKey(options *ArtifactoryStoreOptions) (string, error) {
	if options.AccessToken != nil {
		return *options.AccessToken, nil
	}

	if os.Getenv("ARTIFACTORY_ACCESS_TOKEN") != "" {
		return os.Getenv("ARTIFACTORY_ACCESS_TOKEN"), nil
	}

	if os.Getenv("JFROG_ACCESS_TOKEN") != "" {
		return os.Getenv("JFROG_ACCESS_TOKEN"), nil
	}

	return "", fmt.Errorf("either access_token must be set in options or one of JFROG_ACCESS_TOKEN or ARTIFACTORY_ACCESS_TOKEN environment variables must be set")
}

func NewArtifactoryStore(options ArtifactoryStoreOptions) (Store, error) {
	ctx := context.TODO()

	prefix := ""
	if options.Prefix != nil {
		prefix = *options.Prefix
	}

	stackDelimiter := "/"
	if options.StackDelimiter != nil {
		stackDelimiter = *options.StackDelimiter
	}

	al.SetLogger(al.NewLogger(al.WARN, nil))

	rtDetails := auth.NewArtifactoryDetails()
	rtDetails.SetUrl(options.URL)

	token, err := getAccessKey(&options)
	if err != nil {
		return nil, err
	}

	// If the token is set to "anonymous", we don't need to set the access token.
	if token != "anonymous" {
		rtDetails.SetAccessToken(token)
	}

	serviceConfig, err := config.NewConfigBuilder().
		SetServiceDetails(rtDetails).
		SetDryRun(false).
		SetContext(ctx).
		SetDialTimeout(180 * time.Second).
		SetOverallRequestTimeout(1 * time.Minute).
		SetHttpRetries(0).
		Build()
	if err != nil {
		return nil, err
	}

	rtManager, err := artifactory.New(serviceConfig)
	if err != nil {
		return nil, err
	}

	return &ArtifactoryStore{
		prefix:         prefix,
		repoName:       options.RepoName,
		rtManager:      rtManager,
		stackDelimiter: &stackDelimiter,
	}, nil
}

func (s *ArtifactoryStore) getKey(stack string, component string, key string) (string, error) {
	if s.stackDelimiter == nil {
		return "", fmt.Errorf("stack delimiter is not set")
	}

	prefixParts := []string{s.repoName, s.prefix}
	prefix := strings.Join(prefixParts, "/")
	return getKey(prefix, *s.stackDelimiter, stack, component, key, "/")
}

func (s *ArtifactoryStore) Get(stack string, component string, key string) (interface{}, error) {
	if stack == "" {
		return nil, fmt.Errorf("stack cannot be empty")
	}

	if component == "" {
		return nil, fmt.Errorf("component cannot be empty")
	}

	if key == "" {
		return nil, fmt.Errorf("key cannot be empty")
	}

	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf("failed to get key: %v", err)
	}

	tempDir, err := os.MkdirTemp("", "artifactorystore")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tempDir)

	tempDir = filepath.Clean(tempDir)
	if !strings.HasSuffix(tempDir, string(os.PathSeparator)) {
		tempDir += string(os.PathSeparator)
	}

	params := services.NewDownloadParams()
	params.Pattern = paramName
	params.Target = tempDir
	params.Flat = true

	totalDownloaded, totalFailed, err := s.rtManager.DownloadFiles(params)
	if err != nil {
		return nil, fmt.Errorf("failed to download file: %v", err)
	}

	if totalFailed > 0 {
		return nil, fmt.Errorf("failed to download file: %v", err)
	}

	if totalDownloaded == 0 {
		return nil, fmt.Errorf("no files downloaded")
	}

	downloadedFile := filepath.Join(tempDir, key)
	jsonData, err := os.ReadFile(downloadedFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read file: %v", err)
	}

	var result interface{}
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal file: %v", err)
	}

	return result, nil
}

func (s *ArtifactoryStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return fmt.Errorf("stack cannot be empty")
	}

	if component == "" {
		return fmt.Errorf("component cannot be empty")
	}

	if key == "" {
		return fmt.Errorf("key cannot be empty")
	}

	// Construct the full parameter name using getKey
	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf("failed to get key: %v", err)
	}

	tempFile, err := os.CreateTemp("", "key.json")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %v", err)
	}

	defer tempFile.Close()
	defer os.Remove(tempFile.Name())

	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal value: %v", err)
	}

	_, err = tempFile.Write(jsonData)
	if err != nil {
		return fmt.Errorf("failed to write to temp file: %v", err)
	}

	params := services.NewUploadParams()
	params.Pattern = tempFile.Name()
	params.Target = paramName

	params.Flat = true

	uploadServiceOptions := &artifactory.UploadServiceOptions{
		FailFast: true,
	}

	_, _, err = s.rtManager.UploadFiles(*uploadServiceOptions, params)
	if err != nil {
		return fmt.Errorf("failed to upload file: %v", err)
	}

	return nil
}
