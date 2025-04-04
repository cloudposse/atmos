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

	log "github.com/charmbracelet/log"
)

const (
	errFormatWithCause = "%w: %v"
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

// ArtifactoryClient interface allows us to mock the Artifactory Services Manager in test with only the methods
// we are using in the ArtifactoryStore.
type ArtifactoryClient interface {
	DownloadFiles(...services.DownloadParams) (int, int, error)
	UploadFiles(artifactory.UploadServiceOptions, ...services.UploadParams) (int, int, error)
}

// Ensure ArtifactoryStore implements the store.Store interface.
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

// setupArtifactoryLogger configures the JFrog SDK logger based on the current Atmos log level.
// It enables debug logging when Atmos is in debug or trace mode, otherwise disables all logging.
func setupArtifactoryLogger() {
	// Enable logging in the JFrog client when Atmos is in debug or trace mode
	currentLogLevel := log.GetLevel()

	// Debug level is 0, Trace level would be below Debug (negative values)
	if currentLogLevel <= log.DebugLevel {
		// Show DEBUG logs when Atmos is in debug or trace mode
		al.SetLogger(al.NewLogger(al.DEBUG, nil))
	} else {
		// Completely disable logging from the JFrog SDK
		// The JFrog SDK doesn't have an explicit OFF level, but setting a custom logger
		// with a nil output writer effectively disables all logging
		al.SetLogger(createNoopLogger())
	}
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

	// Configure the artifactory SDK logging based on Atmos log level
	setupArtifactoryLogger()

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

	prefixParts := []string{s.repoName, s.prefix}
	prefix := strings.Join(prefixParts, "/")

	return getKey(prefix, *s.stackDelimiter, stack, component, key, "/")
}

func (s *ArtifactoryStore) validateGetParams(stack, component, key string) error {
	if stack == "" {
		return ErrEmptyStack
	}

	if component == "" {
		return ErrEmptyComponent
	}

	if key == "" {
		return ErrEmptyKey
	}

	return nil
}

func (s *ArtifactoryStore) processDownloadedFile(tempDir, paramName string) (interface{}, error) {
	fileData, err := os.ReadFile(filepath.Join(tempDir, filepath.Base(paramName)))
	if err != nil {
		return nil, fmt.Errorf(errFormatWithCause, ErrReadFile, err)
	}

	// First try to unmarshal as JSON
	var result interface{}
	if err := json.Unmarshal(fileData, &result); err != nil {
		return nil, fmt.Errorf(errFormatWithCause, ErrUnmarshalFile, err)
	}

	return result, nil
}

func (s *ArtifactoryStore) Get(stack string, component string, key string) (interface{}, error) {
	if err := s.validateGetParams(stack, component, key); err != nil {
		return nil, err
	}

	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errFormatWithCause, ErrGetKey, err)
	}

	tempDir, err := os.MkdirTemp("", "atmos-artifactory")
	if err != nil {
		return nil, fmt.Errorf(errFormatWithCause, ErrCreateTempDir, err)
	}
	defer os.RemoveAll(tempDir)

	tempDir = filepath.Clean(tempDir)
	if !strings.HasSuffix(tempDir, string(os.PathSeparator)) {
		tempDir += string(os.PathSeparator)
	}

	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = paramName
	downloadParams.Target = tempDir
	downloadParams.Recursive = false
	downloadParams.IncludeDirs = false

	totalDownloaded, totalExpected, err := s.rtManager.DownloadFiles(downloadParams)
	if err != nil {
		return nil, fmt.Errorf(errFormatWithCause, ErrDownloadFile, err)
	}

	// Only check for mismatch if there was an error
	if err != nil && totalDownloaded != totalExpected {
		return nil, fmt.Errorf(errFormatWithCause, ErrDownloadFile, err)
	}

	if totalDownloaded == 0 {
		return nil, ErrNoFilesDownloaded
	}

	return s.processDownloadedFile(tempDir, paramName)
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
		return fmt.Errorf(errFormatWithCause, ErrGetKey, err)
	}

	tempFile, err := os.CreateTemp("", "atmos-artifactory")
	if err != nil {
		return fmt.Errorf(errFormatWithCause, ErrCreateTempFile, err)
	}
	defer os.Remove(tempFile.Name())
	defer tempFile.Close()

	var dataToWrite []byte
	if byteData, ok := value.([]byte); ok {
		// If value is already []byte, use it directly
		dataToWrite = byteData
	} else {
		// Otherwise, marshal it to JSON
		jsonData, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf(errFormatWithCause, ErrMarshalValue, err)
		}
		dataToWrite = jsonData
	}

	_, err = tempFile.Write(dataToWrite)
	if err != nil {
		return fmt.Errorf(errFormatWithCause, ErrWriteTempFile, err)
	}

	uploadParams := services.NewUploadParams()
	uploadParams.Pattern = tempFile.Name()
	uploadParams.Target = paramName
	uploadParams.Recursive = false
	uploadParams.Flat = true

	_, _, err = s.rtManager.UploadFiles(artifactory.UploadServiceOptions{FailFast: true}, uploadParams)
	if err != nil {
		return fmt.Errorf(errFormatWithCause, ErrUploadFile, err)
	}

	return nil
}
