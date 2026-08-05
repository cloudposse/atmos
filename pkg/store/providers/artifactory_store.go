package providers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	log "github.com/cloudposse/atmos/pkg/logger"
	"github.com/cloudposse/atmos/pkg/store"
	"github.com/jfrog/jfrog-client-go/artifactory"
	"github.com/jfrog/jfrog-client-go/artifactory/auth"
	"github.com/jfrog/jfrog-client-go/artifactory/services"
	rtutils "github.com/jfrog/jfrog-client-go/artifactory/services/utils"
	"github.com/jfrog/jfrog-client-go/config"
	"github.com/jfrog/jfrog-client-go/utils/io/content"
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

// ArtifactoryClient interface allows us to mock the Artifactory Services Manager in test with only the methods we are using in the ArtifactoryStore.
//
//go:generate go run go.uber.org/mock/mockgen@v0.6.0 -source=$GOFILE -destination=mock_artifactory_store.go -package=providers
type ArtifactoryClient interface {
	DownloadFiles(...services.DownloadParams) (int, int, error)
	UploadFiles(artifactory.UploadServiceOptions, ...services.UploadParams) (int, int, error)
	// SearchFiles lists files matching an Ant-style pattern (metadata only, no content). Used by
	// Keys to enumerate keys under a stack/component scope.
	SearchFiles(services.SearchParams) (*content.ContentReader, error)
}

// Ensure ArtifactoryStore implements the store.Store and store.ListableStore interfaces.
var (
	_ store.Store         = (*ArtifactoryStore)(nil)
	_ store.ListableStore = (*ArtifactoryStore)(nil)
)

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

	return "", store.ErrMissingArtifactoryToken
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

func NewArtifactoryStore(options ArtifactoryStoreOptions) (store.Store, error) {
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
		return "", store.ErrStackDelimiterNotSet
	}

	prefixParts := []string{s.repoName, s.prefix}
	prefix := strings.Join(prefixParts, "/")

	return getKey(prefix, *s.stackDelimiter, stack, component, key, "/")
}

func (s *ArtifactoryStore) validateGetParams(stack, component, key string) error {
	if stack == "" {
		return store.ErrEmptyStack
	}

	if component == "" {
		return store.ErrEmptyComponent
	}

	if key == "" {
		return store.ErrEmptyKey
	}

	return nil
}

func (s *ArtifactoryStore) processDownloadedFile(tempDir, paramName string) (interface{}, error) {
	fileData, err := os.ReadFile(filepath.Join(tempDir, filepath.Base(paramName)))
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrReadFile, err)
	}

	// First try to unmarshal as JSON
	var result interface{}
	if err := json.Unmarshal(fileData, &result); err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrUnmarshalFile, err)
	}

	return result, nil
}

func (s *ArtifactoryStore) Get(stack string, component string, key string) (interface{}, error) {
	if err := s.validateGetParams(stack, component, key); err != nil {
		return nil, err
	}

	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrGetKey, err)
	}

	tempDir, err := os.MkdirTemp("", "atmos-artifactory")
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrCreateTempDir, err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Trace("Failed to remove temporary directory during cleanup", "error", err, "dir", tempDir)
		}
	}()

	tempDir = filepath.Clean(tempDir)
	if !strings.HasSuffix(tempDir, string(os.PathSeparator)) {
		tempDir += string(os.PathSeparator)
	}

	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = paramName
	downloadParams.Target = tempDir
	downloadParams.Recursive = false
	downloadParams.IncludeDirs = false
	downloadParams.Flat = true

	totalDownloaded, totalExpected, err := s.rtManager.DownloadFiles(downloadParams)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrDownloadFile, err)
	}

	// Only check for mismatch if there was an error
	if err != nil && totalDownloaded != totalExpected {
		return nil, fmt.Errorf(errWrapFormat, store.ErrDownloadFile, err)
	}

	if totalDownloaded == 0 {
		return nil, store.ErrNoFilesDownloaded
	}

	return s.processDownloadedFile(tempDir, paramName)
}

func (s *ArtifactoryStore) Set(stack string, component string, key string, value interface{}) error {
	if stack == "" {
		return store.ErrEmptyStack
	}

	if component == "" {
		return store.ErrEmptyComponent
	}

	if key == "" {
		return store.ErrEmptyKey
	}
	if value == nil {
		return fmt.Errorf("%w for key %s in stack %s component %s", store.ErrNilValue, key, stack, component)
	}

	// Construct the full parameter name using getKey
	paramName, err := s.getKey(stack, component, key)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrGetKey, err)
	}

	tempFile, err := os.CreateTemp("", "atmos-artifactory")
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrCreateTempFile, err)
	}
	defer func() {
		if err := os.Remove(tempFile.Name()); err != nil && !os.IsNotExist(err) {
			log.Trace("Failed to remove temporary file during cleanup", "error", err, "file", tempFile.Name())
		}
	}()
	defer func() {
		if err := tempFile.Close(); err != nil {
			log.Trace("Failed to close temporary file during cleanup", "error", err, "file", tempFile.Name())
		}
	}()

	var dataToWrite []byte
	if byteData, ok := value.([]byte); ok {
		// If value is already []byte, use it directly
		dataToWrite = byteData
	} else {
		// Otherwise, marshal it to JSON
		jsonData, err := json.Marshal(value)
		if err != nil {
			return fmt.Errorf(errWrapFormat, store.ErrMarshalValue, err)
		}
		dataToWrite = jsonData
	}

	_, err = tempFile.Write(dataToWrite)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrWriteTempFile, err)
	}

	uploadParams := services.NewUploadParams()
	uploadParams.Pattern = tempFile.Name()
	uploadParams.Target = paramName
	uploadParams.Recursive = false
	uploadParams.Flat = true

	_, _, err = s.rtManager.UploadFiles(artifactory.UploadServiceOptions{FailFast: true}, uploadParams)
	if err != nil {
		return fmt.Errorf(errWrapFormat, store.ErrUploadFile, err)
	}

	return nil
}

func (s *ArtifactoryStore) GetKey(key string) (interface{}, error) {
	if key == "" {
		return nil, store.ErrEmptyKey
	}

	// Use the key directly as the file path
	filePath := key

	// If prefix is set, prepend it to the key
	if s.prefix != "" {
		filePath = s.prefix + "/" + key
	}

	// Ensure the file path has the correct extension
	if !strings.HasSuffix(filePath, ".json") {
		filePath += ".json"
	}

	// Construct the full repository path.
	// Use path.Join (not filepath.Join) because this is a URL path for the Artifactory API,
	// which requires forward slashes on all platforms including Windows.
	repoPath := path.Join(s.repoName, filePath) //nolint:forbidigo // URL path requires forward slashes

	// Create a temporary directory to download the file
	tempDir, err := os.MkdirTemp("", "atmos-artifactory-*")
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrCreateTempDir, err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Trace("Failed to remove temporary directory during cleanup", "error", err, "dir", tempDir)
		}
	}()

	// JFrog SDK requires trailing separator for directory targets.
	tempDir = filepath.Clean(tempDir)
	if !strings.HasSuffix(tempDir, string(os.PathSeparator)) {
		tempDir += string(os.PathSeparator)
	}

	// Download the file from Artifactory
	downloadParams := services.NewDownloadParams()
	downloadParams.Pattern = repoPath
	downloadParams.Target = tempDir
	downloadParams.Recursive = false
	downloadParams.IncludeDirs = false
	downloadParams.Flat = true

	_, _, err = s.rtManager.DownloadFiles(downloadParams)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrDownloadFile, err)
	}

	// Read the downloaded file
	localFilePath := filepath.Join(tempDir, filepath.Base(repoPath))
	data, err := os.ReadFile(localFilePath)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormat, store.ErrReadFile, err)
	}

	if len(data) == 0 {
		return "", nil
	}

	// Try to unmarshal as JSON first, fallback to string if it fails.
	var result interface{}
	if jsonErr := json.Unmarshal(data, &result); jsonErr != nil {
		// If JSON unmarshaling fails, return as string.
		return string(data), nil
	}
	return result, nil
}

// artifactItemName returns the item's repo-relative path (repo + path + name) with the store's
// repo+prefix segment stripped, or "" if it does not fall under prefix.
func artifactItemName(item *rtutils.ResultItem, prefix string) string {
	fullPath := item.Repo
	if item.Path != "" && item.Path != "." {
		fullPath += "/" + item.Path
	}
	fullPath += "/" + item.Name

	if name := strings.TrimPrefix(fullPath, prefix+"/"); name != fullPath {
		return name
	}
	return ""
}

// collectArtifactNames drains reader, collecting each result's listed name under prefix. Factored
// out of Keys to keep its cyclomatic complexity within the linter's limit.
func collectArtifactNames(reader *content.ContentReader, prefix string) ([]string, error) {
	var names []string
	for {
		var item rtutils.ResultItem
		readErr := reader.NextRecord(&item)
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				return nil, fmt.Errorf(errWrapFormatWithID, store.ErrListArtifacts, prefix, readErr)
			}
			break
		}
		if name := artifactItemName(&item, prefix); name != "" {
			names = append(names, name)
		}
	}
	if getErr := reader.GetError(); getErr != nil {
		return nil, fmt.Errorf(errWrapFormatWithID, store.ErrListArtifacts, prefix, getErr)
	}
	return names, nil
}

// Keys lists the keys under a stack/component scope (or globally when both are empty), via
// Artifactory's file search API with a recursive Ant-style pattern. Each match's repo-relative
// path (repo + path + name) has the store's repo+prefix segment stripped to recover the key.
func (s *ArtifactoryStore) Keys(stack string, component string) ([]string, error) {
	if s.stackDelimiter == nil {
		return nil, store.ErrStackDelimiterNotSet
	}

	basePrefix := strings.Join([]string{s.repoName, s.prefix}, "/")
	prefix := getKeyPrefix(basePrefix, *s.stackDelimiter, stack, component, "/")

	searchParams := services.NewSearchParams()
	searchParams.Pattern = prefix + "/**"
	searchParams.Recursive = true

	reader, err := s.rtManager.SearchFiles(searchParams)
	if err != nil {
		return nil, fmt.Errorf(errWrapFormatWithID, store.ErrListArtifacts, prefix, err)
	}
	defer func() {
		if closeErr := reader.Close(); closeErr != nil {
			log.Trace("Failed to close Artifactory search reader", "error", closeErr, "prefix", prefix)
		}
	}()

	return collectArtifactNames(reader, prefix)
}

func init() {
	store.Register(store.KindArtifactory, buildArtifactoryStore)
}

// buildArtifactoryStore is the store.StoreFactory for Artifactory stores.
func buildArtifactoryStore(name string, config store.StoreConfig) (store.Store, error) {
	var opts ArtifactoryStoreOptions
	if err := parseOptions(config.Options, &opts); err != nil {
		return nil, fmt.Errorf(errFormat, store.ErrParseArtifactoryOptions, err)
	}

	if config.Identity != "" {
		log.Warn("Identity-based authentication is not supported for Artifactory stores, identity will be ignored",
			"store", name, "identity", config.Identity)
	}

	return NewArtifactoryStore(opts)
}
