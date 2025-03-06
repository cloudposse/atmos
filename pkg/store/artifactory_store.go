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

	return "", ErrMissingArtifactoryToken
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
		return "", ErrStackDelimiterNotSet
	}

	prefixParts := []string{s.prefix}
	prefix := strings.Join(prefixParts, "/")

	return getKey(prefix, *s.stackDelimiter, stack, component, key, "/")
}

func (s *ArtifactoryStore) Get(stack string, component string, key string) (interface{}, error) {
	if stack == "" {
		return nil, ErrEmptyStack
	}

	if component == "" {
		return nil, ErrEmptyComponent
	}

	if key == "" {
		return nil, ErrEmptyKey
	}

	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrGetKey, err)
	}

	tempDir, err := os.MkdirTemp("", "atmos-artifactory")
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	tempDir = filepath.Clean(tempDir)
	if !strings.HasSuffix(tempDir, string(os.PathSeparator)) {
		tempDir += string(os.PathSeparator)
	}

	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = filepath.Join(s.repoName, paramName)
	downloadParams.Target = tempDir
	downloadParams.Recursive = false
	downloadParams.IncludeDirs = false

	totalDownloaded, totalExpected, err := s.rtManager.DownloadFiles(downloadParams)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrDownloadFile, err)
	}

	if totalDownloaded != totalExpected {
		return nil, fmt.Errorf("%w: %v", ErrDownloadFile, err)
	}

	if totalDownloaded == 0 {
		return nil, ErrNoFilesDownloaded
	}

	fileData, err := os.ReadFile(filepath.Join(tempDir, filepath.Base(paramName)))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrReadFile, err)
	}

	// First try to unmarshal as JSON
	var result interface{}
	if err := json.Unmarshal(fileData, &result); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnmarshalFile, err)
	}

	return result, nil
}

func (s *ArtifactoryStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return ErrEmptyStack
	}

	if component == "" {
		return ErrEmptyComponent
	}

	if key == "" {
		return ErrEmptyKey
	}

	// Construct the full parameter name using getKey
	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrGetKey, err)
	}

	tempFile, err := os.CreateTemp("", "atmos-artifactory")
	if err != nil {
		return fmt.Errorf("%w: %v", ErrCreateTempFile, err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	jsonData, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrMarshalValue, err)
	}

	_, err = tempFile.Write(jsonData)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrWriteTempFile, err)
	}

	uploadParams := services.NewUploadParams()
	uploadParams.Pattern = tempFile.Name()
	uploadParams.Target = filepath.Join(s.repoName, paramName)
	uploadParams.Recursive = false
	uploadParams.Flat = true

	_, _, err = s.rtManager.UploadFiles(artifactory.UploadServiceOptions{}, uploadParams)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrUploadFile, err)
	}

	return nil
}
