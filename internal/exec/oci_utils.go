package exec

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	log "github.com/charmbracelet/log" // Charmbracelet structured logger
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ecr"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/uuid"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/cloudposse/atmos/pkg/schema"
)

var ErrNoLayers = errors.New("the OCI image does not have any layers")

const (
	targetArtifactType = "application/vnd.atmos.component.terraform.v1+tar+gzip" // Target artifact type for Atmos components
	// Additional supported artifact types
	opentofuArtifactType  = "application/vnd.opentofu.modulepkg"           // OpenTofu module package
	terraformArtifactType = "application/vnd.terraform.module.v1+tar+gzip" // Terraform module package
	githubTokenEnv        = "GITHUB_TOKEN"
)

// processOciImage processes an OCI image and extracts its layers to the specified destination directory.
func processOciImage(atmosConfig *schema.AtmosConfiguration, imageName string, destDir string) error {
	tempDir, err := os.MkdirTemp("", uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer removeTempDir(tempDir)

	ref, err := name.ParseReference(imageName)
	if err != nil {
		log.Error("Failed to parse OCI image reference", "image", imageName, "error", err)
		return fmt.Errorf("invalid image reference: %w", err)
	}

	descriptor, err := pullImage(ref)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}

	img, err := descriptor.Image()
	if err != nil {
		log.Error("Failed to get image descriptor", "image", imageName, "error", err)
		return fmt.Errorf("cannot get a descriptor for the OCI image '%s': %w", imageName, err)
	}

	checkArtifactType(descriptor, imageName)

	layers, err := img.Layers()
	if err != nil {
		log.Error("Failed to retrieve layers from OCI image", "image", imageName, "error", err)
		return fmt.Errorf("failed to get image layers: %w", err)
	}

	if len(layers) == 0 {
		log.Warn("OCI image has no layers", "image", imageName)
		return ErrNoLayers
	}

	successfulLayers := 0
	for i, layer := range layers {
		if err := processLayer(layer, i, destDir); err != nil {
			log.Warn("Failed to process layer", "index", i, "error", err)
			continue // Continue with other layers instead of failing completely
		}
		successfulLayers++
	}

	// Check if any files were actually extracted
	files, err := os.ReadDir(destDir)
	if err != nil {
		log.Warn("Could not read destination directory", "dir", destDir, "error", err)
	} else if len(files) == 0 {
		log.Warn("No files were extracted to destination directory", "dir", destDir, "totalLayers", len(layers), "successfulLayers", successfulLayers)
	} else {
		log.Debug("Successfully extracted files", "dir", destDir, "fileCount", len(files), "totalLayers", len(layers), "successfulLayers", successfulLayers)
	}

	return nil
}

// pullImage pulls an OCI image from the specified reference and returns its descriptor.
func pullImage(ref name.Reference) (*remote.Descriptor, error) {
	var opts []remote.Option

	// Get registry from parsed reference
	registry := ref.Context().Registry.Name()

	// Try to get authentication from various sources
	auth, err := getRegistryAuth(registry)
	if err != nil {
		log.Debug("No authentication found, using anonymous", "registry", registry)
		opts = append(opts, remote.WithAuth(authn.Anonymous))
	} else {
		opts = append(opts, remote.WithAuth(auth))
		log.Debug("Using authentication for registry", "registry", registry)
	}

	descriptor, err := remote.Get(ref, opts...)
	if err != nil {
		log.Error("Failed to pull OCI image", "image", ref.Name(), "error", err)
		return nil, fmt.Errorf("failed to pull image '%s': %w", ref.Name(), err)
	}

	return descriptor, nil
}

// getRegistryAuth attempts to find authentication credentials for the given registry.
// It checks multiple sources in order of preference:
// 1. GitHub Container Registry (ghcr.io) with GITHUB_TOKEN
// 2. Docker credential helpers (from ~/.docker/config.json)
// 3. Environment variables for specific registries
// 4. AWS ECR authentication (if AWS credentials are available)
func getRegistryAuth(registry string) (authn.Authenticator, error) {
	// Check for GitHub Container Registry
	if strings.EqualFold(registry, "ghcr.io") {
		if token := os.Getenv("githubTokenEnv"); token != "" {
			log.Debug("Using GitHub token for authentication", "registry", registry)
			return &authn.Basic{
				Username: "oauth2",
				Password: token,
			}, nil
		}
	}

	// Terraform credentials (TF_TOKEN_* and credentials.tfrc.json)
	if auth, err := getTerraformAuth(registry); err == nil {
		log.Debug("Using Terraform credentials", "registry", registry)
		return auth, nil
	} else {
		log.Debug("Terraform credentials not found", "registry", registry, "error", err)
	}

	// Check for Docker credential helpers (most common for private registries)
	// This will automatically check ~/.docker/config.json and use credential helpers
	if auth, err := getDockerAuth(registry); err == nil {
		log.Debug("Using Docker config authentication", "registry", registry)
		return auth, nil
	}

	// Check for custom environment variables for specific registries
	// Format: REGISTRY_NAME_USERNAME and REGISTRY_NAME_PASSWORD
	// Example: MY_REGISTRY_COM_USERNAME and MY_REGISTRY_COM_PASSWORD
	registryEnvName := strings.ToUpper(strings.ReplaceAll(registry, ".", "_"))
	username := os.Getenv(fmt.Sprintf("%s_USERNAME", registryEnvName))
	password := os.Getenv(fmt.Sprintf("%s_PASSWORD", registryEnvName))

	if username != "" && password != "" {
		log.Debug("Using environment variable authentication", "registry", registry)
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	// Check for AWS ECR authentication
	if strings.Contains(registry, "dkr.ecr.") && strings.Contains(registry, "amazonaws.com") {
		if auth, err := getECRAuth(registry); err == nil {
			log.Debug("Using AWS ECR authentication", "registry", registry)
			return auth, nil
		}
	}

	// Check for Azure Container Registry authentication
	if strings.Contains(registry, "azurecr.io") {
		if auth, err := getACRAuth(registry); err == nil {
			log.Debug("Using Azure ACR authentication", "registry", registry)
			return auth, nil
		}
	}

	// Check for Google Container Registry authentication
	if strings.Contains(registry, "gcr.io") || strings.Contains(registry, "pkg.dev") {
		if auth, err := getGCRAuth(registry); err == nil {
			log.Debug("Using Google GCR authentication", "registry", registry)
			return auth, nil
		}
	}

	return nil, fmt.Errorf("no authentication found for registry %s", registry)
}

// getDockerAuth attempts to get authentication from Docker config file
// Supports DOCKER_CONFIG environment variable to override the default path
func getDockerAuth(registry string) (authn.Authenticator, error) {
	// Determine Docker config file path
	var dockerConfigPath string

	// Check if DOCKER_CONFIG environment variable is set
	if dockerConfigEnv := os.Getenv("DOCKER_CONFIG"); dockerConfigEnv != "" {
		dockerConfigPath = filepath.Join(dockerConfigEnv, "config.json")
		log.Debug("Using DOCKER_CONFIG environment variable", "path", dockerConfigPath)
	} else {
		// Get user's home directory for default path
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return nil, fmt.Errorf("failed to get user home directory: %w", err)
		}
		dockerConfigPath = filepath.Join(homeDir, ".docker", "config.json")
		log.Debug("Using default Docker config path", "path", dockerConfigPath)
	}

	// Check if Docker config file exists
	if _, err := os.Stat(dockerConfigPath); os.IsNotExist(err) {
		return nil, fmt.Errorf("Docker config file not found: %s", dockerConfigPath)
	}

	// Read Docker config file
	configData, err := os.ReadFile(dockerConfigPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read Docker config file: %w", err)
	}

	// Parse Docker config JSON
	var dockerConfig struct {
		Auths map[string]struct {
			Auth string `json:"auth"`
		} `json:"auths"`
		CredsStore  string            `json:"credsStore"`
		CredHelpers map[string]string `json:"credHelpers"`
	}

	if err := json.Unmarshal(configData, &dockerConfig); err != nil {
		return nil, fmt.Errorf("failed to parse Docker config JSON: %w", err)
	}

	// First, try to get credentials from registry-specific credential helper
	if dockerConfig.CredHelpers != nil {
		if credHelper, exists := dockerConfig.CredHelpers[registry]; exists && credHelper != "" {
			if auth, err := getCredentialStoreAuth(registry, credHelper); err == nil {
				log.Debug("Using registry-specific credential helper", "registry", registry, "helper", credHelper)
				return auth, nil
			} else {
				log.Debug("Registry-specific credential helper failed", "registry", registry, "helper", credHelper, "error", err)
			}
		}
	}

	// Fallback to global credential store if it exists
	if dockerConfig.CredsStore != "" {
		if auth, err := getCredentialStoreAuth(registry, dockerConfig.CredsStore); err == nil {
			log.Debug("Using global credential store authentication", "registry", registry, "store", dockerConfig.CredsStore)
			return auth, nil
		} else {
			log.Debug("Global credential store authentication failed", "registry", registry, "store", dockerConfig.CredsStore, "error", err)
		}
	}

	// Fallback to direct auth strings in the config file
	// Look for exact registry match first
	if authData, exists := dockerConfig.Auths[registry]; exists && authData.Auth != "" {
		username, password, err := decodeDockerAuth(authData.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth for registry %s: %w", registry, err)
		}
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	// Look for registry with https:// prefix
	httpsRegistry := "https://" + registry
	if authData, exists := dockerConfig.Auths[httpsRegistry]; exists && authData.Auth != "" {
		username, password, err := decodeDockerAuth(authData.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth for registry %s: %w", httpsRegistry, err)
		}
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	// Look for registry with http:// prefix
	httpRegistry := "http://" + registry
	if authData, exists := dockerConfig.Auths[httpRegistry]; exists && authData.Auth != "" {
		username, password, err := decodeDockerAuth(authData.Auth)
		if err != nil {
			return nil, fmt.Errorf("failed to decode auth for registry %s: %w", httpRegistry, err)
		}
		return &authn.Basic{
			Username: username,
			Password: password,
		}, nil
	}

	return nil, fmt.Errorf("no authentication found in Docker config for registry %s", registry)
}

// getCredentialStoreAuth attempts to get credentials from Docker's credential store
func getCredentialStoreAuth(registry, credsStore string) (authn.Authenticator, error) {
	// Validate registry to prevent command injection
	if strings.ContainsAny(registry, ";&|`$(){}[]<>'\"\n\r") {
		return nil, fmt.Errorf("invalid registry name: %s", registry)
	}

	// Validate credsStore to prevent command injection
	if strings.ContainsAny(credsStore, ";&|`$(){}[]<>/\\") {
		return nil, fmt.Errorf("invalid credential store name: %s", credsStore)
	}

	// For Docker Desktop on macOS, the credential store is typically "desktop"
	// We need to use the docker-credential-desktop helper to get credentials

	// Try to execute the credential helper
	cmd := exec.Command("docker-credential-"+credsStore, "get")
	cmd.Stdin = strings.NewReader(registry)

	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get credentials from store %s: %w", credsStore, err)
	}

	// Parse the JSON output from the credential helper
	var creds struct {
		Username string `json:"Username"`
		Secret   string `json:"Secret"`
	}

	if err := json.Unmarshal(output, &creds); err != nil {
		return nil, fmt.Errorf("failed to parse credential store output: %w", err)
	}

	if creds.Username == "" || creds.Secret == "" {
		return nil, fmt.Errorf("invalid credentials from store")
	}

	return &authn.Basic{
		Username: creds.Username,
		Password: creds.Secret,
	}, nil
}

// decodeDockerAuth decodes the base64-encoded auth string from Docker config
func decodeDockerAuth(authString string) (string, string, error) {
	// Decode base64
	decoded, err := base64.StdEncoding.DecodeString(authString)
	if err != nil {
		return "", "", fmt.Errorf("failed to decode base64 auth string: %w", err)
	}

	// Split username:password
	parts := strings.SplitN(string(decoded), ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid auth string format, expected username:password")
	}

	return parts[0], parts[1], nil
}

// getECRAuth attempts to get AWS ECR authentication using AWS credentials
func getECRAuth(registry string) (authn.Authenticator, error) {
	// Check if AWS credentials are available
	if os.Getenv("AWS_ACCESS_KEY_ID") == "" && os.Getenv("AWS_PROFILE") == "" {
		return nil, fmt.Errorf("AWS credentials not found")
	}

	// Extract account ID and region from registry URL
	// ECR registry format: <account-id>.dkr.ecr.<region>.amazonaws.com
	parts := strings.Split(registry, ".")
	if len(parts) < 4 {
		return nil, fmt.Errorf("invalid ECR registry format: %s", registry)
	}

	// Extract account ID (first part)
	accountID := parts[0]
	if accountID == "" {
		return nil, fmt.Errorf("could not extract account ID from ECR registry: %s", registry)
	}

	// Find the region part (should be after "ecr")
	region := ""
	for i, part := range parts {
		if part == "ecr" && i+1 < len(parts) {
			region = parts[i+1]
			break
		}
	}

	if region == "" {
		return nil, fmt.Errorf("could not extract region from ECR registry: %s", registry)
	}

	log.Debug("Extracted ECR registry info", "registry", registry, "accountID", accountID, "region", region)

	// Load AWS configuration with specific region
	cfg, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for region %s: %w", region, err)
	}

	// Create ECR client with region-specific config
	ecrClient := ecr.NewFromConfig(cfg)

	// Get ECR authorization token with account ID
	authTokenInput := &ecr.GetAuthorizationTokenInput{
		RegistryIds: []string{accountID},
	}
	authTokenOutput, err := ecrClient.GetAuthorizationToken(context.Background(), authTokenInput)
	if err != nil {
		return nil, fmt.Errorf("failed to get ECR authorization token for account %s in region %s: %w", accountID, region, err)
	}

	if len(authTokenOutput.AuthorizationData) == 0 {
		return nil, fmt.Errorf("no authorization data returned from ECR for account %s", accountID)
	}

	// Decode the authorization token
	authData := authTokenOutput.AuthorizationData[0]
	token, err := base64.StdEncoding.DecodeString(*authData.AuthorizationToken)
	if err != nil {
		return nil, fmt.Errorf("failed to decode ECR authorization token: %w", err)
	}

	// Parse username:password from token
	parts = strings.SplitN(string(token), ":", 2)
	if len(parts) != 2 {
		return nil, fmt.Errorf("invalid ECR authorization token format")
	}

	username := parts[0]
	password := parts[1]

	log.Debug("Successfully obtained ECR credentials", "registry", registry, "accountID", accountID, "region", region)

	return &authn.Basic{
		Username: username,
		Password: password,
	}, nil
}

// getACRAuth attempts to get Azure Container Registry authentication
func getACRAuth(registry string) (authn.Authenticator, error) {
	// Extract ACR name from registry URL first
	// Expected format: <acr-name>.azurecr.io
	acrName := ""
	if strings.HasSuffix(registry, ".azurecr.io") {
		acrName = strings.TrimSuffix(registry, ".azurecr.io")
	} else {
		return nil, fmt.Errorf("invalid Azure Container Registry format: %s (expected <name>.azurecr.io)", registry)
	}

	if acrName == "" {
		return nil, fmt.Errorf("could not extract ACR name from registry: %s", registry)
	}

	// Check for required Azure environment variables
	clientID := os.Getenv("AZURE_CLIENT_ID")
	clientSecret := os.Getenv("AZURE_CLIENT_SECRET")
	tenantID := os.Getenv("AZURE_TENANT_ID")
	subscriptionID := os.Getenv("AZURE_SUBSCRIPTION_ID")

	// Check for Azure CLI authentication as fallback
	if clientID == "" && clientSecret == "" && tenantID == "" {
		// Try to use Azure CLI authentication
		if os.Getenv("AZURE_CLI_AUTH") != "" || hasAzureCLI() {
			log.Debug("Using Azure CLI authentication for ACR", "registry", registry)
			return getACRAuthViaCLI(registry)
		}
		return nil, fmt.Errorf("Azure credentials not found - set AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID, AZURE_SUBSCRIPTION_ID or use Azure CLI")
	}

	// If we have some but not all credentials, validate what's missing
	if clientID == "" {
		return nil, fmt.Errorf("AZURE_CLIENT_ID environment variable is required")
	}
	if clientSecret == "" {
		return nil, fmt.Errorf("AZURE_CLIENT_SECRET environment variable is required")
	}
	if tenantID == "" {
		return nil, fmt.Errorf("AZURE_TENANT_ID environment variable is required")
	}
	if subscriptionID == "" {
		return nil, fmt.Errorf("AZURE_SUBSCRIPTION_ID environment variable is required")
	}

	log.Debug("Authenticating with Azure Container Registry", "registry", registry, "acrName", acrName)

	// For now, return an error indicating this needs Azure SDK implementation
	// In a full implementation, you would:
	// 1. Create Azure credential using azidentity.NewClientSecretCredential
	// 2. Create ACR client using armcontainerregistry.NewClient
	// 3. Call GetCredentials to get username/password
	return nil, fmt.Errorf("Azure ACR authentication requires Azure SDK implementation - ACR name: %s", acrName)
}

// hasAzureCLI checks if Azure CLI is available
func hasAzureCLI() bool {
	cmd := exec.Command("az", "version")
	return cmd.Run() == nil
}

// getACRAuthViaCLI attempts to get ACR credentials using Azure CLI
func getACRAuthViaCLI(registry string) (authn.Authenticator, error) {
	// Extract ACR name from registry URL
	acrName := ""
	if strings.HasSuffix(registry, ".azurecr.io") {
		acrName = strings.TrimSuffix(registry, ".azurecr.io")
	} else {
		return nil, fmt.Errorf("invalid Azure Container Registry format: %s (expected <name>.azurecr.io)", registry)
	}

	if acrName == "" {
		return nil, fmt.Errorf("could not extract ACR name from registry: %s", registry)
	}

	// Use Azure CLI to get ACR credentials
	cmd := exec.Command("az", "acr", "credential", "show", "--name", acrName)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get ACR credentials via Azure CLI: %w", err)
	}

	// Parse the JSON output
	var result struct {
		Passwords []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"passwords"`
		Username string `json:"username"`
	}

	if err := json.Unmarshal(output, &result); err != nil {
		return nil, fmt.Errorf("failed to parse Azure CLI output: %w", err)
	}

	if result.Username == "" {
		return nil, fmt.Errorf("no username returned from Azure CLI")
	}

	if len(result.Passwords) == 0 {
		return nil, fmt.Errorf("no passwords returned from Azure CLI")
	}

	// Use the first password (usually there are two - one for each credential)
	password := result.Passwords[0].Value

	log.Debug("Successfully obtained ACR credentials via Azure CLI", "registry", registry, "acrName", acrName)

	return &authn.Basic{
		Username: result.Username,
		Password: password,
	}, nil
}

// getGCRAuth attempts to get Google Container Registry authentication
func getGCRAuth(registry string) (authn.Authenticator, error) {
	// Check for Google Cloud credentials
	if os.Getenv("GOOGLE_APPLICATION_CREDENTIALS") != "" || os.Getenv("GCP_PROJECT") != "" {
		// For a complete implementation, you would use Google Cloud SDK to get GCR credentials
		return nil, fmt.Errorf("Google GCR authentication not fully implemented")
	}

	return nil, fmt.Errorf("Google Cloud credentials not found")
}

// processLayer processes a single OCI layer and extracts its contents to the specified destination directory.
func processLayer(layer v1.Layer, index int, destDir string) error {
	layerDesc, err := layer.Digest()
	if err != nil {
		log.Warn("Skipping layer with invalid digest", "index", index, "error", err)
		return nil
	}

	// Get layer size for debugging
	size, err := layer.Size()
	if err != nil {
		log.Warn("Could not get layer size", "index", index, "digest", layerDesc, "error", err)
	} else {
		log.Debug("Processing layer", "index", index, "digest", layerDesc, "size", size)
	}

	// Get layer media type for debugging
	mediaType, err := layer.MediaType()
	if err != nil {
		log.Warn("Could not get layer media type", "index", index, "digest", layerDesc, "error", err)
	} else {
		log.Debug("Layer media type", "index", index, "digest", layerDesc, "mediaType", mediaType)
	}

	uncompressed, err := layer.Uncompressed()
	if err != nil {
		log.Error("Layer decompression failed", "index", index, "digest", layerDesc, "error", err)
		return fmt.Errorf("layer decompression error: %w", err)
	}
	defer uncompressed.Close()

	// Try to extract the layer based on media type
	var extractionErr error

	// Check if it's a ZIP file
	mediaTypeStr := string(mediaType)
	if strings.Contains(mediaTypeStr, "zip") {
		log.Debug("Detected ZIP layer, extracting as ZIP", "index", index, "digest", layerDesc, "mediaType", mediaTypeStr)
		extractionErr = extractZipFile(uncompressed, destDir)
	} else {
		// Default to tar extraction
		log.Debug("Extracting as TAR", "index", index, "digest", layerDesc, "mediaType", mediaTypeStr)
		extractionErr = extractTarball(uncompressed, destDir)
	}

	if extractionErr != nil {
		log.Error("Layer extraction failed", "index", index, "digest", layerDesc, "error", extractionErr)

		// Try alternative extraction methods for different formats
		log.Debug("Attempting alternative extraction methods", "index", index, "digest", layerDesc)

		// Reset the uncompressed reader
		if uncompressed != nil {
			uncompressed.Close()
		}
		uncompressed, err = layer.Uncompressed()
		if err != nil {
			log.Error("Failed to reset uncompressed reader", "index", index, "digest", layerDesc, "error", err)
			return fmt.Errorf("layer decompression error: %w", err)
		}
		defer uncompressed.Close()

		// Try to extract as raw data first
		if err := extractRawData(uncompressed, destDir, index); err != nil {
			log.Error("Raw data extraction also failed", "index", index, "digest", layerDesc, "error", err)

			// If this is the first layer and it fails, it might be metadata
			if index == 0 {
				log.Warn("First layer extraction failed, this might be metadata. Skipping layer.", "index", index, "digest", layerDesc)
				return nil // Skip this layer instead of failing
			}

			return fmt.Errorf("all extraction methods failed: %w", err)
		}

		log.Debug("Successfully extracted layer using alternative method", "index", index, "digest", layerDesc)
		return nil
	}

	log.Debug("Successfully extracted layer", "index", index, "digest", layerDesc)
	return nil
}

// extractRawData attempts to extract raw data from the layer as a fallback
func extractRawData(reader io.Reader, destDir string, layerIndex int) error {
	// Create a temporary file to store the raw data
	tempFile := filepath.Join(destDir, fmt.Sprintf("layer_%d_raw", layerIndex))

	file, err := os.Create(tempFile)
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	defer file.Close()

	// Copy the raw data
	_, err = io.Copy(file, reader)
	if err != nil {
		return fmt.Errorf("failed to copy raw data: %w", err)
	}

	log.Debug("Extracted raw data to temp file", "file", tempFile)
	return nil
}

// extractZipFile extracts a ZIP file from an io.Reader into the destination directory
func extractZipFile(reader io.Reader, destDir string) error {
	// Read the entire ZIP data into memory
	zipData, err := io.ReadAll(reader)
	if err != nil {
		return fmt.Errorf("failed to read ZIP data: %w", err)
	}

	// Create a ZIP reader
	zipReader, err := zip.NewReader(bytes.NewReader(zipData), int64(len(zipData)))
	if err != nil {
		return fmt.Errorf("failed to create ZIP reader: %w", err)
	}

	// Extract each file in the ZIP
	for _, file := range zipReader.File {
		// Skip directories
		if file.FileInfo().IsDir() {
			continue
		}

		// Create the file path
		filePath := filepath.Join(destDir, file.Name)

		// Create parent directories if they don't exist
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", file.Name, err)
		}

		// Open the file in the ZIP
		rc, err := file.Open()
		if err != nil {
			return fmt.Errorf("failed to open file %s in ZIP: %w", file.Name, err)
		}

		// Create the destination file
		dstFile, err := os.Create(filePath)
		if err != nil {
			rc.Close()
			return fmt.Errorf("failed to create file %s: %w", filePath, err)
		}

		// Copy the file contents
		if _, err := io.Copy(dstFile, rc); err != nil {
			rc.Close()
			dstFile.Close()
			return fmt.Errorf("failed to copy file %s: %w", file.Name, err)
		}

		// Close both files explicitly
		rc.Close()
		dstFile.Close()

		log.Debug("Extracted file from ZIP", "file", file.Name, "path", filePath)
	}

	log.Debug("Successfully extracted ZIP file", "destination", destDir)
	return nil
}

// checkArtifactType to check and log artifact type mismatches .
func checkArtifactType(descriptor *remote.Descriptor, imageName string) {
	manifest, err := parseOCIManifest(bytes.NewReader(descriptor.Manifest))
	if err != nil {
		log.Error("Failed to parse OCI manifest", "image", imageName, "error", err)
		return
	}

	// Check if the artifact type is supported
	supportedTypes := []string{
		targetArtifactType,
		opentofuArtifactType,
		terraformArtifactType,
	}

	isSupported := false
	for _, supportedType := range supportedTypes {
		if manifest.ArtifactType == supportedType {
			isSupported = true
			break
		}
	}

	if !isSupported {
		log.Warn("OCI image artifact type not recognized", "image", imageName, "artifactType", manifest.ArtifactType, "supportedTypes", supportedTypes)
	} else {
		log.Debug("OCI image artifact type is supported", "image", imageName, "artifactType", manifest.ArtifactType)
	}
}

// ParseOCIManifest reads and decodes an OCI manifest from a JSON file.
func parseOCIManifest(manifestBytes io.Reader) (*ocispec.Manifest, error) {
	var manifest ocispec.Manifest
	if err := json.NewDecoder(manifestBytes).Decode(&manifest); err != nil {
		return nil, err
	}

	return &manifest, nil
}

// getTerraformAuth resolves registry auth from Terraform sources:
// 1) TF_TOKEN_<HOST> env (normalize dots/hyphens to underscores)
// 2) ~/.terraform.d/credentials.tfrc.json
func getTerraformAuth(registry string) (authn.Authenticator, error) {
	hostKey := strings.NewReplacer(".", "_", "-", "_").Replace(strings.ToLower(registry))
	if token := os.Getenv("TF_TOKEN_" + hostKey); token != "" {
		return &authn.Basic{Username: "terraform", Password: token}, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home dir: %w", err)
	}
	path := filepath.Join(home, ".terraform.d", "credentials.tfrc.json")
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read tfrc: %w", err)
	}
	var creds struct {
		Credentials map[string]struct {
			Token string `json:"token"`
		} `json:"credentials"`
	}
	if err := json.Unmarshal(b, &creds); err != nil {
		return nil, fmt.Errorf("parse tfrc: %w", err)
	}
	if c, ok := creds.Credentials[registry]; ok && c.Token != "" {
		return &authn.Basic{Username: "terraform", Password: c.Token}, nil
	}
	if c, ok := creds.Credentials["https://"+registry]; ok && c.Token != "" {
		return &authn.Basic{Username: "terraform", Password: c.Token}, nil
	}
	return nil, fmt.Errorf("terraform credentials not found for %s", registry)
}
